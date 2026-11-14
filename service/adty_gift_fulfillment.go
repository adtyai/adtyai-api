package service

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var errGiftGuaranteeShort = errors.New("site gift guarantee is not covered")

// adtyNotifyGiftFulfillmentPending notifies the site owner and Root that a
// verified payment left a site-funded gift in 待履约. Tests may replace it.
var adtyNotifyGiftFulfillmentPending = func(ext *model.AdtyTopupOrder) {
	if ext == nil {
		return
	}
	tradeNo := ext.TradeNo
	adtyNotifyRootAdmins(func(lang string) (string, string) {
		if stringsHasZH(lang) {
			return "赠金待履约", "订单 " + tradeNo + " 本金已入账，站长赠金尚未取得足额保障，已记为待履约，暂不可消费。"
		}
		return "Gift pending fulfillment", "Order " + tradeNo + " credited the principal, but the site gift is not yet fully guaranteed and cannot be spent."
	})
	site, err := GetAdtySite(ext.SiteId)
	if err != nil || site == nil || site.OwnerUserId <= 0 {
		return
	}
	var owner model.User
	if err := model.DB.Select("id", "email", "setting", "status").First(&owner, site.OwnerUserId).Error; err != nil {
		common.SysLog(common.LogText("gift fulfillment: load owner %d: %s", site.OwnerUserId, err.Error()))
		return
	}
	setting := owner.GetSetting()
	subject, content := "Gift pending fulfillment", "Order "+tradeNo+" credited the principal, but the site gift is not yet fully guaranteed and cannot be spent."
	if stringsHasZH(setting.Language) {
		subject, content = "赠金待履约", "订单 "+tradeNo+" 本金已入账，站长赠金尚未取得足额保障，已记为待履约，暂不可消费。"
	}
	if err := NotifyUser(owner.Id, owner.Email, setting, dto.NewNotify(dto.NotifyTypeChannelUpdate, subject, content, nil)); err != nil {
		common.SysLog(common.LogText("gift fulfillment: notify owner %d: %s", owner.Id, err.Error()))
	}
}

func stringsHasZH(lang string) bool {
	return len(lang) >= 2 && (lang == "zh-CN" || lang == "zh" || lang == "zhCN" || (len(lang) >= 2 && (lang[0] == 'z' || lang[0] == 'Z') && (lang[1] == 'h' || lang[1] == 'H')))
}

func prepareTopUpGiftOnCreditTx(tx *gorm.DB, ext *model.AdtyTopupOrder, priorStatus string) (int64, error) {
	totalBonus := topUpBonusQuota(ext)
	if tx == nil || ext == nil || ext.SiteId <= 0 || !ext.BonusUSD.IsPositive() {
		return totalBonus, nil
	}
	switch priorStatus {
	case common.TopUpStatusExpired, model.TopUpStatusCanceled:
	default:
		return totalBonus, nil
	}
	siteQuota, err := siteGiftBonusQuota(ext)
	if err != nil {
		return 0, err
	}
	held, err := netGiftHoldTx(tx, ext.SiteId, ext.TradeNo)
	if err != nil {
		return 0, err
	}
	if !held.LessThan(ext.BonusUSD) {
		return totalBonus, nil
	}
	if err := reacquireAdtySiteGiftTx(tx, ext); err != nil {
		if !errors.Is(err, errGiftGuaranteeShort) && !errors.Is(err, ErrAdtySiteWalletInsufficient) {
			return 0, err
		}
		if err := upsertPendingGiftFulfillmentTx(tx, ext, siteQuota, err.Error()); err != nil {
			return 0, err
		}
		if err := tx.Model(&model.AdtyTopupOrder{}).Where("id = ?", ext.Id).Update("gift_fulfillment_status", model.AdtyGiftFulfillmentPending).Error; err != nil {
			return 0, err
		}
		ext.GiftFulfillmentStatus = model.AdtyGiftFulfillmentPending
		granted := totalBonus - siteQuota
		if granted < 0 {
			granted = 0
		}
		return granted, nil
	}
	if err := tx.Model(&model.AdtyTopupOrder{}).Where("id = ?", ext.Id).Updates(map[string]any{
		"gift_reserve_source":     ext.GiftReserveSource,
		"gift_reserve_earning":    ext.GiftReserveEarning,
		"gift_reserve_margin":     ext.GiftReserveMargin,
		"gift_fulfillment_status": "",
	}).Error; err != nil {
		return 0, err
	}
	return totalBonus, nil
}

func topUpBonusQuota(ext *model.AdtyTopupOrder) int64 {
	if ext == nil {
		return 0
	}
	bonus := ext.CreditQuota - ext.FaceQuota()
	if bonus < 0 {
		return 0
	}
	return bonus
}

