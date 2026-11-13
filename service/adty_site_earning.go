package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 站长收益记账（任务 11.5，执行层决定 25、26）。
// 客户付 P × k，平台收 P × d，站长收益 P × (k − d)。扣费额度 q = P × k × QuotaPerUnit，所以
//   客户付的钱 = q ÷ QuotaPerUnit
//   平台收入   = q × d ÷ k ÷ QuotaPerUnit
//   站长收益   = q × (k − d) ÷ k ÷ QuotaPerUnit
// 三项都用 decimal 算；站长收益用「客户付 − 平台收」得出，两项相加正好等于客户付的钱。
// 记账只累加当天仍待结算的槽位，钩子失败只记日志，不影响请求。

// adtySiteEarningDayFn 是记账用的日期（服务器时区），测试可以替换。
var adtySiteEarningDayFn = func() string { return time.Now().Format("2006-01-02") }

// AdtySiteEarningSplit 算一笔扣费的三项金额（美元，8 位小数）。k ≥ 1、0 < d ≤ 1 由调用方保证。
func AdtySiteEarningSplit(quota int64, k, d decimal.Decimal) (revenue, platform, earning decimal.Decimal) {
	perUnit := common.QuotaPerUnitDecimal()
	revenue = decimal.NewFromInt(quota).Div(perUnit).Round(8)
	platform = decimal.NewFromInt(quota).Mul(d).Div(k).Div(perUnit).Round(8)
	earning = revenue.Sub(platform)
	return revenue, platform, earning
}

// adtySiteEarningEntry 是一笔要累加的收益。
type adtySiteEarningEntry struct {
	SiteId, UserId  int
	SiteOwnerUserId int
	Kind, Reason    string
	Quota           int64
	Revenue         decimal.Decimal
	Platform        decimal.Decimal
	Earning         decimal.Decimal
}

