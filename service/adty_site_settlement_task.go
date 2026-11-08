package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 站长收益结算（任务 11.5）：主节点每天 settle_hour 点跑一次。
// pending 且不是 skipped、trial 的行：负数下一次就入账；正数要满 freeze_days。
// 汇总后加进站长佣金账户（可以为负），写返佣流水 site_earning，行改成 available。
// 站点 paused 或对账暂停时只结负数。

// adtySettleCutoffDay 是能结算的最后一天：今天往前 freeze_days 天（至少是昨天）。
func adtySettleCutoffDay(now time.Time) string {
	days := max(resellerSetting().FreezeDays, 0)
	return now.AddDate(0, 0, -max(days, 1)).Format("2006-01-02")
}

// adtyEarningDayOf 去掉槽位后缀（"2026-10-31+1" → "2026-10-31"）。
func adtyEarningDayOf(day string) string {
	base, _, _ := strings.Cut(day, "+")
	return base
}

// RunAdtySiteSettlementOnce 结算到期的站长收益，返回结算了几个站。
func RunAdtySiteSettlementOnce(now time.Time) (int, error) {
	var events []model.AdtySiteEarningEvent
	if err := model.DB.Where("projected = ? AND (review_status <> ? OR review_status IS NULL)", false, model.AdtyEarningNeedsReview).Order("created_at").Limit(1000).Find(&events).Error; err != nil {
		return 0, err
	}
	for _, event := range events {
		if err := projectAdtySiteEarningEvent(context.Background(), event.OperationKey); err != nil {
			return 0, err
		}
	}
	cutoff := adtySettleCutoffDay(now)
	var siteIds []int
	if err := model.DB.Model(&model.AdtySiteEarningDaily{}).Distinct("site_id").
		Where("status = ? AND kind NOT IN ? AND (earning < 0 OR day <= ?)",
			model.AdtySiteEarningPending, []string{model.AdtySiteEarningSkipped, model.AdtySiteEarningTrial}, cutoff+"+~").
		Pluck("site_id", &siteIds).Error; err != nil {
		return 0, err
	}
	// 钩子失败可能一行都没写上。这种站也要进对账，否则漏记的正收益不会被拦住。
	var failedSites []int
	if err := model.DB.Model(&model.AdtySite{}).Where("earning_hook_failures > 0").Pluck("id", &failedSites).Error; err != nil {
		return 0, err
	}
	seen := make(map[int]struct{}, len(siteIds))
	for _, siteId := range siteIds {
		seen[siteId] = struct{}{}
	}
	for _, siteId := range failedSites {
		if _, ok := seen[siteId]; !ok {
			siteIds = append(siteIds, siteId)
		}
	}
	settled := 0
	var first error
	for _, siteId := range siteIds {
		ok, err := settleAdtySite(siteId, cutoff, now.Unix())
		if err != nil {
			common.SysError(common.LogText("settle site earnings failed site=%d: %s", siteId, err.Error()))
			if first == nil {
				first = err
			}
			continue
		}
		if ok {
			settled++
		}
	}
	return settled, first
}