func siteGiftBonusQuota(ext *model.AdtyTopupOrder) (int64, error) {
	if ext == nil || !ext.BonusUSD.IsPositive() {
		return 0, nil
	}
	quota, err := common.WalletQuotaFromDecimalStrict(ext.BonusUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil {
		return 0, err
	}
	return int64(quota), nil
}

func netGiftHoldTx(tx *gorm.DB, siteID int, tradeNo string) (decimal.Decimal, error) {
	var rows []model.AdtySiteWalletLedger
	if err := tx.Where("site_id = ? AND ref_type = ? AND ref_id = ? AND type IN ?",
		siteID, "topup", tradeNo, []string{"gift_reserve", "gift_release"}).
		Find(&rows).Error; err != nil {
		return decimal.Zero, err
	}
	held := decimal.Zero
	for _, row := range rows {
		held = held.Add(row.HoldDelta)
	}
	return held, nil
}

func reacquireAdtySiteGiftTx(tx *gorm.DB, ext *model.AdtyTopupOrder) error {
	if tx == nil || ext == nil || ext.SiteId <= 0 || !ext.BonusUSD.IsPositive() {
		return errGiftGuaranteeShort
	}
	if err := ensureAdtySiteWalletsTx(tx, ext.SiteId); err != nil {
		return err
	}
	var earning model.AdtySiteWallet
	if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", ext.SiteId, model.AdtySiteWalletEarnings).First(&earning).Error; err != nil {
		return err
	}
	fromEarning := earning.Balance.Sub(earning.FrozenUSD)
	if fromEarning.GreaterThan(ext.BonusUSD) {
		fromEarning = ext.BonusUSD
	}
	if fromEarning.IsNegative() {
		fromEarning = decimal.Zero
	}
	rest := ext.BonusUSD.Sub(fromEarning)
	fromMargin := decimal.Zero
	if rest.IsPositive() {
		if !adty_setting.SiteBonusMarginFillAllowed(ext.SiteId) {
			return errGiftGuaranteeShort
		}
		var margin model.AdtySiteWallet
		if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", ext.SiteId, model.AdtySiteWalletMargin).First(&margin).Error; err != nil {
			return err
		}
		allowed := AdtyMarginNewDraw(AdtyMarginDrawInput{TierUSD: margin.CycleTierUSD, Balance: margin.Balance, Used: margin.CycleUsedUSD, Frozen: margin.FrozenUSD})
		if rest.GreaterThan(allowed) {
			return errGiftGuaranteeShort
		}
		fromMargin = rest
	}
	key := "gift:" + ext.TradeNo
	if fromEarning.IsPositive() {
		if _, err := applyAdtySiteWalletChangeTx(tx, ext.SiteId, model.AdtySiteWalletEarnings, decimal.Zero, AdtySiteWalletChange{
			Type: "gift_reserve", RefType: "topup", RefID: ext.TradeNo, IdempotencyKey: key + ":earn:reacquire",
			HoldDelta: fromEarning,
		}); err != nil {
			return err
		}
	}
	if fromMargin.IsPositive() {
		if _, err := applyAdtySiteWalletChangeTx(tx, ext.SiteId, model.AdtySiteWalletMargin, decimal.Zero, AdtySiteWalletChange{
			Type: "gift_reserve", RefType: "topup", RefID: ext.TradeNo, IdempotencyKey: key + ":margin:reacquire",
			HoldDelta: fromMargin,
		}); err != nil {
			return err
		}
	}
	ext.GiftReserveEarning = fromEarning
	ext.GiftReserveMargin = fromMargin
	switch {
	case fromEarning.IsPositive() && fromMargin.IsPositive():
		ext.GiftReserveSource = "earning+margin"
	case fromMargin.IsPositive():
		ext.GiftReserveSource = "margin"
	default:
		ext.GiftReserveSource = "earning"
	}
	return nil
}

func upsertPendingGiftFulfillmentTx(tx *gorm.DB, ext *model.AdtyTopupOrder, bonusQuota int64, reason string) error {
	event, _ := json.Marshal(map[string]any{
		"at":      time.Now().Unix(),
		"action":  "pending",
		"reason":  reason,
		"source":  ext.GiftReserveSource,
		"earning": ext.GiftReserveEarning.String(),
		"margin":  ext.GiftReserveMargin.String(),
	})
	var existing model.AdtyGiftFulfillment
	lookup := tx.Where("trade_no = ?", ext.TradeNo).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return lookup.Error
	}
	if lookup.RowsAffected > 0 {
		if existing.Status == model.AdtyGiftFulfillmentPending {
			return nil
		}
		// A later official fulfillment or cancel is not overwritten here.
		return nil
	}
	row := &model.AdtyGiftFulfillment{
		TradeNo:         ext.TradeNo,
		UserId:          ext.UserId,
		SiteId:          ext.SiteId,
		BonusUSD:        ext.BonusUSD,
		BonusQuota:      bonusQuota,
		OriginalSource:  ext.GiftReserveSource,
		OriginalEarning: ext.GiftReserveEarning,
		OriginalMargin:  ext.GiftReserveMargin,
		Status:          model.AdtyGiftFulfillmentPending,
		Reason:          reason,
		History:         "[" + string(event) + "]",
	}
	return tx.Create(row).Error
}

func notifyPendingGiftFulfillmentAfterCredit(_ *model.TopUp, ext *model.AdtyTopupOrder, _ int) {
	if ext == nil || ext.GiftFulfillmentStatus != model.AdtyGiftFulfillmentPending {
		return
	}
	adtyNotifyGiftFulfillmentPending(ext)
}

func initGiftFulfillmentHooks() {
	model.AdtyPrepareTopUpGiftOnCreditTx = prepareTopUpGiftOnCreditTx
	model.AdtyAfterTopUpCreditCommitted = notifyPendingGiftFulfillmentAfterCredit
}
