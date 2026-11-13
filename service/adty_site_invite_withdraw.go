package service

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	adtySiteInviteWithdrawMinKey  = "adty_site_invite_withdraw.min_usd"
	adtySiteInviteWithdrawFeeKey  = "adty_site_invite_withdraw.fee_usd"
	adtySiteInviteWithdrawRateKey = "adty_site_invite_withdraw.cny_rate"
)

// SaveAdtySiteInviteWithdrawRules stores this product's own min, USDT fee, and CNY rate.
// All three must be positive. Empty values clear them. No default is written.
func SaveAdtySiteInviteWithdrawRules(minRaw, feeRaw, rateRaw string) error {
	minRaw, feeRaw, rateRaw = strings.TrimSpace(minRaw), strings.TrimSpace(feeRaw), strings.TrimSpace(rateRaw)
	if minRaw == "" && feeRaw == "" && rateRaw == "" {
		return model.UpdateOptionsBulk(map[string]string{
			adtySiteInviteWithdrawMinKey: "", adtySiteInviteWithdrawFeeKey: "", adtySiteInviteWithdrawRateKey: "",
		})
	}
	minUSD, err := decimal.NewFromString(minRaw)
	if err != nil || !minUSD.IsPositive() {
		return common.NewMessage("Invalid parameters")
	}
	fee, err := decimal.NewFromString(feeRaw)
	if err != nil || fee.IsNegative() {
		return common.NewMessage("Invalid parameters")
	}
	rate, err := decimal.NewFromString(rateRaw)
	if err != nil || !rate.IsPositive() {
		return common.NewMessage("Invalid parameters")
	}
	return model.UpdateOptionsBulk(map[string]string{
		adtySiteInviteWithdrawMinKey: minUSD.String(), adtySiteInviteWithdrawFeeKey: fee.String(), adtySiteInviteWithdrawRateKey: rate.String(),
	})
}

type siteInviteWithdrawRules struct {
	Min, Fee, Rate decimal.Decimal
	Ready          bool
}

func loadSiteInviteWithdrawRules() (siteInviteWithdrawRules, error) {
	minRaw, minOK, err := model.ReadOptionFromDB(adtySiteInviteWithdrawMinKey)
	if err != nil {
		return siteInviteWithdrawRules{}, err
	}
	feeRaw, feeOK, err := model.ReadOptionFromDB(adtySiteInviteWithdrawFeeKey)
	if err != nil {
		return siteInviteWithdrawRules{}, err
	}
	rateRaw, rateOK, err := model.ReadOptionFromDB(adtySiteInviteWithdrawRateKey)
	if err != nil {
		return siteInviteWithdrawRules{}, err
	}
	minUSD, err1 := decimal.NewFromString(strings.TrimSpace(minRaw))
	fee, err2 := decimal.NewFromString(strings.TrimSpace(feeRaw))
	rate, err3 := decimal.NewFromString(strings.TrimSpace(rateRaw))
	if !minOK || !feeOK || !rateOK || err1 != nil || err2 != nil || err3 != nil || !minUSD.IsPositive() || fee.IsNegative() || !rate.IsPositive() {
		return siteInviteWithdrawRules{}, nil
	}
	return siteInviteWithdrawRules{Min: minUSD, Fee: fee, Rate: rate, Ready: true}, nil
}

// AdtySiteInviteWithdrawalInput is one site-customer commission withdrawal request.
type AdtySiteInviteWithdrawalInput struct {
	UserID          int
	SiteID          int
	Amount          decimal.Decimal
	Method          string
	Network         string
	AccountNo       string
	AccountName     string
	QRImageURL      string
	ExpectedCNYRate decimal.Decimal
	IdempotencyKey  string
}

