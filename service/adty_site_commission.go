package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	adtySiteCommissionCapKey  = "adty_site_referral.pending_coverage_cap"
	adtySiteCommissionDaysKey = "adty_site_referral.observation_days"
)

type adtySiteCommissionPolicy struct {
	Cap  decimal.Decimal
	Days int64
}

// SaveAdtySiteCommissionPolicy stores Root's pending-coverage cap and observation days.
// Both must be positive. No default is written.
func SaveAdtySiteCommissionPolicy(capRaw, daysRaw string) error {
	cap, err := decimal.NewFromString(strings.TrimSpace(capRaw))
	if err != nil || !cap.IsPositive() {
		return common.NewMessage("Invalid parameters")
	}
	days, err := decimal.NewFromString(strings.TrimSpace(daysRaw))
	if err != nil || !days.Equal(days.Truncate(0)) || !days.IsPositive() {
		return common.NewMessage("Invalid parameters")
	}
	return model.UpdateOptionsBulk(map[string]string{
		adtySiteCommissionCapKey:  cap.String(),
		adtySiteCommissionDaysKey: days.String(),
	})
}

// AdtySiteCommissionPolicyView is blank until Root saves both values.
type AdtySiteCommissionPolicyView struct {
	Configured         bool   `json:"configured"`
	PendingCoverageCap string `json:"pending_coverage_cap,omitempty"`
	ObservationDays    string `json:"observation_days,omitempty"`
}

// LoadAdtySiteCommissionPolicy reports the saved cap and observation days.
func LoadAdtySiteCommissionPolicy() (AdtySiteCommissionPolicyView, error) {
	policy, err := loadAdtySiteCommissionPolicy()
	if errors.Is(err, ErrAdtySiteReferralUnset) {
		return AdtySiteCommissionPolicyView{}, nil
	}
	if err != nil {
		return AdtySiteCommissionPolicyView{}, err
	}
	return AdtySiteCommissionPolicyView{
		Configured: true, PendingCoverageCap: policy.Cap.String(), ObservationDays: decimal.NewFromInt(policy.Days).String(),
	}, nil
}

func loadAdtySiteCommissionPolicy() (adtySiteCommissionPolicy, error) {
	capRaw, capOK, err := model.ReadOptionFromDB(adtySiteCommissionCapKey)
	if err != nil {
		return adtySiteCommissionPolicy{}, err
	}
	daysRaw, daysOK, err := model.ReadOptionFromDB(adtySiteCommissionDaysKey)
	if err != nil {
		return adtySiteCommissionPolicy{}, err
	}
	if !capOK || !daysOK || strings.TrimSpace(capRaw) == "" || strings.TrimSpace(daysRaw) == "" {
		return adtySiteCommissionPolicy{}, ErrAdtySiteReferralUnset
	}
	cap, err := decimal.NewFromString(strings.TrimSpace(capRaw))
	if err != nil || !cap.IsPositive() {
		return adtySiteCommissionPolicy{}, ErrAdtySiteReferralUnset
	}
	days, err := decimal.NewFromString(strings.TrimSpace(daysRaw))
	if err != nil || !days.Equal(days.Truncate(0)) || !days.IsPositive() {
		return adtySiteCommissionPolicy{}, ErrAdtySiteReferralUnset
	}
	return adtySiteCommissionPolicy{Cap: cap, Days: days.IntPart()}, nil
}

func commissionMarginOptionKey(siteID int) string {
	return fmt.Sprintf("adty_site_referral.commission_margin.%d", siteID)
}

// AuthorizeAdtySiteCommissionMargin records Root's permission for this site to
// cover an invite-commission shortfall from its own margin cycle.
// It does not authorize gift margin fill and does not set an amount.
func AuthorizeAdtySiteCommissionMargin(siteID int, allowed bool) error {
	if siteID <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	var count int64
	if err := model.DB.Model(&model.AdtySite{}).Where("id = ?", siteID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return common.NewMessage("Invalid parameters")
	}
	value := "0"
	if allowed {
		value = "1"
	}
	return model.UpdateOption(commissionMarginOptionKey(siteID), value)
}

