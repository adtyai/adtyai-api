package service

import (
	"errors"
	"fmt"
	"strconv"
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
	adtySiteBatchThresholdKey = "adty_site_referral.batch_register_threshold"
	adtySiteBatchWindowKey    = "adty_site_referral.batch_register_window_hours"
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

// SaveAdtySiteInviteBatchPolicy stores a separate Root threshold for site invite binds.
// Both values must be positive integers. Empty values clear the policy. No default is written.
func SaveAdtySiteInviteBatchPolicy(thresholdRaw, hoursRaw string) error {
	thresholdRaw = strings.TrimSpace(thresholdRaw)
	hoursRaw = strings.TrimSpace(hoursRaw)
	if thresholdRaw == "" && hoursRaw == "" {
		return model.UpdateOptionsBulk(map[string]string{
			adtySiteBatchThresholdKey: "",
			adtySiteBatchWindowKey:    "",
		})
	}
	threshold, err := strconv.ParseInt(thresholdRaw, 10, 64)
	if err != nil || threshold <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	hours, err := strconv.ParseInt(hoursRaw, 10, 64)
	if err != nil || hours <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	return model.UpdateOptionsBulk(map[string]string{
		adtySiteBatchThresholdKey: strconv.FormatInt(threshold, 10),
		adtySiteBatchWindowKey:    strconv.FormatInt(hours, 10),
	})
}

// AdtySiteInviteBatchPolicyView is blank until Root saves both values.
type AdtySiteInviteBatchPolicyView struct {
	Configured  bool   `json:"configured"`
	Threshold   string `json:"threshold,omitempty"`
	WindowHours string `json:"window_hours,omitempty"`
}

// LoadAdtySiteInviteBatchPolicy reports the saved site-invite batch threshold.
func LoadAdtySiteInviteBatchPolicy() (AdtySiteInviteBatchPolicyView, error) {
	thresholdRaw, thresholdOK, err := model.ReadOptionFromDB(adtySiteBatchThresholdKey)
	if err != nil {
		return AdtySiteInviteBatchPolicyView{}, err
	}
	windowRaw, windowOK, err := model.ReadOptionFromDB(adtySiteBatchWindowKey)
	if err != nil {
		return AdtySiteInviteBatchPolicyView{}, err
	}
	threshold, err := strconv.ParseInt(strings.TrimSpace(thresholdRaw), 10, 64)
	hours, err2 := strconv.ParseInt(strings.TrimSpace(windowRaw), 10, 64)
	if !thresholdOK || !windowOK || err != nil || err2 != nil || threshold <= 0 || hours <= 0 {
		return AdtySiteInviteBatchPolicyView{}, nil
	}
	return AdtySiteInviteBatchPolicyView{
		Configured: true, Threshold: strconv.FormatInt(threshold, 10), WindowHours: strconv.FormatInt(hours, 10),
	}, nil
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
	if row == nil || row.Status != model.AdtySiteCommissionAvailable || row.AvailableAt <= 0 || now.Before(time.Unix(row.AvailableAt, 0)) {
		return false
	}
	return siteInviteCommissionFree(row).IsPositive()
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
	var siteRate model.AdtySiteReferralRate
	rateLookup := tx.Where("site_id = ?", ext.SiteId).Limit(1).Find(&siteRate)
	if rateLookup.Error != nil {
		return rateLookup.Error
	}
	if rateLookup.RowsAffected == 0 || !siteRate.Enabled {
		return nil
	}
	amount := SiteInviteCommissionUSD(ext.FaceUSD, invite.Rate)
	if !amount.IsPositive() {
		return nil
	}
	var observationDays int64
	if policy, err := loadAdtySiteCommissionPolicy(); err == nil {
		observationDays = policy.Days
	} else if !errors.Is(err, ErrAdtySiteReferralUnset) {
		return err
	}
	row := model.AdtySiteInviteCommission{
		SiteId: ext.SiteId, InviterId: invite.InviterId, InviteeId: ext.UserId, TradeNo: ext.TradeNo,
		FaceUSD: ext.FaceUSD, Rate: invite.Rate, AmountUSD: amount, Status: model.AdtySiteCommissionPendingCoverage,
		ObservationDays: observationDays, CreatedAt: now.Unix(),
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// CoverAdtySiteInviteCommissionTx freezes site earnings, then authorized margin.
// It does not deduct a wallet. Observation starts when the freeze is complete.
func CoverAdtySiteInviteCommissionTx(tx *gorm.DB, id int64, now time.Time) error {
	return coverAdtySiteInviteCommissionTx(tx, id, now, false)
}

// ResumeAdtySiteInviteCoverage funds the site's original pending rows in
// creation order. It does not create a commission and does not invent an amount.
func ResumeAdtySiteInviteCoverage(siteID int, now time.Time) error {
	if siteID <= 0 || model.DB == nil {
		return common.NewMessage("Invalid parameters")
	}
	var ids []int64
	if err := model.DB.Model(&model.AdtySiteInviteCommission{}).
		Where("site_id = ? AND status = ?", siteID, model.AdtySiteCommissionPendingCoverage).
		Order("created_at, id").Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := model.DB.Transaction(func(tx *gorm.DB) error {
			return coverAdtySiteInviteCommissionTx(tx, id, now, true)
		}); err != nil {
			return err
		}
	}
	return nil
}

func coverAdtySiteInviteCommissionTx(tx *gorm.DB, id int64, now time.Time, resume bool) error {
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
	if !resume {
		var pending decimal.Decimal
		if err := tx.Model(&model.AdtySiteInviteCommission{}).
			Where("site_id = ? AND status = ? AND id <> ?", row.SiteId, model.AdtySiteCommissionPendingCoverage, row.Id).
			Select("COALESCE(SUM(amount_usd - clawed_usd - covered_usd + recovered_usd), 0)").Scan(&pending).Error; err != nil {
			return err
		}
		if !pending.LessThan(policy.Cap) {
			return nil
		}
	}
	days := row.ObservationDays
	if days <= 0 {
		days = policy.Days
		row.ObservationDays = days
		if err := tx.Model(&row).Update("observation_days", days).Error; err != nil {
			return err
		}
	}
	need := row.AmountUSD.Sub(row.CoveredUSD)
	if !need.IsPositive() {
		return markSiteCommissionCovered(tx, &row, now, days)
	}
	taken, err := freezeSiteCommissionSourceTx(tx, row.SiteId, model.AdtySiteWalletEarnings, need, row.TradeNo)
	if err != nil {
		return err
	}
	row.CoveredUSD = row.CoveredUSD.Add(taken)
	row.EarningsHoldUSD = row.EarningsHoldUSD.Add(taken)
	if row.CoveredUSD.LessThan(row.AmountUSD) {
		marginTaken, err := freezeSiteCommissionMarginTx(tx, row.SiteId, row.AmountUSD.Sub(row.CoveredUSD), row.TradeNo)
		if err != nil {
			return err
		}
		row.CoveredUSD = row.CoveredUSD.Add(marginTaken)
		row.MarginHoldUSD = row.MarginHoldUSD.Add(marginTaken)
	}
	if row.CoveredUSD.LessThan(row.AmountUSD) {
		return tx.Model(&row).Updates(map[string]any{
			"covered_usd": row.CoveredUSD, "earnings_hold_usd": row.EarningsHoldUSD, "margin_hold_usd": row.MarginHoldUSD,
		}).Error
	}
	return markSiteCommissionCovered(tx, &row, now, days)
}

func freezeSiteCommissionMarginTx(tx *gorm.DB, siteID int, need decimal.Decimal, tradeNo string) (decimal.Decimal, error) {
	allowedBusiness, err := SiteCommissionMarginAllowed(siteID)
	if err != nil || !allowedBusiness || !need.IsPositive() {
		return decimal.Zero, err
	}
	var wallet model.AdtySiteWallet
	if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletMargin).First(&wallet).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}
	room := AdtyMarginNewDraw(AdtyMarginDrawInput{
		TierUSD: wallet.CycleTierUSD, Balance: wallet.Balance, Used: wallet.CycleUsedUSD, Frozen: wallet.FrozenUSD,
	})
	take := decimal.Min(need, room)
	already, err := siteCommissionFrozenHoldTx(tx, siteID, model.AdtySiteWalletMargin, tradeNo)
	if err != nil {
		return decimal.Zero, err
	}
	return applySiteCommissionFreezeTx(tx, siteID, model.AdtySiteWalletMargin, tradeNo, take, already)
}