// settleAdtySite 结算一个站：锁站点和佣金账户，汇总到期的行。
func settleAdtySite(siteId int, cutoff string, now int64) (bool, error) {
	settled := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var site model.AdtySite
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteId).First(&site).Error; err != nil {
			return err
		}
		var unresolved int64
		if err := tx.Model(&model.AdtySiteEarningEvent{}).Where("site_id = ? AND (review_status = ? OR projected = ?)", siteId, model.AdtyEarningNeedsReview, false).Count(&unresolved).Error; err != nil {
			return err
		}
		if unresolved == 0 && site.SettleHoldReason == model.AdtyEarningReviewHoldReason {
			if err := tx.Model(&site).Update("settle_hold_reason", "").Error; err != nil {
				return err
			}
			site.SettleHoldReason = ""
		}
		holdPositive := site.Status == model.AdtySiteStatusPaused || site.Status == model.AdtySiteStatusPending || strings.TrimSpace(site.SettleHoldReason) != "" || unresolved > 0
		if !holdPositive {
			mismatch, reason, err := adtyUsageReconcileMismatch(tx, site.Id, cutoff)
			if err != nil {
				return err
			}
			if mismatch {
				if err := tx.Model(&site).Update("settle_hold_reason", reason).Error; err != nil {
					return err
				}
				holdPositive = true
			}
		}
		var rows []model.AdtySiteEarningDaily
		if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND status = ? AND kind NOT IN ?",
			siteId, model.AdtySiteEarningPending, []string{model.AdtySiteEarningSkipped, model.AdtySiteEarningTrial}).
			Find(&rows).Error; err != nil {
			return err
		}
		total := decimal.Zero
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			negative := row.Earning.IsNegative()
			// 负数不等冻结期。正数要过了 cutoff 才结。
			if !negative && adtyEarningDayOf(row.Day) > cutoff {
				continue
			}
			// 暂停、待审或对账暂停时只结负收益，正收益留着等恢复。
			if holdPositive && !negative {
				continue
			}
			total = total.Add(row.Earning)
			ids = append(ids, row.Id)
		}
		if len(ids) == 0 {
			return nil
		}
		account, err := adtyOwnerAccountTx(tx, site.OwnerUserId)
		if err != nil {
			return err
		}
		if !total.IsZero() {
			// A later callback/review may produce another batch on the same
			// cutoff. Its rows, rather than its date or amount, identify it.
			slices.Sort(ids)
			batchKey := fmt.Sprintf("site-earnings:%d:%x", site.Id, sha256.Sum256([]byte(fmt.Sprint(ids))))
			if _, err := applyAdtySiteWalletChangeTx(tx, site.Id, model.AdtySiteWalletEarnings, total, AdtySiteWalletChange{
				Type: "site_earning_settlement", RefType: "site_earning", RefID: cutoff,
				Reason: fmt.Sprintf("site earnings up to %s", cutoff), IdempotencyKey: batchKey, AllowOverdraft: true,
			}); err != nil {
				return err
			}
		}
		if err := AdjustAccountTx(tx, account, decimal.Zero, total, decimal.Zero, decimal.Zero, model.ReferralCommissionLedger{
			Type: "site_earning", RefType: "site", RefId: fmt.Sprint(site.Id),
			ExternalRefId: fmt.Sprintf("site-earning:%d:%s:%d", site.Id, cutoff, now), Remark: fmt.Sprintf("site earnings up to %s", cutoff),
		}); err != nil {
			return err
		}
		if err := tx.Model(&model.AdtySiteEarningDaily{}).Where("id IN ?", ids).Updates(map[string]any{
			"status": model.AdtySiteEarningAvailable, "settled_at": now,
		}).Error; err != nil {
			return err
		}
		settled = true
		return nil
	})
	return settled, err
}