// SiteCommissionMarginAllowed reports the saved commission-margin authorization.
func SiteCommissionMarginAllowed(siteID int) (bool, error) {
	raw, ok, err := model.ReadOptionFromDB(commissionMarginOptionKey(siteID))
	if err != nil || !ok {
		return false, err
	}
	return strings.TrimSpace(raw) == "1", nil
}

// SiteInviteCommissionWithdrawable is true only after coverage and observation.
func SiteInviteCommissionWithdrawable(row *model.AdtySiteInviteCommission, now time.Time) bool {
	return row != nil && row.Status == model.AdtySiteCommissionAvailable && row.AvailableAt > 0 && !now.Before(time.Unix(row.AvailableAt, 0))
}

// RecordAdtySiteInviteCommissionTx snapshots one site invite commission.
// It does not spend a wallet. Team top-ups and missing invites record nothing.
func RecordAdtySiteInviteCommissionTx(tx *gorm.DB, ext *model.AdtyTopupOrder, now time.Time) error {
	if tx == nil || ext == nil || ext.SiteId <= 0 || ext.TeamId > 0 || ext.UserId <= 0 || strings.TrimSpace(ext.TradeNo) == "" || !ext.FaceUSD.IsPositive() {
		return nil
	}
	var invite model.AdtySiteInvite
	lookup := tx.Where("site_id = ? AND invitee_id = ?", ext.SiteId, ext.UserId).Limit(1).Find(&invite)
	if lookup.Error != nil {
		return lookup.Error
	}
	if lookup.RowsAffected == 0 || invite.InviterId == ext.UserId || !invite.Rate.IsPositive() {
		return nil
	}
	amount := SiteInviteCommissionUSD(ext.FaceUSD, invite.Rate)
	if !amount.IsPositive() {
		return nil
	}
	row := model.AdtySiteInviteCommission{
		SiteId: ext.SiteId, InviterId: invite.InviterId, InviteeId: ext.UserId, TradeNo: ext.TradeNo,
		FaceUSD: ext.FaceUSD, Rate: invite.Rate, AmountUSD: amount, Status: model.AdtySiteCommissionPendingCoverage,
		CreatedAt: now.Unix(),
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// CoverAdtySiteInviteCommissionTx spends site earnings up to Root's pending cap.
// Observation starts when coverage completes, not when the top-up was created.
func CoverAdtySiteInviteCommissionTx(tx *gorm.DB, id int64, now time.Time) error {
	policy, err := loadAdtySiteCommissionPolicy()
	if errors.Is(err, ErrAdtySiteReferralUnset) {
		return nil
	}
	if err != nil {
		return err
	}
	var row model.AdtySiteInviteCommission
	if err := model.AdtyLockForUpdate(tx).First(&row, id).Error; err != nil {
		return err
	}
	if row.Status != model.AdtySiteCommissionPendingCoverage {
		return nil
	}
	var pending decimal.Decimal
	if err := tx.Model(&model.AdtySiteInviteCommission{}).
		Where("site_id = ? AND status = ? AND id <> ?", row.SiteId, model.AdtySiteCommissionPendingCoverage, row.Id).
		Select("COALESCE(SUM(amount_usd - covered_usd), 0)").Scan(&pending).Error; err != nil {
		return err
	}
	if !pending.LessThan(policy.Cap) {
		return nil
	}
	need := row.AmountUSD.Sub(row.CoveredUSD)
	if !need.IsPositive() {
		return markSiteCommissionCovered(tx, &row, now, policy.Days)
	}
	taken, err := takeSiteCommissionCover(tx, row.SiteId, model.AdtySiteWalletEarnings, need, row.TradeNo)
	if err != nil {
		return err
	}
	row.CoveredUSD = row.CoveredUSD.Add(taken)
	if row.CoveredUSD.LessThan(row.AmountUSD) {
		marginTaken, err := takeSiteCommissionMargin(tx, row.SiteId, row.AmountUSD.Sub(row.CoveredUSD), row.TradeNo)
		if err != nil {
			return err
		}
		row.CoveredUSD = row.CoveredUSD.Add(marginTaken)
	}
	if row.CoveredUSD.LessThan(row.AmountUSD) {
		return tx.Model(&row).Update("covered_usd", row.CoveredUSD).Error
	}
	return markSiteCommissionCovered(tx, &row, now, policy.Days)
}

func takeSiteCommissionMargin(tx *gorm.DB, siteID int, need decimal.Decimal, tradeNo string) (decimal.Decimal, error) {
	allowedBusiness, err := SiteCommissionMarginAllowed(siteID)
	if err != nil || !allowedBusiness || !need.IsPositive() {
		return decimal.Zero, err
	}
	key := "commission:" + tradeNo + ":" + model.AdtySiteWalletMargin
	var existing model.AdtySiteWalletLedger
	lookup := tx.Where("idempotency_key = ?", key).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return decimal.Zero, lookup.Error
	}
	if lookup.RowsAffected > 0 {
		return decimal.Zero, nil
	}
	var wallet model.AdtySiteWallet
	if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletMargin).First(&wallet).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}
	room := AdtyMarginNewDraw(AdtyMarginDrawInput{
		TierUSD: wallet.CycleTierUSD, Balance: wallet.Balance, Used: wallet.CycleUsedUSD,
	})
	take := decimal.Min(need, room)
	if !take.IsPositive() {
		return decimal.Zero, nil
	}
	if _, err := applyAdtySiteWalletChangeTx(tx, siteID, model.AdtySiteWalletMargin, take.Neg(), AdtySiteWalletChange{
		Type: "commission_cover", RefType: "site_invite", RefID: tradeNo, IdempotencyKey: key,
	}); err != nil {
		if errors.Is(err, ErrAdtySiteWalletInsufficient) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}
	return take, nil
}