func freezeSiteCommissionSourceTx(tx *gorm.DB, siteID int, kind string, need decimal.Decimal, tradeNo string) (decimal.Decimal, error) {
	var wallet model.AdtySiteWallet
	if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", siteID, kind).First(&wallet).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}
	take := wallet.Balance.Sub(wallet.FrozenUSD)
	if take.GreaterThan(need) {
		take = need
	}
	already, err := siteCommissionFrozenHoldTx(tx, siteID, kind, tradeNo)
	if err != nil {
		return decimal.Zero, err
	}
	return applySiteCommissionFreezeTx(tx, siteID, kind, tradeNo, take, already)
}

// siteCommissionFrozenHoldTx sums commission_freeze holds already booked for
// this order and wallet. Resume uses that total so a later increment can
// take a new idempotency key instead of treating a partial freeze as done.
func siteCommissionFrozenHoldTx(tx *gorm.DB, siteID int, kind, tradeNo string) (decimal.Decimal, error) {
	var entries []model.AdtySiteWalletLedger
	if err := tx.Where("site_id = ? AND wallet_kind = ? AND type = ? AND ref_type = ? AND ref_id = ?",
		siteID, kind, "commission_freeze", "site_invite", tradeNo).Find(&entries).Error; err != nil {
		return decimal.Zero, err
	}
	held := decimal.Zero
	for _, entry := range entries {
		held = held.Add(entry.HoldDelta)
	}
	return held, nil
}