// CreateAdtySiteInviteWithdrawal freezes available site invite commission.
// Balance completes immediately into quota. Cash methods stay pending for that site's owner.
func CreateAdtySiteInviteWithdrawal(input AdtySiteInviteWithdrawalInput) (*model.AdtySiteInviteWithdrawal, error) {
	amount := input.Amount.Round(8)
	key := strings.TrimSpace(input.IdempotencyKey)
	method := strings.TrimSpace(input.Method)
	if input.UserID <= 0 || input.SiteID <= 0 || !amount.IsPositive() || key == "" || len(key) > 128 {
		return nil, common.NewMessage("Invalid parameters")
	}
	row := &model.AdtySiteInviteWithdrawal{
		SiteId: input.SiteID, UserId: input.UserID, AmountUSD: amount, NetUSD: amount,
		Method: method, Status: model.AdtySiteInviteWithdrawPending, IdempotencyKey: key,
	}
	rules, err := loadSiteInviteWithdrawRules()
	if err != nil {
		return nil, err
	}
	switch method {
	case "balance":
		row.Status = model.AdtySiteInviteWithdrawPaid
		row.PaidAt = time.Now().Unix()
	case "usdt":
		if !rules.Ready {
			return nil, common.NewMessage("This withdrawal method is not available")
		}
		if amount.LessThan(rules.Min) {
			return nil, common.NewMessage("The minimum withdrawal is ${{amount}}", map[string]any{"amount": rules.Min.String()})
		}
		network := strings.ToUpper(strings.TrimSpace(input.Network))
		if !ValidateUSDTAddress(network, input.AccountNo) {
			return nil, common.NewMessage("The address does not match the {{network}} network", map[string]any{"network": network})
		}
		net := amount.Sub(rules.Fee)
		if !net.IsPositive() {
			return nil, common.NewMessage("The amount must be larger than the network fee")
		}
		row.Network, row.AccountNo, row.FeeUSD, row.NetUSD = network, strings.TrimSpace(input.AccountNo), rules.Fee, net
	case "alipay", "wechat":
		if !rules.Ready {
			return nil, common.NewMessage("This withdrawal method is not available")
		}
		if amount.LessThan(rules.Min) {
			return nil, common.NewMessage("The minimum withdrawal is ${{amount}}", map[string]any{"amount": rules.Min.String()})
		}
		if !validReferralQR(input.QRImageURL) || strings.TrimSpace(input.AccountName) == "" {
			return nil, common.NewMessage("Upload the payment QR code and enter the payee name")
		}
		if !rules.Rate.Equal(input.ExpectedCNYRate) {
			return nil, &RateChangedError{Change: ReferralRateChange{Rate: rules.Rate.String(), CNYAmount: amount.Mul(rules.Rate).Round(2).StringFixed(2)}}
		}
		row.AccountName = strings.TrimSpace(input.AccountName)
		row.QRImageURL = strings.TrimSpace(input.QRImageURL)
		row.Rate = rules.Rate.String()
		row.PayoutAmount = amount.Mul(rules.Rate).Round(2).StringFixed(2)
		row.PayoutCurrency = "CNY"
	default:
		return nil, common.NewMessage("Unsupported withdrawal method")
	}
	var credit int
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var existing []model.AdtySiteInviteWithdrawal
		if err := tx.Where("idempotency_key = ?", key).Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > 0 {
			if existing[0].UserId != input.UserID || existing[0].Method != method || !existing[0].AmountUSD.Equal(amount) {
				return model.ErrAdtyIdempotencyConflict
			}
			row = &existing[0]
			return nil
		}
		var user model.User
		if err := tx.First(&user, input.UserID).Error; err != nil {
			return err
		}
		if user.SiteId != input.SiteID {
			return common.NewMessage("No permission to perform this action")
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		if err := allocateSiteInviteWithdrawalTx(tx, row, method == "balance"); err != nil {
			return err
		}
		if method == "balance" {
			exact := amount.Mul(common.QuotaPerUnitDecimal())
			quota, err := common.WalletQuotaFromDecimalStrict(exact)
			if err != nil {
				return err
			}
			credit = quota
			ref := "site-invite-wd:" + strconv.FormatInt(row.Id, 10)
			before, err := model.AdtyCreditUserQuotaTx(tx, input.UserID, int64(quota))
			if err != nil {
				return err
			}
			return model.InsertAdtyLedgerTx(tx, &model.AdtyBalanceLedger{
				UserId: input.UserID, DeltaQuota: int64(quota), BalanceBefore: before, BalanceAfter: before + int64(quota),
				Type: model.AdtyLedgerTypeCommissionTransfer, RefType: "site_invite_withdrawal", RefId: strconv.FormatInt(row.Id, 10),
				Reason: "site invite commission to balance", IdempotencyKey: &ref,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if credit > 0 {
		model.AdtySyncCreditUserQuotaCache(input.UserID, int64(credit), "site invite commission to balance")
	}
	return row, nil
}

func allocateSiteInviteWithdrawalTx(tx *gorm.DB, withdrawal *model.AdtySiteInviteWithdrawal, toBalance bool) error {
	var rows []model.AdtySiteInviteCommission
	if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND inviter_id = ?", withdrawal.SiteId, withdrawal.UserId).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	left := withdrawal.AmountUSD
	now := time.Now()
	for i := range rows {
		if !left.IsPositive() {
			break
		}
		if !SiteInviteCommissionWithdrawable(&rows[i], now) {
			continue
		}
		free := siteInviteCommissionFree(&rows[i])
		take := free
		if take.GreaterThan(left) {
			take = left
		}
		if !take.IsPositive() {
			continue
		}
		status := model.AdtySiteInviteWithdrawItemFrozen
		field := "frozen_usd"
		if toBalance {
			status = model.AdtySiteInviteWithdrawItemPaid
			field = "converted_usd"
		}
		if err := tx.Create(&model.AdtySiteInviteWithdrawalItem{
			WithdrawalId: withdrawal.Id, CommissionId: rows[i].Id, AmountUSD: take, Status: status,
		}).Error; err != nil {
			return err
		}
		if toBalance {
			if err := consumeSiteCommissionHoldTx(tx, &rows[i], take, false, "wd:"+strconv.FormatInt(withdrawal.Id, 10)); err != nil {
				return err
			}
		}
		if err := tx.Model(&rows[i]).Update(field, gorm.Expr(field+" + ?", take)).Error; err != nil {
			return err
		}
		left = left.Sub(take)
	}
	if left.IsPositive() {
		return common.NewMessage("Not enough withdrawable commission")
	}
	return nil
}

func siteInviteCommissionFree(row *model.AdtySiteInviteCommission) decimal.Decimal {
	free := row.AmountUSD.Sub(row.ClawedUSD).Sub(row.ConvertedUSD).Sub(row.WithdrawnUSD).Sub(row.FrozenUSD)
	if free.IsNegative() {
		return decimal.Zero
	}
	return free
}

// ReviewAdtySiteInviteWithdrawal approves or rejects a pending cash withdrawal.
// Only the owner of that withdrawal's site may review it.
func ReviewAdtySiteInviteWithdrawal(ownerID int, withdrawalID int64, approve bool, reason string) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtySiteInviteWithdrawal
		if err := model.AdtyLockForUpdate(tx).First(&row, withdrawalID).Error; err != nil {
			return err
		}
		if err := requireSiteInviteWithdrawOwner(tx, ownerID, row.SiteId); err != nil {
			return err
		}
		if row.Status != model.AdtySiteInviteWithdrawPending || row.Method == "balance" {
			return common.NewMessage("Only pending withdrawals can be reviewed")
		}
		if approve {
			return tx.Model(&row).Updates(map[string]any{
				"status": model.AdtySiteInviteWithdrawApproved, "reviewed_by": ownerID, "reviewed_at": time.Now().Unix(),
			}).Error
		}
		if err := releaseSiteInviteWithdrawalTx(tx, &row); err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{
			"status": model.AdtySiteInviteWithdrawRejected, "reviewed_by": ownerID, "reviewed_at": time.Now().Unix(),
			"reject_reason": strings.TrimSpace(reason),
		}).Error
	})
}