// upsertAdtySiteEarningTx 把一笔收益累加进当天、同类型、还没结算的那一行。
// 已经结算或作废的行不再改：这时记进下一个槽位（Day 加 "+n"），结算按状态而不是按日期取行。
func upsertAdtySiteEarningTx(tx *gorm.DB, day string, entry adtySiteEarningEntry) error {
	return tx.Transaction(func(tx *gorm.DB) error {
		ownerID, err := model.ResolveAdtySiteFinancialOwnerTx(tx, entry.SiteId, entry.SiteOwnerUserId)
		if err != nil {
			return err
		}
		entry.SiteOwnerUserId = ownerID
		now := time.Now().Unix()
		for attempt := range 8 {
			slot := day
			if attempt > 0 {
				slot = fmt.Sprintf("%s+%d", day, attempt)
			}
			row := model.AdtySiteEarningDaily{
				SiteId: entry.SiteId, SiteOwnerUserId: ownerID, UserId: entry.UserId, Day: slot, Kind: entry.Kind, Reason: entry.Reason,
				Revenue: decimal.Zero, Platform: decimal.Zero, Earning: decimal.Zero,
				Status: model.AdtySiteEarningPending, UpdatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			// MySQL's ON DUPLICATE KEY UPDATE ignores OnConflict.Where. A
			// conditional UPDATE keeps settled rows immutable on every engine.
			result := tx.Model(&model.AdtySiteEarningDaily{}).
				Where("site_id = ? AND site_owner_user_id = ? AND user_id = ? AND day = ? AND kind = ? AND status = ?", entry.SiteId, ownerID, entry.UserId, slot, entry.Kind, model.AdtySiteEarningPending).
				Updates(map[string]any{
					"quota":      gorm.Expr("quota + ?", entry.Quota),
					"revenue":    gorm.Expr("revenue + ?", entry.Revenue),
					"platform":   gorm.Expr("platform + ?", entry.Platform),
					"earning":    gorm.Expr("earning + ?", entry.Earning),
					"updated_at": now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				return nil
			}
			// MySQL reports changed rows by default. Adding an exact zero entry
			// to a new zero slot may match it without changing any column.
			var pending int64
			if err := tx.Model(&model.AdtySiteEarningDaily{}).
				Where("site_id = ? AND site_owner_user_id = ? AND user_id = ? AND day = ? AND kind = ? AND status = ?", entry.SiteId, ownerID, entry.UserId, slot, entry.Kind, model.AdtySiteEarningPending).
				Count(&pending).Error; err != nil {
				return err
			}
			if pending > 0 {
				return nil
			}
		}
		return errors.New("no open earning slot for today")
	})
}

// adtySiteCustomerRisk 取客户的风控状态：有标记并且没复核过时，收益记进 skipped。
func adtySiteCustomerRisk(userId int) (flag string) {
	var rows []model.AdtySiteCustomer
	if err := model.DB.Select("user_id", "risk_flag", "reviewed").Where("user_id = ?", userId).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return ""
	}
	if rows[0].Reviewed {
		return ""
	}
	return rows[0].RiskFlag
}

// AdtySiteEarningHook 给一笔消费或退款记站长收益：quota 是客户的扣费额度（退款传正数，kind 用 refund），
// k、d 是扣费当时的快照。客户不是分站客户或没有 k 时直接返回。
func AdtySiteEarningHook(siteId, userId int, quota int64, k, d float64, kind string, frozenOwnerIDs ...int) {
	if siteId <= 0 || userId <= 0 || quota == 0 || k < 1 || d <= 0 || d > 1 || math.IsNaN(k) || math.IsInf(k, 0) {
		return
	}
	revenue, platform, earning := AdtySiteEarningSplit(quota, decimal.NewFromFloat(k), decimal.NewFromFloat(d))
	entry := adtySiteEarningEntry{SiteId: siteId, UserId: userId, Kind: kind, Quota: quota, Revenue: revenue, Platform: platform, Earning: earning}
	if kind == model.AdtySiteEarningRefund {
		entry.Quota, entry.Revenue, entry.Platform, entry.Earning = -quota, revenue.Neg(), platform.Neg(), earning.Neg()
	}
	if flag := adtySiteCustomerRisk(userId); flag != "" {
		// 疑似自用或主站迁移：照常扣费，收益照算，但记进 skipped，不结算（复核后可以改回 usage）。
		entry.Kind, entry.Reason = model.AdtySiteEarningSkipped, flag
	}
	var err error
	if len(frozenOwnerIDs) > 0 {
		entry.SiteOwnerUserId = frozenOwnerIDs[0]
		if entry.SiteOwnerUserId <= 0 {
			err = errors.New("site financial owner snapshot unavailable")
		}
	} else if kind == model.AdtySiteEarningUsage {
		entry.SiteOwnerUserId, err = model.CurrentAdtySiteFinancialOwnerTx(model.DB, siteId)
	}
	if err == nil {
		err = upsertAdtySiteEarningTx(model.DB, adtySiteEarningDayFn(), entry)
	}
	if err != nil {
		common.SysError(common.LogText("record site earning failed site=%d user=%d: %s", siteId, userId, err.Error()))
		if incErr := model.DB.Model(&model.AdtySite{}).Where("id = ?", siteId).
			UpdateColumn("earning_hook_failures", gorm.Expr("earning_hook_failures + 1")).Error; incErr != nil {
			common.SysError(common.LogText("count site earning failure site=%d: %s", siteId, incErr.Error()))
		}
	}
}

// adtyRecordConsumeEarning 是 RecordConsumeLog 的钩子：从 gin 上下文取 11.4 写进去的 k、d。
// 渠道测试（controller/channel-test.go）不经过 HandleGroupRatio 的乘 k，上下文里没有 k，所以不记。
func adtyRecordConsumeEarning(c *gin.Context, userId int, params *model.RecordConsumeLogParams) {
	if c.GetBool(adtyDurableBillingContextKey) {
		return
	}
	k, d, ok := AdtySiteMarkupFromContext(c)
	if !ok {
		return
	}
	// 余额下限：收益按实扣记，日志额度减去平台承担的差额。
	charged := int64(params.Quota) - adtyLoggedShortfall(params.Other)
	if charged <= 0 {
		return
	}
	AdtySiteEarningHook(c.GetInt(AdtySiteIdContextKey), userId, charged, k, d, model.AdtySiteEarningUsage, c.GetInt(AdtySiteOwnerContextKey))
}

// adtyAppendTaskSiteAdminInfo 把任务快照里的 k、d、站点写进日志的 admin_info（taskBillingOther 调用），
// RecordTaskBillingLog 的钩子从那里读。普通用户的日志投影看不到 admin_info。
func adtyAppendTaskSiteAdminInfo(bc *model.TaskBillingContext, other *model.LogOther) {
	if bc == nil || other == nil || bc.SiteMarkup < 1 || bc.SiteCostFactor <= 0 {
		return
	}
	other.SetAdmin("site_markup", bc.SiteMarkup)
	other.SetAdmin("site_cost_factor", bc.SiteCostFactor)
	other.SetAdmin("site_id", bc.SiteId)
	other.SetAdmin("site_owner_user_id", bc.SiteOwnerUserId)
}

// adtyRecordTaskBillingEarning 是 RecordTaskBillingLog 的钩子：任务差额结算（consume）记 usage，
// 退款和差额为负（refund）按快照的 k、d 记负收益。
// MJ 退款把提交时写入任务的 k、d 放进 admin_info。没有这份快照的旧任务才回退到当前倍率。
func adtyRecordTaskBillingEarning(params *model.RecordTaskBillingLogParams) {
	if params == nil || params.Quota <= 0 {
		return
	}
	kind := model.AdtySiteEarningUsage
	switch params.LogType {
	case model.LogTypeConsume:
	case model.LogTypeRefund:
		kind = model.AdtySiteEarningRefund
	default:
		return
	}
	snapshot := params.Other.Snapshot()
	adminInfo, _ := snapshot["admin_info"].(map[string]any)
	if adminInfo["billing_operation_key"] != nil {
		return
	}
	k, _ := adminInfo["site_markup"].(float64)
	d, _ := adminInfo["site_cost_factor"].(float64)
	siteId, _ := adminInfo["site_id"].(int)
	if k < 1 || d <= 0 {
		if _, isTask := snapshot["is_task"]; isTask {
			return // 主站客户的任务
		}
		site := AdtyBillingSiteForUser(params.UserId)
		if site == nil {
			return
		}
		cost, ok := AdtySiteCostFactor(site)
		if !ok {
			return
		}
		siteId, d = site.Id, cost.InexactFloat64()
		k = adtyMarkupFloat(AdtySiteMarkup(params.UserId, params.Group), params.UserId, params.Group)
	}
	// 余额下限：差额结算的收益按实扣记（只有补扣会有差额，退款日志里没有这个键）。
	quota := int64(params.Quota) - adtyLoggedShortfall(params.Other)
	if quota <= 0 {
		return
	}
	ownerID, err := model.ResolveAdtySiteFinancialOwnerTx(model.DB, siteId, adtyAnyInt(adminInfo["site_owner_user_id"]))
	if err != nil {
		common.SysError("Task earning owner snapshot unavailable")
		return
	}
	AdtySiteEarningHook(siteId, params.UserId, quota, k, d, kind, ownerID)
}

// adtyBonusUSDForSite 是一笔订单要由站长出的加赠（美元）：站点档位的加赠，加上站长出资优惠码的优惠加赠（12.4）。
// 平台出资的优惠加赠不算；不是分站订单返回 0。入账时码已用完被去掉的优惠加赠，扩展行里已经是 0。
func adtyBonusUSDForSite(ext *model.AdtyTopupOrder) decimal.Decimal {
	if ext == nil || ext.SiteId <= 0 {
		return decimal.Zero
	}
	bonus := decimal.Max(ext.BonusUSD, decimal.Zero)
	if ext.PromoFunder == model.AdtyPromoFunderSiteOwner && ext.PromoBonusUSD.IsPositive() {
		bonus = bonus.Add(ext.PromoBonusUSD)
	}
	return bonus
}

// adtySiteBonusWithinCap：一笔分站充值的加赠合计（站点档位加赠 + 站长优惠加赠）不超过
// 面额 ×（主站同档比例 + max_site_bonus_extra）÷ 100。max_site_bonus_extra 没设按 0。
func adtySiteBonusCapAmount(face int64) decimal.Decimal {
	extra := decimal.Zero
	if value := resellerSetting().MaxSiteBonusExtra; value != nil {
		extra = decimal.NewFromFloat(*value)
	}
	mainRate, _ := AdtyBonusFor(face)
	return decimal.NewFromInt(face).Mul(mainRate.Add(extra)).Div(decimal.NewFromInt(100)).Round(8)
}

func adtySiteBonusWithinCap(face int64, totalBonus decimal.Decimal) bool {
	return !totalBonus.GreaterThan(adtySiteBonusCapAmount(face))
}

// adtySiteBonusAfterCreditTx：分站客户充值入账时，加赠按面值从站长收益里扣（记一笔负数，执行层决定 26）。
// 在入账事务里，只用 OnConflict DoUpdate 写，不做可能失败的查询（审阅重点 10）。
func adtySiteBonusAfterCreditTx(tx *gorm.DB, topUp *model.TopUp, ext *model.AdtyTopupOrder) error {
	bonus := adtyBonusUSDForSite(ext)
	if bonus.IsZero() {
		return nil
	}
	return upsertAdtySiteEarningTx(tx, adtySiteEarningDayFn(), adtySiteEarningEntry{
		SiteId: ext.SiteId, SiteOwnerUserId: ext.SiteOwnerUserId, UserId: topUp.UserId, Kind: model.AdtySiteEarningBonus, Earning: bonus.Neg(),
	})
}

// adtySiteBonusAfterRefundTx：这笔充值退款时赠金不退给客户（只退没花掉的本金）。按「先花本金」，收回的赠金是
// min(G, R)，客户已经花掉的赠金收不回来，所以站长出的加赠只按收回的比例还给站长：
// 站长出资的加赠 × min(G, R) ÷ G，记一笔 bonus_reversal（正数）。平台优惠码出的赠金收回了不还给任何人
// （adtyBonusUSDForSite 只算站长出的部分）。站长的消费分成不扣：充值本身不产生站长收益，收益来自消费，
// 客户已经花掉的额度是真实消费。
func adtySiteBonusAfterRefundTx(tx *gorm.DB, topUp *model.TopUp, ext *model.AdtyTopupOrder) error {
	bonus := adtyBonusUSDForSite(ext)
	if bonus.IsZero() || ext.RefundBonusQuota <= 0 {
		return nil
	}
	reclaimed := min(ext.RefundBonusQuota, max(ext.RefundDeductedQuota, 0))
	returned := bonus.Mul(decimal.NewFromInt(reclaimed)).Div(decimal.NewFromInt(ext.RefundBonusQuota)).RoundFloor(8)
	if !returned.IsPositive() {
		return nil
	}
	return upsertAdtySiteEarningTx(tx, adtySiteEarningDayFn(), adtySiteEarningEntry{
		SiteId: ext.SiteId, SiteOwnerUserId: ext.SiteOwnerUserId, UserId: topUp.UserId, Kind: model.AdtySiteEarningBonusReversal, Earning: returned,
	})
}

// AdtySiteBonusRates 是站点的加赠档位（站长设的 JSON：档位美元 → 百分比）；空就不送。
func AdtySiteBonusRates(site *model.AdtySite) map[int]float64 {
	rates := map[int]float64{}
	if site == nil || strings.TrimSpace(site.BonusRates) == "" {
		return rates
	}
	if err := common.UnmarshalJsonStr(site.BonusRates, &rates); err != nil {
		common.SysError(common.LogText("invalid site bonus rates site=%d: %s", site.Id, err.Error()))
		return map[int]float64{}
	}
	return rates
}

// ValidateAdtySiteBonusRates 检查站长要保存的加赠档位：每一档不能超过「主站同档比例 + max_site_bonus_extra」。
// 主站同档按 AdtyBonusFor 取（不超过这个面额的最高档）。max_site_bonus_extra 没设就是 0。
func ValidateAdtySiteBonusRates(rates map[int]float64) error {
	extra := 0.0
	if value := resellerSetting().MaxSiteBonusExtra; value != nil {
		extra = *value
	}
	for tier, rate := range rates {
		if tier <= 0 || rate < 0 || rate > 100 || math.IsNaN(rate) {
			return common.NewMessage("Bonus rate for {{tier}} must be between 0 and 100", map[string]any{"tier": tier})
		}
		mainRate, _ := AdtyBonusFor(int64(tier))
		limit := mainRate.Add(decimal.NewFromFloat(extra))
		if decimal.NewFromFloat(rate).GreaterThan(limit) {
			return common.NewMessage("The bonus for ${{tier}} can be at most {{max}}%", map[string]any{"tier": tier, "max": limit.String()})
		}
	}
	return nil
}

// ErrAdtyAmountTierRetired is returned when a caller tries to save the retired amount-tier gift.
var ErrAdtyAmountTierRetired = common.NewMessage("Amount-tier bonuses are retired")

// SaveAdtySiteBonusRates rejects new amount-tier gift tables. Payment-method rates are the live rule.
func SaveAdtySiteBonusRates(siteId int, rates map[int]float64) error {
	return ErrAdtyAmountTierRetired
}

// ErrAdtySiteTrialUnavailable 是试用额度没设上限时的错误（约定 I）。
var ErrAdtySiteTrialUnavailable = common.NewMessage("Trial credit is not available")

// GrantAdtySiteTrial 站长给自己站的客户送试用额度（11.5）：当场从站长可提现扣除，给客户加额度，
// trial 行直接记成已结算，结算任务不再取它。可提现不够就拒绝。幂等键重放直接返回上次的发放。
func GrantAdtySiteTrial(siteId, customerId int, amountUSD decimal.Decimal, idempotencyKey string) (*model.AdtySiteTrialGrant, error) {
	cfg := resellerSetting()
	if cfg.TrialGrantMaxUSD == nil || cfg.TrialGrantDailyUSD == nil {
		return nil, ErrAdtySiteTrialUnavailable
	}
	amount := amountUSD.Round(2)
	if !amount.IsPositive() {
		return nil, common.NewMessage("Enter an amount")
	}
	if amount.GreaterThan(decimal.NewFromFloat(*cfg.TrialGrantMaxUSD)) {
		return nil, common.NewMessage("A single trial credit can be at most ${{amount}}", map[string]any{"amount": *cfg.TrialGrantMaxUSD})
	}
	key := strings.TrimSpace(idempotencyKey)
	if key == "" || len(key) > 100 {
		return nil, common.NewMessage("Invalid idempotency key")
	}
	key = fmt.Sprintf("site-trial:%d:%s", siteId, key)
	quota := amount.Mul(common.QuotaPerUnitDecimal()).Floor()
	quotaInt, err := common.WalletQuotaFromDecimalStrict(quota)
	if err != nil {
		return nil, err
	}
	day := adtySiteEarningDayFn()
	var grant model.AdtySiteTrialGrant
	replay := false
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var existing []model.AdtySiteTrialGrant
		if err := tx.Where("idempotency_key = ?", key).Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > 0 {
			if existing[0].UserId != customerId || !existing[0].AmountUSD.Equal(amount) {
				return model.ErrAdtyIdempotencyConflict
			}
			grant, replay = existing[0], true
			return nil
		}
		var site model.AdtySite
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteId).First(&site).Error; err != nil {
			return err
		}
		if site.Status != model.AdtySiteStatusActive {
			return ErrAdtySitePaused
		}
		var customers []model.User
		if err := tx.Select("id", "site_id").Where("id = ? AND site_id = ?", customerId, siteId).Limit(1).Find(&customers).Error; err != nil {
			return err
		}
		if len(customers) == 0 {
			return common.NewMessage("Customer not found")
		}
		account, err := adtyOwnerAccountTx(tx, site.OwnerUserId)
		if err != nil {
			return err
		}
		var granted struct{ Total decimal.Decimal }
		if err := tx.Model(&model.AdtySiteTrialGrant{}).Select("COALESCE(SUM(amount_usd),0) AS total").Where("site_id = ? AND day = ?", siteId, day).Scan(&granted).Error; err != nil {
			return err
		}
		if granted.Total.Add(amount).GreaterThan(decimal.NewFromFloat(*cfg.TrialGrantDailyUSD)) {
			return common.NewMessage("Today's trial credit limit of ${{amount}} has been reached", map[string]any{"amount": *cfg.TrialGrantDailyUSD})
		}
		debited := tx.Model(&model.ReferralCommissionAccount{}).
			Where("id = ? AND available_amount >= ?", account.Id, moneyF(amount)).
			Update("available_amount", gorm.Expr("available_amount - ?", moneyF(amount)))
		if debited.Error != nil {
			return debited.Error
		}
		if debited.RowsAffected == 0 {
			return common.NewMessage("Not enough withdrawable commission")
		}
		if err := addReferralLedgerTx(tx, &model.ReferralCommissionLedger{
			AffiliateId: account.AffiliateId, UserId: account.UserId,
			DeltaAvailable: moneyF(amount.Neg()), Type: "site_trial",
			RefType: "site_trial", RefId: fmt.Sprint(siteId),
			ExternalRefId: key, Remark: "trial credit paid by the site owner",
		}); err != nil {
			return err
		}
		grant = model.AdtySiteTrialGrant{SiteId: siteId, SiteOwnerUserId: site.OwnerUserId, UserId: customerId, AmountUSD: amount, Quota: int64(quotaInt), Day: day, IdempotencyKey: key}
		if err := tx.Create(&grant).Error; err != nil {
			return err
		}
		before, err := model.AdtyCreditUserQuotaTx(tx, customerId, int64(quotaInt))
		if err != nil {
			return err
		}
		ref := fmt.Sprintf("site-trial-%d", grant.Id)
		if err := model.InsertAdtyLedgerTx(tx, &model.AdtyBalanceLedger{
			UserId: customerId, DeltaQuota: int64(quotaInt), BalanceBefore: before, BalanceAfter: before + int64(quotaInt),
			Type: model.AdtyLedgerTypeSiteTrial, RefType: "site_trial", RefId: ref, Reason: "trial credit from the site owner",
			OperatorId: site.OwnerUserId, IdempotencyKey: &ref,
		}); err != nil {
			return err
		}
		now := time.Now().Unix()
		updated := tx.Model(&model.AdtySiteEarningDaily{}).
			Where("site_id = ? AND site_owner_user_id = ? AND user_id = ? AND day = ? AND kind = ? AND status = ?", siteId, site.OwnerUserId, customerId, day, model.AdtySiteEarningTrial, model.AdtySiteEarningAvailable).
			Update("earning", gorm.Expr("earning + ?", amount.Neg()))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected > 0 {
			return nil
		}
		return tx.Create(&model.AdtySiteEarningDaily{
			SiteId: siteId, SiteOwnerUserId: site.OwnerUserId, UserId: customerId, Day: day, Kind: model.AdtySiteEarningTrial,
			Earning: amount.Neg(), Status: model.AdtySiteEarningAvailable, SettledAt: now, UpdatedAt: now,
		}).Error
	})
	if err != nil {
		if adtyUniqueViolation(err) {
			return replayAdtySiteTrial(key, customerId, amount)
		}
		return nil, err
	}
	if !replay {
		model.AdtySyncCreditUserQuotaCache(customerId, int64(quotaInt), "site trial credit")
	}
	return &grant, nil
}