// applySiteCommissionFreezeTx books one freeze increment. The first increment
// keeps commission:<tradeNo>:<kind> so a completed one-shot freeze stays
// uniquely keyed. Later increments append the cumulative hold so a partial
// freeze can be resumed after a top-up. A replay of the same increment is a
// no-op and must not be added to CoveredUSD again.
func applySiteCommissionFreezeTx(tx *gorm.DB, siteID int, kind, tradeNo string, take, already decimal.Decimal) (decimal.Decimal, error) {
	if !take.IsPositive() {
		return decimal.Zero, nil
	}
	key := "commission:" + tradeNo + ":" + kind
	if already.IsPositive() {
		key += ":" + already.Add(take).String()
	}
	if len(key) > 128 {
		return decimal.Zero, common.NewMessage("Invalid parameters")
	}
	var existing model.AdtySiteWalletLedger
	lookup := tx.Where("idempotency_key = ?", key).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return decimal.Zero, lookup.Error
	}
	if lookup.RowsAffected > 0 {
		return decimal.Zero, nil
	}
	if _, err := applyAdtySiteWalletChangeTx(tx, siteID, kind, decimal.Zero, AdtySiteWalletChange{
		Type: "commission_freeze", RefType: "site_invite", RefID: tradeNo, IdempotencyKey: key, HoldDelta: take,
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
		"earnings_hold_usd": row.EarningsHoldUSD, "margin_hold_usd": row.MarginHoldUSD,
	}).Error
}