// MarkAdtySiteInviteWithdrawalPaid records the owner's offline payment.
// It moves the freeze to withdrawn commission and does not credit quota.
func MarkAdtySiteInviteWithdrawalPaid(ownerID int, withdrawalID int64, txID, proof string) error {
	txID = strings.TrimSpace(txID)
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtySiteInviteWithdrawal
		if err := model.AdtyLockForUpdate(tx).First(&row, withdrawalID).Error; err != nil {
			return err
		}
		if err := requireSiteInviteWithdrawOwner(tx, ownerID, row.SiteId); err != nil {
			return err
		}
		if row.Status != model.AdtySiteInviteWithdrawApproved {
			return common.NewMessage("This withdrawal cannot be marked as paid")
		}
		if row.Method == "usdt" && txID == "" {
			return common.NewMessage("Enter the transaction hash")
		}
		var items []model.AdtySiteInviteWithdrawalItem
		if err := tx.Where("withdrawal_id = ? AND status = ?", row.Id, model.AdtySiteInviteWithdrawItemFrozen).Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			var commission model.AdtySiteInviteCommission
			if err := model.AdtyLockForUpdate(tx).First(&commission, item.CommissionId).Error; err != nil {
				return err
			}
			if err := consumeSiteCommissionHoldTx(tx, &commission, item.AmountUSD, true, "paid:"+strconv.FormatInt(row.Id, 10)+":"+strconv.FormatInt(item.Id, 10)); err != nil {
				return err
			}
			if err := tx.Model(&commission).Updates(map[string]any{
				"frozen_usd":    gorm.Expr("frozen_usd - ?", item.AmountUSD),
				"withdrawn_usd": gorm.Expr("withdrawn_usd + ?", item.AmountUSD),
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&item).Update("status", model.AdtySiteInviteWithdrawItemPaid).Error; err != nil {
				return err
			}
		}
		return tx.Model(&row).Updates(map[string]any{
			"status": model.AdtySiteInviteWithdrawPaid, "paid_at": time.Now().Unix(), "reviewed_by": ownerID,
			"tx_id": txID, "proof_note": strings.TrimSpace(proof),
		}).Error
	})
}