func takeSiteCommissionCover(tx *gorm.DB, siteID int, kind string, need decimal.Decimal, tradeNo string) (decimal.Decimal, error) {
	var wallet model.AdtySiteWallet
	if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", siteID, kind).First(&wallet).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}
	take := wallet.Balance
	if take.GreaterThan(need) {
		take = need
	}
	if !take.IsPositive() {
		return decimal.Zero, nil
	}
	if _, err := applyAdtySiteWalletChangeTx(tx, siteID, kind, take.Neg(), AdtySiteWalletChange{
		Type: "commission_cover", RefType: "site_invite", RefID: tradeNo, IdempotencyKey: "commission:" + tradeNo + ":" + kind,
	}); err != nil {
		if errors.Is(err, ErrAdtySiteWalletInsufficient) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}
	return take, nil
}

func markSiteCommissionCovered(tx *gorm.DB, row *model.AdtySiteInviteCommission, now time.Time, days int64) error {
	row.Status = model.AdtySiteCommissionCovered
	row.CoveredAt = now.Unix()
	row.AvailableAt = now.Unix() + days*86400
	row.CoveredUSD = row.AmountUSD
	return tx.Model(row).Updates(map[string]any{
		"status": row.Status, "covered_at": row.CoveredAt, "available_at": row.AvailableAt, "covered_usd": row.CoveredUSD,
	}).Error
}

func adtyRecordSiteInviteCommissionTx(tx *gorm.DB, ext *model.AdtyTopupOrder) error {
	now := time.Now()
	if err := RecordAdtySiteInviteCommissionTx(tx, ext, now); err != nil {
		return err
	}
	if ext == nil || strings.TrimSpace(ext.TradeNo) == "" {
		return nil
	}
	var row model.AdtySiteInviteCommission
	lookup := tx.Where("trade_no = ?", ext.TradeNo).Limit(1).Find(&row)
	if lookup.Error != nil || lookup.RowsAffected == 0 {
		return lookup.Error
	}
	return CoverAdtySiteInviteCommissionTx(tx, row.Id, now)
}

// ReleaseAdtySiteInviteCommissions marks covered rows available after observation.
// It does not credit personal quota or a second commission account.
func ReleaseAdtySiteInviteCommissions(now time.Time) error {
	return model.DB.Model(&model.AdtySiteInviteCommission{}).
		Where("status = ? AND available_at > 0 AND available_at <= ?", model.AdtySiteCommissionCovered, now.Unix()).
		Update("status", model.AdtySiteCommissionAvailable).Error
}