func adtySiteInviteCommissionAfterRefundTx(tx *gorm.DB, ext *model.AdtyTopupOrder) error {
	if tx == nil || ext == nil || ext.SiteId <= 0 || strings.TrimSpace(ext.TradeNo) == "" {
		return nil
	}
	if ext.RefundRatio().IsZero() && ext.FaceQuota() > 0 && ext.RefundQuota <= 0 {
		return nil
	}
	var row model.AdtySiteInviteCommission
	lookup := model.AdtyLockForUpdate(tx).Where("trade_no = ?", ext.TradeNo).Limit(1).Find(&row)
	if lookup.Error != nil || lookup.RowsAffected == 0 || row.Status == model.AdtySiteCommissionClawedBack {
		return lookup.Error
	}
	thisClaw := model.SiteInviteCommissionThisClaw(row.AmountUSD, row.ClawedUSD, ext.RefundQuota, ext.FaceQuota())
	if !thisClaw.IsPositive() {
		return nil
	}
	if err := releaseSiteInviteCommissionFreezeTx(tx, &row, thisClaw); err != nil {
		return err
	}
	if err := tx.First(&row, row.Id).Error; err != nil {
		return err
	}
	withdrawnLive, err := model.SiteInviteCommissionWithdrawnLive(tx, row.Id, row.WithdrawnUSD)
	if err != nil {
		return err
	}
	releaseAmt, convertedAmt, withdrawnAmt, pendingAmt, _ := model.SiteInviteCommissionRefundParts(row, withdrawnLive, ext.RefundQuota, ext.FaceQuota())
	parts := []decimal.Decimal{releaseAmt, convertedAmt, withdrawnAmt, pendingAmt}
	target := row.ClawedUSD.Add(parts[0]).Add(parts[1]).Add(parts[2]).Add(parts[3])
	if parts[0].IsPositive() {
		if err := consumeSiteCommissionHoldTx(tx, &row, parts[0], true, "refund:"+target.String()); err != nil {
			return err
		}
		row.RecoveredUSD = row.RecoveredUSD.Add(parts[0])
	}
	shortfall := decimal.Zero
	if parts[1].IsPositive() {
		unpaid, err := clawConvertedSiteCommissionTx(tx, &row, parts[1], target)
		if err != nil {
			return err
		}
		shortfall = unpaid
		row.ConvertedClawedUSD = row.ConvertedClawedUSD.Add(parts[1])
	}
	if parts[2].IsPositive() {
		key := "commission-recovery:" + row.TradeNo + ":" + target.String()
		if err := tx.Create(&model.AdtyCommissionRecovery{
			SiteId: row.SiteId, CommissionId: row.Id, TradeNo: row.TradeNo, AmountUSD: parts[2],
			Kind: "withdrawn", Status: "open", IdempotencyKey: key,
		}).Error; err != nil {
			return err
		}
	}
	if shortfall.IsPositive() {
		key := "commission-recovery-converted:" + row.TradeNo + ":" + target.String()
		if err := tx.Create(&model.AdtyCommissionRecovery{
			SiteId: row.SiteId, CommissionId: row.Id, TradeNo: row.TradeNo, AmountUSD: shortfall,
			Kind: "converted", Status: "open", IdempotencyKey: key,
		}).Error; err != nil {
			return err
		}
	}
	row.ClawedUSD = target
	row.RecoveryUSD = row.RecoveryUSD.Add(shortfall)
	updates := map[string]any{
		"clawed_usd": row.ClawedUSD, "recovered_usd": row.RecoveredUSD, "recovery_usd": row.RecoveryUSD,
		"earnings_hold_usd": row.EarningsHoldUSD, "margin_hold_usd": row.MarginHoldUSD,
		"converted_clawed_usd": row.ConvertedClawedUSD,
	}
	if !row.ClawedUSD.LessThan(row.AmountUSD) {
		row.Status = model.AdtySiteCommissionClawedBack
		updates["status"] = row.Status
	}
	return tx.Model(&row).Updates(updates).Error
}