func requireSiteInviteWithdrawOwner(tx *gorm.DB, ownerID, siteID int) error {
	var site model.AdtySite
	if err := tx.First(&site, siteID).Error; err != nil {
		return err
	}
	if site.OwnerUserId != ownerID {
		return common.NewMessage("No permission to perform this action")
	}
	return nil
}

func releaseSiteInviteCommissionFreezeTx(tx *gorm.DB, commission *model.AdtySiteInviteCommission, amount decimal.Decimal) error {
	if !amount.IsPositive() || !commission.FrozenUSD.IsPositive() {
		return nil
	}
	var items []model.AdtySiteInviteWithdrawalItem
	if err := tx.Where("commission_id = ? AND status = ?", commission.Id, model.AdtySiteInviteWithdrawItemFrozen).Order("id").Find(&items).Error; err != nil {
		return err
	}
	left := amount
	if left.GreaterThan(commission.FrozenUSD) {
		left = commission.FrozenUSD
	}
	for _, item := range items {
		if !left.IsPositive() {
			break
		}
		var withdrawal model.AdtySiteInviteWithdrawal
		if err := model.AdtyLockForUpdate(tx).First(&withdrawal, item.WithdrawalId).Error; err != nil {
			return err
		}
		if withdrawal.Status != model.AdtySiteInviteWithdrawPending && withdrawal.Status != model.AdtySiteInviteWithdrawApproved {
			continue
		}
		if err := releaseSiteInviteWithdrawalTx(tx, &withdrawal); err != nil {
			return err
		}
		if err := tx.Model(&withdrawal).Updates(map[string]any{
			"status": model.AdtySiteInviteWithdrawCanceled, "reject_reason": "refund released the freeze",
		}).Error; err != nil {
			return err
		}
		left = left.Sub(item.AmountUSD)
	}
	return nil
}