// adtyUsageReconcileMismatch 用到期日之前的消费日志重算 usage 收益。没有带 site_id 的日志时不暂停
// （消费日志关掉时收益仍可能记上）。日志和已记账的 usage 对不上才暂停正收益。
func adtyUsageReconcileMismatch(tx *gorm.DB, siteId int, cutoff string) (bool, string, error) {
	var events []model.AdtySiteEarningEvent
	if err := tx.Where("site_id = ? AND day <= ?", siteId, cutoff).Find(&events).Error; err != nil {
		return false, "", err
	}
	if len(events) > 0 {
		expected := decimal.Zero
		for _, event := range events {
			if event.ReviewStatus == model.AdtyEarningNeedsReview {
				continue
			}
			_, _, earning := AdtySiteEarningSplit(int64(event.Quota), decimal.NewFromFloat(event.Markup), decimal.NewFromFloat(event.CostFactor))
			expected = expected.Add(earning)
		}
		var recorded struct{ Total decimal.Decimal }
		if err := tx.Model(&model.AdtySiteEarningDaily{}).Select("COALESCE(SUM(earning),0) AS total").
			Where("site_id = ? AND day LIKE ? AND day <= ? AND kind IN ? AND status IN ?", siteId, "%+op%", cutoff+"+~",
				[]string{model.AdtySiteEarningUsage, model.AdtySiteEarningRefund, model.AdtySiteEarningSkipped},
				[]string{model.AdtySiteEarningPending, model.AdtySiteEarningAvailable}).Scan(&recorded).Error; err != nil {
			return false, "", err
		}
		if !expected.Round(8).Equal(recorded.Total.Round(8)) {
			return true, fmt.Sprintf("usage earnings %s do not match durable events %s", recorded.Total.StringFixed(8), expected.StringFixed(8)), nil
		}
	}
	if model.LOG_DB == nil {
		return false, "", nil
	}
	end, err := time.ParseInLocation("2006-01-02", cutoff, time.Local)
	if err != nil {
		return false, "", err
	}
	end = end.AddDate(0, 0, 1)
	var logs []model.Log
	if err := model.LOG_DB.Where("type IN ? AND created_at < ? AND other LIKE ?", []int{model.LogTypeConsume, model.LogTypeRefund}, end.Unix(), `%"site_id"%`).Find(&logs).Error; err != nil {
		return false, "", err
	}
	logTotal := decimal.Zero
	hadLogs := false
	for _, row := range logs {
		earning, ok := adtyLogUsageEarning(row, siteId)
		if !ok {
			continue
		}
		hadLogs = true
		logTotal = logTotal.Add(earning)
	}
	if !hadLogs {
		return false, "", nil
	}
	var recorded struct{ Total decimal.Decimal }
	if err := tx.Model(&model.AdtySiteEarningDaily{}).Select("COALESCE(SUM(earning),0) AS total").
		Where("site_id = ? AND kind IN ? AND status IN ? AND day <= ? AND day NOT LIKE '%+op%'",
			siteId, []string{model.AdtySiteEarningUsage, model.AdtySiteEarningRefund, model.AdtySiteEarningSkipped},
			[]string{model.AdtySiteEarningPending, model.AdtySiteEarningAvailable}, cutoff+"+~").
		Scan(&recorded).Error; err != nil {
		return false, "", err
	}
	logTotal = logTotal.Round(8)
	rowTotal := recorded.Total.Round(8)
	if logTotal.Equal(rowTotal) {
		return false, "", nil
	}
	return true, fmt.Sprintf("usage earnings %s do not match consume logs %s", rowTotal.StringFixed(8), logTotal.StringFixed(8)), nil
}

func adtyLogUsageEarning(row model.Log, siteId int) (decimal.Decimal, bool) {
	var parsed map[string]any
	if err := common.UnmarshalJsonStr(row.Other, &parsed); err != nil {
		return decimal.Zero, false
	}
	info, _ := parsed["admin_info"].(map[string]any)
	if info["billing_operation_key"] != nil {
		return decimal.Zero, false
	}
	if adtyAnyInt(info["site_id"]) != siteId {
		return decimal.Zero, false
	}
	k, d := adtyAnyFloat(info["site_markup"]), adtyAnyFloat(info["site_cost_factor"])
	if k < 1 || d <= 0 || d > 1 {
		return decimal.Zero, false
	}
	charged := int64(row.Quota) - int64(adtyAnyInt(info["shortfall_quota"]))
	if charged <= 0 {
		return decimal.Zero, true
	}
	_, _, earning := AdtySiteEarningSplit(charged, decimal.NewFromFloat(k), decimal.NewFromFloat(d))
	if row.Type == model.LogTypeRefund {
		earning = earning.Neg()
	}
	return earning, true
}

func adtyAnyInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case string:
		n, _ := strconv.Atoi(typed)
		return n
	default:
		return 0
	}
}

func adtyAnyFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case string:
		n, _ := strconv.ParseFloat(typed, 64)
		return n
	default:
		return 0
	}
}

// ResumeAdtySiteSettlement 清掉对账暂停并立刻再结一次。数字还是对不上会重新暂停。
func ResumeAdtySiteSettlement(siteId, operatorId int, now time.Time) (*model.AdtySite, error) {
	var site model.AdtySite
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteId).First(&site).Error; err != nil {
			return err
		}
		if strings.TrimSpace(site.SettleHoldReason) == "" {
			return common.NewMessage("This site is not on settlement hold")
		}
		old := site.SettleHoldReason
		if err := tx.Model(&site).Update("settle_hold_reason", "").Error; err != nil {
			return err
		}
		site.SettleHoldReason = ""
		return recordSiteAuditTx(tx, site.Id, "settlement_resume", operatorId,
			map[string]any{"settle_hold_reason": old}, map[string]any{"settle_hold_reason": ""}, "")
	})
	if err != nil {
		return nil, err
	}
	if _, err := settleAdtySite(siteId, adtySettleCutoffDay(now), now.Unix()); err != nil {
		return nil, err
	}
	if err := model.DB.First(&site, siteId).Error; err != nil {
		return nil, err
	}
	return &site, nil
}

// adtyOwnerAccountTx 锁站长的佣金账户；还没有推广员记录时建一个（开站前用户可能没走过 P6）。
func adtyOwnerAccountTx(tx *gorm.DB, ownerUserId int) (*model.ReferralCommissionAccount, error) {
	var affiliates []model.ReferralAffiliate
	if err := tx.Where("user_id = ?", ownerUserId).Limit(1).Find(&affiliates).Error; err != nil {
		return nil, err
	}
	if len(affiliates) == 0 {
		code, err := newInviteCode()
		if err != nil {
			return nil, err
		}
		affiliate := model.ReferralAffiliate{
			UserId: ownerUserId, InviteCode: code, Status: model.ReferralAffiliateStatusApproved, SourceType: "site",
			SettlementEnabled: true, WithdrawalEnabled: true, ApprovedAt: time.Now().Unix(), SiteOwnerSince: time.Now().Unix(),
		}
		if err := tx.Create(&affiliate).Error; err != nil {
			return nil, err
		}
		if err := tx.Create(&model.ReferralCommissionAccount{AffiliateId: affiliate.Id, UserId: ownerUserId, SettlementCurrency: "USD"}).Error; err != nil {
			return nil, err
		}
		affiliates = append(affiliates, affiliate)
	}
	return lockReferralAccountTx(tx, affiliates[0].Id)
}