func splitDecimalByWeights(total decimal.Decimal, weights []decimal.Decimal) []decimal.Decimal {
	return model.SplitDecimalByWeights(total, weights)
}

func clawConvertedSiteCommissionTx(tx *gorm.DB, row *model.AdtySiteInviteCommission, amount, clawedAfter decimal.Decimal) (decimal.Decimal, error) {
	exact := amount.Mul(common.QuotaPerUnitDecimal())
	if !exact.Equal(exact.Floor()) {
		return amount, nil
	}
	quota, err := common.WalletQuotaFromDecimalStrict(exact)
	if err != nil || quota <= 0 {
		return amount, nil
	}
	before, debited, err := model.AdtyDebitUserQuotaTx(tx, row.InviterId, int64(quota))
	if err != nil {
		return decimal.Zero, err
	}
	if debited > 0 {
		key := "sic-claw:" + strconv.FormatInt(row.Id, 10) + ":" + clawedAfter.String()
		if len(key) > 64 {
			return decimal.Zero, common.NewMessage("Invalid parameters")
		}
		if err := model.InsertAdtyLedgerTx(tx, &model.AdtyBalanceLedger{
			UserId: row.InviterId, DeltaQuota: -debited, BalanceBefore: before, BalanceAfter: before - debited,
			Type: model.AdtyLedgerTypeCommissionTransfer, RefType: "site_invite", RefId: row.TradeNo,
			Reason: "site invite commission clawback", IdempotencyKey: &key,
		}); err != nil {
			return decimal.Zero, err
		}
	}
	debitedUSD := decimal.NewFromInt(debited).Div(common.QuotaPerUnitDecimal())
	shortfall := amount.Sub(debitedUSD)
	if shortfall.IsNegative() {
		return decimal.Zero, nil
	}
	return shortfall, nil
}

func siteCommissionWithdrawnOutstanding(tx *gorm.DB, commissionID int64, withdrawn decimal.Decimal) (decimal.Decimal, error) {
	return model.SiteInviteCommissionWithdrawnLive(tx, commissionID, withdrawn)
}

// consumeSiteCommissionHoldTx settles a frozen hold. release returns it to the
// original wallet. A deduction removes it from that wallet.
func consumeSiteCommissionHoldTx(tx *gorm.DB, row *model.AdtySiteInviteCommission, amount decimal.Decimal, release bool, idemBase string) error {
	if !amount.IsPositive() {
		return nil
	}
	holds := []decimal.Decimal{row.EarningsHoldUSD, row.MarginHoldUSD}
	parts := splitDecimalByWeights(amount, holds)
	settled := decimal.Zero
	for _, part := range parts {
		settled = settled.Add(part)
	}
	if settled.LessThan(amount) {
		return common.NewMessage("Not enough withdrawable commission")
	}
	kinds := []string{model.AdtySiteWalletEarnings, model.AdtySiteWalletMargin}
	codes := []string{"e", "m"}
	prefix := "cd"
	if release {
		prefix = "cr"
	}
	changed := false
	for i, part := range parts {
		if !part.IsPositive() {
			continue
		}
		key := prefix + ":" + strconv.FormatInt(row.Id, 10) + ":" + codes[i] + ":" + idemBase
		if len(key) > 128 {
			return common.NewMessage("Invalid parameters")
		}
		var existing model.AdtySiteWalletLedger
		lookup := tx.Where("idempotency_key = ?", key).Limit(1).Find(&existing)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected > 0 {
			continue
		}
		change := AdtySiteWalletChange{
			Type: "commission_release", RefType: "site_invite", RefID: row.TradeNo, IdempotencyKey: key, HoldDelta: part.Neg(),
		}
		delta := decimal.Zero
		if !release {
			change.Type = "commission_deduct"
			delta = part.Neg()
		}
		if _, err := applyAdtySiteWalletChangeTx(tx, row.SiteId, kinds[i], delta, change); err != nil {
			return err
		}
		if i == 0 {
			row.EarningsHoldUSD = row.EarningsHoldUSD.Sub(part)
		} else {
			row.MarginHoldUSD = row.MarginHoldUSD.Sub(part)
		}
		changed = true
	}
	if !changed {
		return nil
	}
	return tx.Model(row).Updates(map[string]any{
		"earnings_hold_usd": row.EarningsHoldUSD, "margin_hold_usd": row.MarginHoldUSD,
	}).Error
}

