package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
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

// adtyNotifyGiftFulfillmentFulfilled tells the customer that a 待履约 gift was
// finally paid out after the site topped up its guarantee. Tests may replace it.
var adtyNotifyGiftFulfillmentFulfilled = func(row *model.AdtyGiftFulfillment, quota int64) {
	if row == nil || quota <= 0 {
		return
	}
	usd := decimal.NewFromInt(quota).Div(decimal.NewFromFloat(common.QuotaPerUnit)).Round(2).StringFixed(2)
	var customer model.User
	if err := model.DB.Select("id", "email", "setting", "status").First(&customer, row.UserId).Error; err != nil {
		common.SysLog(common.LogText("gift fulfillment: load customer %d: %s", row.UserId, err.Error()))
		return
	}
	setting := customer.GetSetting()
	subject := "Gift fulfilled"
	content := "The site gift for order " + row.TradeNo + " is credited: +" + usd + " USD (" + strconv.FormatInt(quota, 10) + " quota)."
	if stringsHasZH(setting.Language) {
		subject = "赠金已履约"
		content = "订单 " + row.TradeNo + " 的站长赠金已到账：+" + usd + " USD（" + strconv.FormatInt(quota, 10) + " 额度）。"
	}
	if err := NotifyUser(customer.Id, customer.Email, setting, dto.NewNotify(dto.NotifyTypeChannelUpdate, subject, content, nil)); err != nil {
		common.SysLog(common.LogText("gift fulfillment: notify customer %d: %s", customer.Id, err.Error()))
	}
}

func stringsHasZH(lang string) bool {
	return len(lang) >= 2 && (lang == "zh-CN" || lang == "zh" || lang == "zhCN" || (len(lang) >= 2 && (lang[0] == 'z' || lang[0] == 'Z') && (lang[1] == 'h' || lang[1] == 'H')))
}