// adtySiteClawBackTx 是强制关站扣回：正数 pending 作废，负数 pending 当场结算（可以为负），
// skipped 作废。再从当前可提现扣回已结算收益，不超过已结算合计，也不把账户扣成负数。
// 试用额度发放时已经扣过，不计入扣回。
func adtySiteClawBackTx(tx *gorm.DB, site *model.AdtySite, operatorId int) error {
	account, err := adtyOwnerAccountTx(tx, site.OwnerUserId)
	if err != nil {
		return err
	}
	if err := tx.Model(&model.AdtySiteEarningDaily{}).
		Where("site_id = ? AND status = ? AND earning > 0", site.Id, model.AdtySiteEarningPending).
		Update("status", model.AdtySiteEarningVoid).Error; err != nil {
		return err
	}
	var negatives []model.AdtySiteEarningDaily
	if err := model.AdtyLockForUpdate(tx).
		Where("site_id = ? AND status = ? AND earning < 0 AND kind NOT IN ?",
			site.Id, model.AdtySiteEarningPending, []string{model.AdtySiteEarningSkipped, model.AdtySiteEarningTrial}).
		Find(&negatives).Error; err != nil {
		return err
	}
	if len(negatives) > 0 {
		total := decimal.Zero
		ids := make([]int64, 0, len(negatives))
		for _, row := range negatives {
			total = total.Add(row.Earning)
			ids = append(ids, row.Id)
		}
		now := time.Now().Unix()
		if err := AdjustAccountTx(tx, account, decimal.Zero, total, decimal.Zero, decimal.Zero, model.ReferralCommissionLedger{
			Type: "site_earning", RefType: "site", RefId: fmt.Sprint(site.Id),
			ExternalRefId: fmt.Sprintf("site-earning-close:%d:%d", site.Id, now),
			Remark:        "negative earnings settled on forced close", Operator: fmt.Sprint(operatorId),
		}); err != nil {
			return err
		}
		if err := tx.Model(&model.AdtySiteEarningDaily{}).Where("id IN ?", ids).Updates(map[string]any{
			"status": model.AdtySiteEarningAvailable, "settled_at": now,
		}).Error; err != nil {
			return err
		}
	}
	if err := tx.Model(&model.AdtySiteEarningDaily{}).
		Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningSkipped).
		Update("status", model.AdtySiteEarningVoid).Error; err != nil {
		return err
	}
	var fresh model.ReferralCommissionAccount
	if err := tx.Where("id = ?", account.Id).First(&fresh).Error; err != nil {
		return err
	}
	var settled struct{ Total decimal.Decimal }
	if err := tx.Model(&model.AdtySiteEarningDaily{}).Select("COALESCE(SUM(earning),0) AS total").
		Where("site_id = ? AND status = ? AND kind <> ?", site.Id, model.AdtySiteEarningAvailable, model.AdtySiteEarningTrial).
		Scan(&settled).Error; err != nil {
		return err
	}
	claw := settled.Total
	available := decimal.NewFromFloat(fresh.AvailableAmount)
	if !claw.IsPositive() || !available.IsPositive() {
		claw = decimal.Zero
	} else if available.LessThan(claw) {
		claw = available
	}
	if claw.IsPositive() {
		debited := tx.Model(&model.ReferralCommissionAccount{}).
			Where("id = ? AND available_amount >= ?", account.Id, moneyF(claw)).
			Update("available_amount", gorm.Expr("available_amount - ?", moneyF(claw)))
		if debited.Error != nil {
			return debited.Error
		}
		if debited.RowsAffected == 0 {
			return common.NewMessage("Not enough withdrawable commission")
		}
		if err := addReferralLedgerTx(tx, &model.ReferralCommissionLedger{
			AffiliateId: account.AffiliateId, UserId: account.UserId,
			DeltaAvailable: moneyF(claw.Neg()), Type: "site_earning_clawback",
			RefType: "site", RefId: fmt.Sprint(site.Id),
			ExternalRefId: fmt.Sprintf("site-clawback:%d", site.Id), Remark: "forced site close",
			Operator: fmt.Sprint(operatorId),
		}); err != nil {
			return err
		}
	}
	return tx.Model(&model.AdtySiteEarningDaily{}).Where("site_id = ? AND status = ?", site.Id, model.AdtySiteEarningAvailable).
		Update("status", model.AdtySiteEarningVoid).Error
}

func init() {
	AdtySiteClawBackTx = adtySiteClawBackTx
	adtyMasterTasks = append(adtyMasterTasks, startAdtySiteSettlementTask)
}

// startAdtySiteSettlementTask 每 10 分钟看一次：到了 settle_hour 点、今天还没跑过就结算一次。
func startAdtySiteSettlementTask() {
	go func() {
		lastRun := ""
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for now := range ticker.C {
			today := now.Format("2006-01-02")
			if lastRun == today || now.Hour() < resellerSetting().SettleHour {
				continue
			}
			if _, err := RunAdtySiteSettlementOnce(now); err != nil {
				common.SysError(common.LogText("site settlement: %s", err.Error()))
				continue
			}
			lastRun = today
		}
	}()
}