// ListAdtySiteInviteCommissions returns the inviter's commission rows and their status.
// It does not mark a row withdrawable.
func ListAdtySiteInviteCommissions(inviterID int, now time.Time) ([]model.AdtySiteInviteCommission, error) {
	if inviterID <= 0 {
		return nil, common.NewMessage("Invalid parameters")
	}
	var rows []model.AdtySiteInviteCommission
	err := model.DB.Where("inviter_id = ?", inviterID).Order("id").Find(&rows).Error
	return rows, err
}

// ConvertAdtySiteInviteCommission moves an available remainder into the inviter's
// consumption quota and deducts the matching site hold in the same transaction.
func ConvertAdtySiteInviteCommission(inviterID int, commissionID int64, amount decimal.Decimal, key string) error {
	key = strings.TrimSpace(key)
	if inviterID <= 0 || commissionID <= 0 || !amount.IsPositive() || key == "" || len(key) > 40 {
		return common.NewMessage("Invalid parameters")
	}
	exact := amount.Mul(common.QuotaPerUnitDecimal())
	if !exact.Equal(exact.Floor()) {
		return common.NewMessage("Invalid parameters")
	}
	quota, err := common.WalletQuotaFromDecimalStrict(exact)
	if err != nil || quota <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	ref := "site-comm-cvt:" + strconv.FormatInt(commissionID, 10) + ":" + key
	if len(ref) > 64 {
		return common.NewMessage("Invalid parameters")
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		existing, err := model.FindAdtyLedgerByIdemKeyTx(tx, ref)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.UserId != inviterID || existing.DeltaQuota != int64(quota) || existing.Type != model.AdtyLedgerTypeCommissionTransfer {
				return model.ErrAdtyIdempotencyConflict
			}
			return nil
		}
		var row model.AdtySiteInviteCommission
		if err := model.AdtyLockForUpdate(tx).First(&row, commissionID).Error; err != nil {
			return err
		}
		if row.InviterId != inviterID || !SiteInviteCommissionWithdrawable(&row, time.Now()) {
			return common.NewMessage("No permission to perform this action")
		}
		remaining := siteInviteCommissionFree(&row)
		if amount.GreaterThan(remaining) {
			return common.NewMessage("Invalid parameters")
		}
		if err := consumeSiteCommissionHoldTx(tx, &row, amount, false, key); err != nil {
			return err
		}
		before, err := model.AdtyCreditUserQuotaTx(tx, inviterID, int64(quota))
		if err != nil {
			return err
		}
		if err := model.InsertAdtyLedgerTx(tx, &model.AdtyBalanceLedger{
			UserId: inviterID, DeltaQuota: int64(quota), BalanceBefore: before, BalanceAfter: before + int64(quota),
			Type: model.AdtyLedgerTypeCommissionTransfer, RefType: "site_invite", RefId: row.TradeNo,
			Reason: "site invite commission to balance", IdempotencyKey: &ref,
		}); err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{
			"converted_usd":     row.ConvertedUSD.Add(amount),
			"earnings_hold_usd": row.EarningsHoldUSD, "margin_hold_usd": row.MarginHoldUSD,
		}).Error
	})
	if err != nil {
		return err
	}
	model.AdtySyncCreditUserQuotaCache(inviterID, int64(quota), "site invite commission")
	return nil
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