func releaseSiteInviteWithdrawalTx(tx *gorm.DB, row *model.AdtySiteInviteWithdrawal) error {
	var items []model.AdtySiteInviteWithdrawalItem
	if err := tx.Where("withdrawal_id = ? AND status = ?", row.Id, model.AdtySiteInviteWithdrawItemFrozen).Find(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		if err := tx.Model(&model.AdtySiteInviteCommission{}).Where("id = ?", item.CommissionId).
			Update("frozen_usd", gorm.Expr("frozen_usd - ?", item.AmountUSD)).Error; err != nil {
			return err
		}
		if err := tx.Model(&item).Update("status", model.AdtySiteInviteWithdrawItemReleased).Error; err != nil {
			return err
		}
	}
	return nil
}

// AdtySiteInviteWithdrawRulesView is blank for cash methods until Root saves min, fee, and rate.
type AdtySiteInviteWithdrawRulesView struct {
	Configured bool     `json:"configured"`
	MinUSD     string   `json:"min_usd,omitempty"`
	FeeUSD     string   `json:"fee_usd,omitempty"`
	CNYRate    string   `json:"cny_rate,omitempty"`
	Methods    []string `json:"methods"`
}

// LoadAdtySiteInviteWithdrawRules reports the separate site-invite withdrawal rules.
func LoadAdtySiteInviteWithdrawRules() (AdtySiteInviteWithdrawRulesView, error) {
	rules, err := loadSiteInviteWithdrawRules()
	if err != nil {
		return AdtySiteInviteWithdrawRulesView{}, err
	}
	view := AdtySiteInviteWithdrawRulesView{Methods: []string{"balance"}}
	if !rules.Ready {
		return view, nil
	}
	view.Configured = true
	view.MinUSD, view.FeeUSD, view.CNYRate = rules.Min.String(), rules.Fee.String(), rules.Rate.String()
	view.Methods = []string{"balance", "usdt", "alipay", "wechat"}
	return view, nil
}

// ListAdtySiteInviteWithdrawals returns the applicant's own requests.
func ListAdtySiteInviteWithdrawals(userID int) ([]model.AdtySiteInviteWithdrawal, error) {
	var rows []model.AdtySiteInviteWithdrawal
	err := model.DB.Where("user_id = ?", userID).Order("id DESC").Find(&rows).Error
	return rows, err
}

// ListAdtyOwnerSiteInviteWithdrawals returns withdrawals for the owner's site.
func ListAdtyOwnerSiteInviteWithdrawals(ownerID int) ([]model.AdtySiteInviteWithdrawal, error) {
	var site model.AdtySite
	if err := model.DB.Where("owner_user_id = ?", ownerID).First(&site).Error; err != nil {
		return nil, err
	}
	var rows []model.AdtySiteInviteWithdrawal
	err := model.DB.Where("site_id = ?", site.Id).Order("id DESC").Find(&rows).Error
	return rows, err
}

// CancelAdtySiteInviteWithdrawal lets the applicant cancel a pending request inside the main-site cancel window.
func CancelAdtySiteInviteWithdrawal(userID int, withdrawalID int64, now int64) error {
	window := int64(max(adty_setting.GetReferralSetting().CancelWindowMinutes, 0)) * 60
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtySiteInviteWithdrawal
		if err := model.AdtyLockForUpdate(tx).Where("id = ? AND user_id = ?", withdrawalID, userID).First(&row).Error; err != nil {
			return err
		}
		if row.Status != model.AdtySiteInviteWithdrawPending {
			return common.NewMessage("Only pending withdrawals can be canceled")
		}
		if now-row.CreatedAt > window {
			return common.NewMessage("The cancel window has passed")
		}
		if err := releaseSiteInviteWithdrawalTx(tx, &row); err != nil {
			return err
		}
		return tx.Model(&row).Update("status", model.AdtySiteInviteWithdrawCanceled).Error
	})
}