func replayAdtySiteTrial(key string, customerId int, amount decimal.Decimal) (*model.AdtySiteTrialGrant, error) {
	var existing []model.AdtySiteTrialGrant
	if err := model.DB.Where("idempotency_key = ?", key).Limit(1).Find(&existing).Error; err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		return nil, model.ErrAdtyIdempotencyConflict
	}
	if existing[0].UserId != customerId || !existing[0].AmountUSD.Equal(amount) {
		return nil, model.ErrAdtyIdempotencyConflict
	}
	return &existing[0], nil
}

func adtyUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "23505") || strings.Contains(text, "duplicate key")
}

// ReviewAdtySiteCustomer 管理员复核风控客户（分站·审核）：认可后之后的消费正常计收益；
// convertSkipped 时把还没结算的 skipped 行改成 usage。
func ReviewAdtySiteCustomer(userId, adminId int, convertSkipped bool) error {
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var customer model.AdtySiteCustomer
		if err := model.AdtyLockForUpdate(tx).Where("user_id = ?", userId).First(&customer).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", userId).Update("reviewed", true).Error; err != nil {
			return err
		}
		if convertSkipped {
			if err := tx.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", userId).Update("earning_risk_released", true).Error; err != nil {
				return err
			}
			var events []model.AdtySiteEarningEvent
			if err := model.AdtyLockForUpdate(tx).Where("user_id = ? AND site_id = ? AND risk_flag <> ? AND (risk_released = ? OR risk_released IS NULL) AND (review_status = ? OR review_status IS NULL)",
				userId, customer.SiteId, "", false, "").Find(&events).Error; err != nil {
				return err
			}
			for _, event := range events {
				if event.Projected {
					revenue, platform, earning := AdtySiteEarningSplit(int64(event.Quota), decimal.NewFromFloat(event.Markup), decimal.NewFromFloat(event.CostFactor))
					kind := model.AdtySiteEarningUsage
					if event.Quota < 0 {
						kind = model.AdtySiteEarningRefund
					}
					if err := upsertAdtySiteEarningTx(tx, event.Day+"+op", adtySiteEarningEntry{SiteId: event.SiteId, SiteOwnerUserId: event.SiteOwnerUserId, UserId: userId,
						Kind: kind, Quota: int64(event.Quota), Revenue: revenue, Platform: platform, Earning: earning}); err != nil {
						return err
					}
					if err := upsertAdtySiteEarningTx(tx, event.Day+"+op", adtySiteEarningEntry{SiteId: event.SiteId, SiteOwnerUserId: event.SiteOwnerUserId, UserId: userId,
						Kind: model.AdtySiteEarningSkipped, Reason: event.RiskFlag, Quota: -int64(event.Quota), Revenue: revenue.Neg(), Platform: platform.Neg(), Earning: earning.Neg()}); err != nil {
						return err
					}
				}
				if err := tx.Model(&event).Update("risk_released", true).Error; err != nil {
					return err
				}
			}
			var rows []model.AdtySiteEarningDaily
			if err := tx.Where("user_id = ? AND site_id = ? AND kind = ? AND status = ? AND day NOT LIKE '%+op%'", userId, customer.SiteId,
				model.AdtySiteEarningSkipped, model.AdtySiteEarningPending).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if err := upsertAdtySiteEarningTx(tx, row.Day, adtySiteEarningEntry{
					SiteId: row.SiteId, SiteOwnerUserId: row.SiteOwnerUserId, UserId: row.UserId, Kind: model.AdtySiteEarningUsage,
					Quota: row.Quota, Revenue: row.Revenue, Platform: row.Platform, Earning: row.Earning,
				}); err != nil {
					return err
				}
				if err := tx.Where("id = ?", row.Id).Delete(&model.AdtySiteEarningDaily{}).Error; err != nil {
					return err
				}
			}
		}
		return recordSiteAuditTx(tx, customer.SiteId, "customer_review", adminId, map[string]any{"user_id": userId, "risk_flag": customer.RiskFlag},
			map[string]any{"reviewed": true, "convert_skipped": convertSkipped}, "")
	})
	return err
}

func init() {
	AdtyRecordConsumeEarning = adtyRecordConsumeEarning
	model.AdtyOnTaskBillingLog = adtyRecordTaskBillingEarning
}