func prepareTopUpGiftOnCreditTx(tx *gorm.DB, ext *model.AdtyTopupOrder, priorStatus string) (int64, error) {
	totalBonus := topUpBonusQuota(ext)
	bonus := adtyBonusUSDForSite(ext)
	if tx == nil || ext == nil || ext.SiteId <= 0 || !bonus.IsPositive() {
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
	if !held.LessThan(bonus) {
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
	if ext == nil {
		return 0, nil
	}
	bonus := adtyBonusUSDForSite(ext)
	if !bonus.IsPositive() {
		return 0, nil
	}
	quota, err := common.WalletQuotaFromDecimalStrict(bonus.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil {
		return 0, err
	}
	return int64(quota), nil
}

// adtyGiftNetHoldTypes is the typelist for remaining site-gift guarantee hold.
// gift_spend and gift_release both carry a negative HoldDelta.
func adtyGiftNetHoldTypes() []string {
	return []string{"gift_reserve", "gift_release", "gift_spend"}
}

func isAdtyGiftNetHoldType(typ string) bool {
	switch typ {
	case "gift_reserve", "gift_release", "gift_spend":
		return true
	default:
		return false
	}
}

func netGiftHoldTx(tx *gorm.DB, siteID int, tradeNo string) (decimal.Decimal, error) {
	var rows []model.AdtySiteWalletLedger
	if err := tx.Where("site_id = ? AND ref_type = ? AND ref_id = ? AND type IN ?",
		siteID, "topup", tradeNo, adtyGiftNetHoldTypes()).
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
	if tx == nil || ext == nil || ext.SiteId <= 0 {
		return errGiftGuaranteeShort
	}
	bonus := adtyBonusUSDForSite(ext)
	if !bonus.IsPositive() {
		return errGiftGuaranteeShort
	}
	if err := ensureAdtySiteWalletsTx(tx, ext.SiteId); err != nil {
		return err
	}
	var earning model.AdtySiteWallet
	if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", ext.SiteId, model.AdtySiteWalletEarnings).First(&earning).Error; err != nil {
		return err
	}
	held, err := netGiftHoldTx(tx, ext.SiteId, ext.TradeNo)
	if err != nil {
		return err
	}
	if held.IsNegative() {
		held = decimal.Zero
	}
	needed := bonus.Sub(held)
	if !needed.IsPositive() {
		return nil
	}
	fromEarning := earning.Balance.Sub(earning.FrozenUSD)
	if fromEarning.GreaterThan(needed) {
		fromEarning = needed
	}
	if fromEarning.IsNegative() {
		fromEarning = decimal.Zero
	}
	rest := needed.Sub(fromEarning)
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
	// Official snapshot used later by ReleaseAdtySiteGiftTx. Capture the
	// pre-increment base now so a shortfall top-up can add this reservation
	// onto the original official total instead of replacing it.
	addToExisting := held.IsPositive()
	origEarning, origMargin := ext.GiftReserveEarning, ext.GiftReserveMargin
	if addToExisting && !origEarning.IsPositive() && !origMargin.IsPositive() {
		heldEarning, heldMargin, err := netGiftHoldByKindTx(tx, ext.SiteId, ext.TradeNo)
		if err != nil {
			return err
		}
		origEarning, origMargin = heldEarning, heldMargin
	}
	key := "gift:" + ext.TradeNo
	already, err := adtyWalletIdempotencyExists(tx, key+":earn:reacquire", key+":margin:reacquire")
	if err != nil {
		return err
	}
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
	if addToExisting {
		ext.GiftReserveEarning = origEarning.Add(fromEarning)
		ext.GiftReserveMargin = origMargin.Add(fromMargin)
	} else {
		// Full reacquire after cancel/expire: net hold is 0 because Release
		// already wrote gift_release. Set the snapshot to this reservation
		// only; do not add it onto a stale pre-release snapshot.
		ext.GiftReserveEarning = fromEarning
		ext.GiftReserveMargin = fromMargin
	}
	switch {
	case ext.GiftReserveEarning.IsPositive() && ext.GiftReserveMargin.IsPositive():
		ext.GiftReserveSource = "earning+margin"
	case ext.GiftReserveMargin.IsPositive():
		ext.GiftReserveSource = "margin"
	default:
		ext.GiftReserveSource = "earning"
	}
	if !already && (fromEarning.IsPositive() || fromMargin.IsPositive()) {
		writeAdtyGiftOfficialAudit(ext.UserId, ext.SiteId, ext.TradeNo, "adty.gift.reserve", "reacquire",
			map[string]any{"earning": "0", "margin": "0"},
			map[string]any{
				"trade_no": ext.TradeNo, "site_id": ext.SiteId, "bonus_usd": ext.BonusUSD.String(),
				"source":  ext.GiftReserveSource,
				"earning": fromEarning.String(), "margin": fromMargin.String(),
				"phase": "reacquire",
			})
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

// adtyGiftFulfillmentResumeLimit caps one resume sweep. Pending rows are rare
// (one per order paid after cancel/expiry while the guarantee was short), so one
// bounded batch is enough per top-up; the next top-up picks up whatever is left.
const adtyGiftFulfillmentResumeLimit = 100

// ResumePendingAdtyGiftFulfillments 兑现该站点所有「待履约」赠金。
//
// 待履约（pending）是「客户已实付、站长承诺、但保障不足被扣下」的那部分赠金
// （ISS-R4-0076）。它此前没有出口：只有全额退款会把它转成 canceled，导致客户
// 付了钱、平台记着欠、却永远不会到账。站长补足保证金后保障就有了，这里按
// trade_no 重跑 reacquireAdtySiteGiftTx 取回保障，再把当时扣下的额度补进客户
// 的余额与批次，状态转 fulfilled。
//
// 保障仍然不足时整笔跳过：不写额度、不改状态、不写批次，留待下次补足。
// 每笔各起一个事务，一笔失败不影响同一站点的其它笔，也不会重复发放
// （pending → fulfilled 的迁移在行锁内只发生一次）。
func ResumePendingAdtyGiftFulfillments(siteID int) (int, error) {
	if siteID <= 0 || model.DB == nil {
		return 0, nil
	}
	var rows []model.AdtyGiftFulfillment
	if err := model.DB.Where("site_id = ? AND status = ?", siteID, model.AdtyGiftFulfillmentPending).
		Order("id asc").Limit(adtyGiftFulfillmentResumeLimit).Find(&rows).Error; err != nil {
		return 0, err
	}
	fulfilled := 0
	for i := range rows {
		done, err := fulfillPendingAdtyGiftFulfillment(rows[i].TradeNo)
		if err != nil {
			if !errors.Is(err, errGiftGuaranteeShort) && !errors.Is(err, ErrAdtySiteWalletInsufficient) {
				common.SysLog(common.LogText("gift fulfillment: resume %s: %s", rows[i].TradeNo, err.Error()))
			}
			continue
		}
		if done {
			fulfilled++
		}
	}
	return fulfilled, nil
}

// fulfillPendingAdtyGiftFulfillment 兑现一笔待履约赠金。返回 true 表示这一笔
// 本来 pending、这次兑现成功。保障不足时返回 errGiftGuaranteeShort，
// 调用方据此保持 pending。
func fulfillPendingAdtyGiftFulfillment(tradeNo string) (bool, error) {
	if tradeNo == "" || model.DB == nil {
		return false, nil
	}
	var notify model.AdtyGiftFulfillment
	var granted int64
	fulfilled := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtyGiftFulfillment
		if err := model.AdtyLockForUpdate(tx).Where("trade_no = ? AND status = ?", tradeNo, model.AdtyGiftFulfillmentPending).
			Limit(1).Find(&row).Error; err != nil {
			return err
		}
		if row.Id == 0 {
			// 已兑现或已终结：重放的扫描不能再发一次。
			return nil
		}
		var ext model.AdtyTopupOrder
		if err := model.AdtyLockForUpdate(tx).Where("trade_no = ? AND user_id = ?", tradeNo, row.UserId).
			Limit(1).Find(&ext).Error; err != nil {
			return err
		}
		if ext.Id == 0 || ext.SiteId != row.SiteId {
			return nil
		}
		// 本金已经全额退掉的单子不再补发：终结由退款钩子
		// （cancelPendingGiftFulfillmentOnFullRefundTx）负责，这里只兜底。
		// 部分退款不缩小这笔赠金——退款口径是「本金退、赠金不退」，赠金在实付时
		// 就已成立，扣下的那部分仍要兑现。
		if face := ext.FaceQuota(); face > 0 && ext.RefundQuota >= face {
			return nil
		}
		// 当时被扣下的正是「本单赠送额度 - 已发放额度」。pending 行记的 bonus
		// 额度是站长承诺额，本单赠送额度是实发上限，取小的那个才不会多发。
		owed := min(row.BonusQuota, topUpBonusQuota(&ext))
		if owed <= 0 {
			return nil
		}
		if err := reacquireAdtySiteGiftTx(tx, &ext); err != nil {
			return err
		}
		before, err := model.AdtyCreditUserQuotaTx(tx, row.UserId, owed)
		if err != nil {
			return err
		}
		if err := addAdtyGiftFulfilledBonusTx(tx, row.UserId, tradeNo, owed); err != nil {
			return err
		}
		key := "gift-fulfill:" + common.Sha1([]byte(tradeNo))
		if err := model.InsertAdtyLedgerTx(tx, &model.AdtyBalanceLedger{
			UserId: row.UserId, DeltaQuota: owed, BalanceBefore: before, BalanceAfter: before + owed,
			Type: model.AdtyLedgerTypeGiftFulfillment, RefType: "topup", RefId: tradeNo,
			Reason: "site gift fulfilled after guarantee top-up", IdempotencyKey: &key,
		}); err != nil {
			return err
		}
		if err := tx.Model(&model.AdtyTopupOrder{}).Where("id = ?", ext.Id).Updates(map[string]any{
			"gift_reserve_source":     ext.GiftReserveSource,
			"gift_reserve_earning":    ext.GiftReserveEarning,
			"gift_reserve_margin":     ext.GiftReserveMargin,
			"gift_fulfillment_status": model.AdtyGiftFulfillmentFulfilled,
		}).Error; err != nil {
			return err
		}
		history := adtyGiftFulfillmentHistory(row.History, map[string]any{
			"at": time.Now().Unix(), "action": "fulfilled", "quota": owed,
			"source": ext.GiftReserveSource, "earning": ext.GiftReserveEarning.String(), "margin": ext.GiftReserveMargin.String(),
		})
		if err := tx.Model(&model.AdtyGiftFulfillment{}).Where("id = ?", row.Id).Updates(map[string]any{
			"status": model.AdtyGiftFulfillmentFulfilled, "reason": "fulfilled after guarantee top-up", "history": history,
		}).Error; err != nil {
			return err
		}
		writeAdtyGiftOfficialAudit(row.UserId, row.SiteId, tradeNo, "adty.gift.fulfill",
			"guarantee top-up released the pending gift",
			map[string]any{"status": model.AdtyGiftFulfillmentPending, "quota": 0},
			map[string]any{
				"trade_no": tradeNo, "status": model.AdtyGiftFulfillmentFulfilled, "quota": owed,
				"source": ext.GiftReserveSource, "earning": ext.GiftReserveEarning.String(), "margin": ext.GiftReserveMargin.String(),
			})
		fulfilled = true
		granted = owed
		notify = row
		return nil
	})
	if err != nil {
		return false, err
	}
	if fulfilled {
		model.AdtySyncCreditUserQuotaCache(notify.UserId, granted, "gift fulfillment")
		adtyNotifyGiftFulfillmentFulfilled(&notify, granted)
	}
	return fulfilled, nil
}

// addAdtyGiftFulfilledBonusTx 把补发的赠金加回本单的 bonus 批次。
//
// 不能直接用 CreateAdtyFundBatchesTx：同一 (user, source) 已有批次时它是空操作，
// 补发的额度会从「Σ批次 == 余额」里漏掉，后续 gift_spend 按比例释放保障也就错了。
// 所以先按 source 把 original/remaining 一起加上；本单从未落过 bonus 批次
// （赠金被整笔扣下）时才新建。
func addAdtyGiftFulfilledBonusTx(tx *gorm.DB, userID int, source string, quota int64) error {
	if tx == nil || userID <= 0 || source == "" || quota <= 0 {
		return nil
	}
	res := tx.Model(&model.AdtyFundBatch{}).
		Where("user_id = ? AND source = ? AND kind = ?", userID, source, "bonus").
		Updates(map[string]any{
			"original":  gorm.Expr("original + ?", quota),
			"remaining": gorm.Expr("remaining + ?", quota),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	// 没有 bonus 批次：本单收钱时赠金被整笔扣下，当时只落了本金批次。
	// CreateAdtyFundBatchesTx 在这里不能用——它按 (user, source) 判重，
	// 本金批次在就算「已存在」直接返回，补发的额度会静默丢掉。
	return tx.Create(&model.AdtyFundBatch{
		UserId: userID, Kind: "bonus", Source: source,
		Original: quota, Remaining: quota, CreatedAt: common.GetTimestamp(),
	}).Error
}

// adtyGiftFulfillmentHistory appends one event to the row's JSON history.
func adtyGiftFulfillmentHistory(raw string, event map[string]any) string {
	body, err := json.Marshal(event)
	if err != nil {
		return raw
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return "[" + string(body) + "]"
	}
	if strings.HasSuffix(raw, "]") {
		return raw[:len(raw)-1] + "," + string(body) + "]"
	}
	return "[" + string(body) + "]"
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
	model.AdtyAfterBonusSpentTx = applySiteGiftSpendOnBonusTx
}

// releaseUnusedSiteGiftGuaranteeTx releases the unused portion of a paid
// order's site-gift guarantee back to the original earning and/or margin
// wallets. The unused amount follows the official reclaim: site gift ×
// min(G, R) ÷ G. A full reclaim clears any remaining net hold, including a
// hold re-acquired after cancel or expiry. Idempotent for one official refund.
func releaseUnusedSiteGiftGuaranteeTx(tx *gorm.DB, ext *model.AdtyTopupOrder) error {
	if tx == nil || ext == nil || ext.SiteId <= 0 {
		return nil
	}
	bonusQuota := ext.RefundBonusQuota
	if bonusQuota <= 0 {
		return nil
	}
	reclaimed := min(bonusQuota, max(ext.RefundDeductedQuota, 0))
	siteBonus := adtyBonusUSDForSite(ext)
	if !siteBonus.IsPositive() || reclaimed <= 0 {
		return nil
	}
	unusedUSD := siteBonus.Mul(decimal.NewFromInt(reclaimed)).Div(decimal.NewFromInt(bonusQuota)).RoundFloor(8)
	if reclaimed == bonusQuota {
		unusedUSD = siteBonus
	}
	heldEarning, heldMargin, err := netGiftHoldByKindTx(tx, ext.SiteId, ext.TradeNo)
	if err != nil {
		return err
	}
	held := heldEarning.Add(heldMargin)
	if !held.IsPositive() {
		return cancelPendingGiftFulfillmentOnFullRefundTx(tx, ext, reclaimed, bonusQuota)
	}
	toRelease := unusedUSD
	if toRelease.GreaterThan(held) || reclaimed == bonusQuota {
		toRelease = held
	}
	if !toRelease.IsPositive() {
		return cancelPendingGiftFulfillmentOnFullRefundTx(tx, ext, reclaimed, bonusQuota)
	}
	relEarning := decimal.Zero
	if held.IsPositive() && heldEarning.IsPositive() {
		relEarning = toRelease.Mul(heldEarning).Div(held).RoundFloor(8)
		if relEarning.GreaterThan(heldEarning) {
			relEarning = heldEarning
		}
	}
	relMargin := toRelease.Sub(relEarning)
	if relMargin.GreaterThan(heldMargin) {
		relMargin = heldMargin
		relEarning = toRelease.Sub(relMargin)
		if relEarning.GreaterThan(heldEarning) {
			relEarning = heldEarning
		}
	}
	key := "gift:" + ext.TradeNo
	already, err := adtyWalletIdempotencyExists(tx, key+":earn:refund-release", key+":margin:refund-release")
	if err != nil {
		return err
	}
	if relEarning.IsPositive() {
		if _, err := applyAdtySiteWalletChangeTx(tx, ext.SiteId, model.AdtySiteWalletEarnings, decimal.Zero, AdtySiteWalletChange{
			Type: "gift_release", RefType: "topup", RefID: ext.TradeNo, IdempotencyKey: key + ":earn:refund-release",
			HoldDelta: relEarning.Neg(),
		}); err != nil {
			return err
		}
	}
	if relMargin.IsPositive() {
		if _, err := applyAdtySiteWalletChangeTx(tx, ext.SiteId, model.AdtySiteWalletMargin, decimal.Zero, AdtySiteWalletChange{
			Type: "gift_release", RefType: "topup", RefID: ext.TradeNo, IdempotencyKey: key + ":margin:refund-release",
			HoldDelta: relMargin.Neg(),
		}); err != nil {
			return err
		}
	}
	if !already && (relEarning.IsPositive() || relMargin.IsPositive()) {
		writeAdtyGiftOfficialAudit(ext.UserId, ext.SiteId, ext.TradeNo, "adty.gift.release", "unused gift guarantee released on refund",
			map[string]any{"earning": heldEarning.String(), "margin": heldMargin.String()},
			map[string]any{
				"trade_no": ext.TradeNo, "earning": relEarning.String(), "margin": relMargin.String(),
				"phase": "refund",
			})
	}
	return cancelPendingGiftFulfillmentOnFullRefundTx(tx, ext, reclaimed, bonusQuota)
}

func netGiftHoldByKindTx(tx *gorm.DB, siteID int, tradeNo string) (earning, margin decimal.Decimal, err error) {
	var rows []model.AdtySiteWalletLedger
	if err = tx.Where("site_id = ? AND ref_type = ? AND ref_id = ? AND type IN ?",
		siteID, "topup", tradeNo, adtyGiftNetHoldTypes()).
		Find(&rows).Error; err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	for _, row := range rows {
		switch row.WalletKind {
		case model.AdtySiteWalletEarnings:
			earning = earning.Add(row.HoldDelta)
		case model.AdtySiteWalletMargin:
			margin = margin.Add(row.HoldDelta)
		}
	}
	if earning.IsNegative() {
		earning = decimal.Zero
	}
	if margin.IsNegative() {
		margin = decimal.Zero
	}
	return earning, margin, nil
}

func cancelPendingGiftFulfillmentOnFullRefundTx(tx *gorm.DB, ext *model.AdtyTopupOrder, reclaimed, bonusQuota int64) error {
	if ext == nil {
		return nil
	}
	fullPrincipal := ext.FaceQuota() > 0 && ext.RefundQuota >= ext.FaceQuota()
	fullGift := bonusQuota > 0 && reclaimed >= bonusQuota
	if !fullPrincipal && !fullGift {
		return nil
	}
	res := tx.Model(&model.AdtyGiftFulfillment{}).
		Where("trade_no = ? AND status = ?", ext.TradeNo, model.AdtyGiftFulfillmentPending).
		Updates(map[string]any{"status": model.AdtyGiftFulfillmentCanceled, "reason": "full refund cancelled pending fulfillment"})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}
	if err := tx.Model(&model.AdtyTopupOrder{}).Where("id = ?", ext.Id).Update("gift_fulfillment_status", model.AdtyGiftFulfillmentCanceled).Error; err != nil {
		return err
	}
	ext.GiftFulfillmentStatus = model.AdtyGiftFulfillmentCanceled
	return nil
}

func applySiteGiftSpendOnBonusTx(tx *gorm.DB, userID int, spends []model.AdtyBonusSpend) error {
	for _, spend := range spends {
		if err := applySiteGiftSpendForSourceTx(tx, userID, spend.Source, spend.Quota); err != nil {
			return err
		}
	}
	return nil
}

func applySiteGiftSpendForSourceTx(tx *gorm.DB, userID int, tradeNo string, takenQuota int64) error {
	if tx == nil || userID <= 0 || tradeNo == "" || takenQuota <= 0 {
		return nil
	}
	var ext model.AdtyTopupOrder
	if err := tx.Where("trade_no = ? AND user_id = ?", tradeNo, userID).Limit(1).Find(&ext).Error; err != nil {
		return err
	}
	bonus := adtyBonusUSDForSite(&ext)
	if ext.Id == 0 || ext.SiteId <= 0 || !bonus.IsPositive() {
		return nil
	}
	if ext.GiftFulfillmentStatus == model.AdtyGiftFulfillmentPending {
		return nil
	}
	heldEarning, heldMargin, err := netGiftHoldByKindTx(tx, ext.SiteId, ext.TradeNo)
	if err != nil {
		return err
	}
	held := heldEarning.Add(heldMargin)
	if !held.IsPositive() {
		return nil
	}
	siteQuota, err := siteGiftBonusQuota(&ext)
	if err != nil || siteQuota <= 0 {
		return err
	}
	var remaining int64
	if err := tx.Model(&model.AdtyFundBatch{}).
		Where("user_id = ? AND source = ? AND kind = ?", userID, tradeNo, "bonus").
		Select("COALESCE(SUM(remaining),0)").Scan(&remaining).Error; err != nil {
		return err
	}
	toSpend := bonus.Mul(decimal.NewFromInt(takenQuota)).Div(decimal.NewFromInt(siteQuota)).RoundFloor(8)
	if remaining <= 0 || toSpend.GreaterThan(held) {
		toSpend = held
	}
	if !toSpend.IsPositive() {
		return nil
	}
	relEarning := decimal.Zero
	if heldEarning.IsPositive() {
		relEarning = toSpend.Mul(heldEarning).Div(held).RoundFloor(8)
		if relEarning.GreaterThan(heldEarning) {
			relEarning = heldEarning
		}
	}
	relMargin := toSpend.Sub(relEarning)
	if relMargin.GreaterThan(heldMargin) {
		relMargin = heldMargin
		relEarning = toSpend.Sub(relMargin)
		if relEarning.GreaterThan(heldEarning) {
			relEarning = heldEarning
		}
	}
	var spentAfter int64
	if err := tx.Model(&model.AdtyFundBatch{}).
		Where("user_id = ? AND source = ? AND kind = ?", userID, tradeNo, "bonus").
		Select("COALESCE(SUM(original - remaining),0)").Scan(&spentAfter).Error; err != nil {
		return err
	}
	key := "gift:" + tradeNo + ":spend:" + strconv.FormatInt(spentAfter, 10)
	already, err := adtyWalletIdempotencyExists(tx, key+":earn", key+":margin")
	if err != nil {
		return err
	}
	if relEarning.IsPositive() {
		if _, err := applyAdtySiteWalletChangeTx(tx, ext.SiteId, model.AdtySiteWalletEarnings, relEarning.Neg(), AdtySiteWalletChange{
			Type: "gift_spend", RefType: "topup", RefID: tradeNo, IdempotencyKey: key + ":earn",
			HoldDelta: relEarning.Neg(),
		}); err != nil {
			return err
		}
	}
	if relMargin.IsPositive() {
		if _, err := applyAdtySiteWalletChangeTx(tx, ext.SiteId, model.AdtySiteWalletMargin, relMargin.Neg(), AdtySiteWalletChange{
			Type: "gift_spend", RefType: "topup", RefID: tradeNo, IdempotencyKey: key + ":margin",
			HoldDelta: relMargin.Neg(),
		}); err != nil {
			return err
		}
	}
	if !already && (relEarning.IsPositive() || relMargin.IsPositive()) {
		writeAdtyGiftOfficialAudit(userID, ext.SiteId, tradeNo, "adty.gift.spend", "",
			map[string]any{"earning": heldEarning.String(), "margin": heldMargin.String()},
			map[string]any{
				"trade_no": tradeNo, "earning": relEarning.String(), "margin": relMargin.String(),
				"taken_quota": takenQuota,
			})
	}
	return nil
}
