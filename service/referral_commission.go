package service

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 计佣、结算、退款作废和扣回（任务 6.3）。

// adtyReferralSnapshot 是下单时的返佣快照：用户有绑定、推广员有效、绑定没有自邀标记、比例已设时，
// 返回推广员和当时的比例。之后改比例不影响已经下的单。
// 推广员开了分站（SiteOwnerSince > 0）、或者下单的是分站客户，就不计佣；开站前下的单照常（执行层决定 27）。
func adtyReferralSnapshot(userId int) (int, decimal.Decimal) {
	if !Referral.IsEnabled() {
		return 0, decimal.Zero
	}
	// 分站客户不计 P6 佣金（执行层决定 27）。读库不读缓存：site_id 只在注册和关站转移时变。
	var owners []model.User
	if err := model.DB.Select("id", "site_id").Where("id = ?", userId).Limit(1).Find(&owners).Error; err != nil || len(owners) == 0 || owners[0].SiteId > 0 {
		return 0, decimal.Zero
	}
	var bindings []model.ReferralBinding
	if err := model.DB.Where("invitee_user_id = ?", userId).Limit(1).Find(&bindings).Error; err != nil || len(bindings) == 0 {
		return 0, decimal.Zero
	}
	binding := bindings[0]
	if binding.RiskFlag == model.ReferralRiskSelfInvite {
		return 0, decimal.Zero
	}
	var affiliates []model.ReferralAffiliate
	if err := model.DB.Where("id = ?", binding.AffiliateId).Limit(1).Find(&affiliates).Error; err != nil || len(affiliates) == 0 {
		return 0, decimal.Zero
	}
	affiliate := affiliates[0]
	if affiliate.Status != model.ReferralAffiliateStatusApproved || affiliate.SiteOwnerSince > 0 {
		return 0, decimal.Zero
	}
	var affiliateUsers []model.User
	if err := model.DB.Select("site_id").Where("id = ?", affiliate.UserId).Limit(1).Find(&affiliateUsers).Error; err != nil || len(affiliateUsers) == 0 || affiliateUsers[0].SiteId > 0 {
		return 0, decimal.Zero
	}
	rate, ok := AffiliateRate(&affiliate)
	if !ok {
		return 0, decimal.Zero
	}
	return affiliate.Id, rate
}

// adtyReferralAfterCreditTx 在入账的同一个事务里插一条计佣任务。只用 OnConflict DoNothing，
// 不做可能报错的查询：PostgreSQL 下钩子一出错，整个入账事务都作废（审阅重点 10）。
func adtyReferralAfterCreditTx(tx *gorm.DB, topUp *model.TopUp, ext *model.AdtyTopupOrder) error {
	if ext == nil || ext.ReferralAffiliateId == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ReferralCommissionJob{
		SourceType:    model.ReferralSourceTopup,
		SourceTradeNo: topUp.TradeNo,
		AffiliateId:   ext.ReferralAffiliateId,
		Status:        model.ReferralCommissionJobStatusPending,
	}).Error
}

// ProcessAdtyTopUpCommission 处理一个充值的计佣任务：佣金 = 面额（不含加赠）× 快照比例 ÷ 100，
// 状态 pending，SettleAt = 现在 + 冻结天数。由结算任务每轮扫描调用。
func ProcessAdtyTopUpCommission(tradeNo string) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var job model.ReferralCommissionJob
		if err := model.AdtyLockForUpdate(tx).Where("source_type = ? AND source_trade_no = ?", model.ReferralSourceTopup, tradeNo).First(&job).Error; err != nil {
			return err
		}
		if job.Status == model.ReferralCommissionJobStatusSucceeded || job.Status == model.ReferralCommissionJobStatusSkipped {
			return nil
		}
		skip := func(reason string) error {
			return tx.Model(&model.ReferralCommissionJob{}).Where("id = ?", job.Id).Updates(map[string]any{
				"status": model.ReferralCommissionJobStatusSkipped, "last_error": reason, "attempt_count": job.AttemptCount + 1,
			}).Error
		}
		var topUps []model.TopUp
		if err := tx.Where("trade_no = ?", tradeNo).Limit(1).Find(&topUps).Error; err != nil {
			return err
		}
		if len(topUps) == 0 {
			return skip("order not found")
		}
		status := topUps[0].Status
		if status != common.TopUpStatusSuccess && status != model.TopUpStatusRefunded {
			return skip("order is " + status)
		}
		ext, err := model.GetAdtyTopupOrder(tradeNo)
		if err != nil {
			return err
		}
		if ext == nil || ext.ReferralAffiliateId == 0 {
			return skip("no referral snapshot")
		}
		// 退款早于计佣：退掉的本金不计佣，没退的（用户花掉的）照算。全额退本金时跳过。
		refundRatio := decimal.Zero
		if status == model.TopUpStatusRefunded {
			refundRatio = ext.RefundRatio()
			if refundRatio.GreaterThanOrEqual(decimal.NewFromInt(1)) {
				return skip("order refunded before commission")
			}
		}
		var bindings []model.ReferralBinding
		if err := tx.Where("invitee_user_id = ?", topUps[0].UserId).Limit(1).Find(&bindings).Error; err != nil {
			return err
		}
		if len(bindings) > 0 && bindings[0].RiskFlag == model.ReferralRiskSelfInvite {
			return skip("self invite: no commission")
		}
		var affiliate model.ReferralAffiliate
		if err := tx.Where("id = ?", ext.ReferralAffiliateId).First(&affiliate).Error; err != nil {
			return err
		}
		amount := ext.FaceUSD.Mul(ext.ReferralRate).Div(decimal.NewFromInt(100)).Round(8)
		amount = amount.Sub(referralRefundClawback(amount, refundRatio))
		if !amount.IsPositive() {
			return skip("zero commission")
		}
		account, err := lockReferralAccountTx(tx, affiliate.Id)
		if err != nil {
			return err
		}
		now := time.Now().Unix()
		freeze := int64(max(referralSetting().FreezeDays, 0)) * 86400
		commission := &model.ReferralCommission{
			AffiliateId: affiliate.Id, AffiliateUserId: affiliate.UserId, InviteeUserId: topUps[0].UserId,
			SourceType: model.ReferralSourceTopup, SourceOrderId: topUps[0].Id, SourceTradeNo: tradeNo, OrderType: model.ReferralSourceTopup,
			BaseAmount: moneyF(ext.FaceUSD), PaidAmount: moneyF(ext.PaidAmount), PaidCurrency: ext.PaidCurrency,
			SettlementCurrency: "USD", SettlementFxRate: 1, SettlementBaseAmount: moneyF(ext.FaceUSD),
			Rate: moneyF(ext.ReferralRate), CommissionAmount: moneyF(amount),
			Status: model.ReferralCommissionStatusPending, SettleAt: now + freeze,
		}
		if err := tx.Create(commission).Error; err != nil {
			return err
		}
		if err := AdjustAccountTx(tx, account, amount, decimal.Zero, decimal.Zero, decimal.Zero, model.ReferralCommissionLedger{
			CommissionId: commission.Id, Type: "commission_accrue", RefType: model.ReferralSourceTopup, RefId: tradeNo,
			ExternalRefId: "accrue:" + tradeNo, Remark: "commission accrued",
		}); err != nil {
			return err
		}
		return tx.Model(&model.ReferralCommissionJob{}).Where("id = ?", job.Id).Updates(map[string]any{
			"status": model.ReferralCommissionJobStatusSucceeded, "succeeded_at": now, "attempt_count": job.AttemptCount + 1, "last_error": "",
		}).Error
	})
}

// markReferralJobFailed 把出错的计佣任务记成 failed，下一轮结算再重试。
func markReferralJobFailed(tradeNo string, cause error) {
	_ = model.DB.Model(&model.ReferralCommissionJob{}).
		Where("source_type = ? AND source_trade_no = ? AND status IN ?", model.ReferralSourceTopup, tradeNo,
			[]string{model.ReferralCommissionJobStatusPending, model.ReferralCommissionJobStatusFailed}).
		Updates(map[string]any{"status": model.ReferralCommissionJobStatusFailed, "failed_at": time.Now().Unix(), "last_error": truncateText(cause.Error(), 1000), "attempt_count": gorm.Expr("attempt_count + 1")}).Error
}

// RunReferralSettlementOnce 跑一轮结算：先重试 pending、failed 的计佣任务，再把到期的 pending 佣金转成可提现
// （停结算的推广员跳过）。返回转成可提现的笔数。
func RunReferralSettlementOnce(now int64, limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	var jobs []model.ReferralCommissionJob
	if err := model.DB.Where("source_type = ? AND status IN ?", model.ReferralSourceTopup,
		[]string{model.ReferralCommissionJobStatusPending, model.ReferralCommissionJobStatusFailed}).
		Order("id").Limit(limit).Find(&jobs).Error; err != nil {
		return 0, err
	}
	for _, job := range jobs {
		if err := ProcessAdtyTopUpCommission(job.SourceTradeNo); err != nil {
			markReferralJobFailed(job.SourceTradeNo, err)
		}
	}
	var due []model.ReferralCommission
	if err := model.DB.Table("referral_commissions AS c").Select("c.*").
		Joins("JOIN referral_affiliates a ON a.id = c.affiliate_id").
		Where("c.status = ? AND c.settle_at <= ? AND a.settlement_enabled = ?", model.ReferralCommissionStatusPending, now, true).
		Order("c.settle_at, c.id").Limit(limit).Scan(&due).Error; err != nil {
		return 0, err
	}
	settled := 0
	for _, item := range due {
		err := model.DB.Transaction(func(tx *gorm.DB) error {
			var commission model.ReferralCommission
			if err := model.AdtyLockForUpdate(tx).Where("id = ?", item.Id).First(&commission).Error; err != nil {
				return err
			}
			if commission.Status != model.ReferralCommissionStatusPending {
				return nil
			}
			var affiliate model.ReferralAffiliate
			if err := model.AdtyLockForUpdate(tx).Where("id = ?", commission.AffiliateId).First(&affiliate).Error; err != nil {
				return err
			}
			if !affiliate.SettlementEnabled {
				return nil
			}
			account, err := lockReferralAccountTx(tx, commission.AffiliateId)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.ReferralCommission{}).Where("id = ?", commission.Id).Updates(map[string]any{
				"status": model.ReferralCommissionStatusAvailable, "available_at": now,
			}).Error; err != nil {
				return err
			}
			amount := moneyDec(commission.CommissionAmount)
			if err := AdjustAccountTx(tx, account, amount.Neg(), amount, decimal.Zero, decimal.Zero, model.ReferralCommissionLedger{
				CommissionId: commission.Id, Type: "commission_settle", RefType: model.ReferralSourceTopup, RefId: commission.SourceTradeNo,
				ExternalRefId: fmt.Sprintf("settle:%d", commission.Id), Remark: "commission settled",
			}); err != nil {
				return err
			}
			settled++
			return nil
		})
		if err != nil {
			common.SysError(common.LogText("referral settle commission %d: %s", item.Id, err.Error()))
		}
	}
	return settled, nil
}

// referralRefundClawback 是退款要扣回的佣金：佣金 × 退掉的本金比例，8 位小数向下取整。计佣任务和退款钩子共用。
func referralRefundClawback(commission, ratio decimal.Decimal) decimal.Decimal {
	if !ratio.IsPositive() || !commission.IsPositive() {
		return decimal.Zero
	}
	return commission.Mul(ratio).RoundFloor(8)
}

// adtyReferralAfterRefundTx 在退款事务里按退掉的本金比例 ratio = P ÷ F 处理佣金（赠金不退，用户花掉的部分照样计佣）。
//   - ratio = 1：和以前一样整笔处理。pending 作废并扣掉待结算；已经可提现或已提现的扣回，从可提现里扣，
//     可以为负（之后的新佣金结算时自然先抵扣）。还没计佣时，计佣任务标 skipped。
//   - 0 < ratio < 1：状态不改，扣回 佣金 × ratio，记一笔 commission_clawback_partial。pending 的佣金同时把
//     佣金额减掉这部分，结算时只转剩下的；已经可提现或已提现的从可提现里扣。还没计佣时任务不动，
//     计佣任务看到订单已退款，按 (1 − ratio) 计佣。
//   - ratio = 0：什么都不做。
//
// 先锁计佣任务行：计佣任务处理时也先锁它，两边串行，不会漏扣或多计。
func adtyReferralAfterRefundTx(tx *gorm.DB, topUp *model.TopUp, ext *model.AdtyTopupOrder) error {
	ratio := ext.RefundRatio()
	if !ratio.IsPositive() {
		return nil
	}
	var jobs []model.ReferralCommissionJob
	if err := model.AdtyLockForUpdate(tx).Where("source_type = ? AND source_trade_no = ?", model.ReferralSourceTopup, topUp.TradeNo).Find(&jobs).Error; err != nil {
		return err
	}
	full := ratio.GreaterThanOrEqual(decimal.NewFromInt(1))
	if full {
		if err := tx.Model(&model.ReferralCommissionJob{}).
			Where("source_type = ? AND source_trade_no = ? AND status IN ?", model.ReferralSourceTopup, topUp.TradeNo,
				[]string{model.ReferralCommissionJobStatusPending, model.ReferralCommissionJobStatusFailed}).
			Updates(map[string]any{"status": model.ReferralCommissionJobStatusSkipped, "last_error": "order refunded before commission"}).Error; err != nil {
			return err
		}
	}
	var commissions []model.ReferralCommission
	if err := model.AdtyLockForUpdate(tx).Where("source_type = ? AND source_trade_no = ?", model.ReferralSourceTopup, topUp.TradeNo).Order("id").Find(&commissions).Error; err != nil {
		return err
	}
	for _, commission := range commissions {
		amount := moneyDec(commission.CommissionAmount)
		account, err := lockReferralAccountTx(tx, commission.AffiliateId)
		if err != nil {
			return err
		}
		if !full {
			if err := clawBackCommissionPartTx(tx, account, &commission, referralRefundClawback(amount, ratio), topUp.TradeNo); err != nil {
				return err
			}
			continue
		}
		switch commission.Status {
		case model.ReferralCommissionStatusPending:
			if err := tx.Model(&model.ReferralCommission{}).Where("id = ?", commission.Id).Update("status", model.ReferralCommissionStatusVoid).Error; err != nil {
				return err
			}
			if err := AdjustAccountTx(tx, account, amount.Neg(), decimal.Zero, decimal.Zero, decimal.Zero, model.ReferralCommissionLedger{
				CommissionId: commission.Id, Type: "commission_void", RefType: "refund", RefId: topUp.TradeNo,
				ExternalRefId: fmt.Sprintf("void:%d", commission.Id), Remark: "order refunded",
			}); err != nil {
				return err
			}
		case model.ReferralCommissionStatusAvailable, model.ReferralCommissionStatusFrozen, model.ReferralCommissionStatusPaid:
			if err := tx.Model(&model.ReferralCommission{}).Where("id = ?", commission.Id).Update("status", model.ReferralCommissionStatusClawedBack).Error; err != nil {
				return err
			}
			// 不带 available_amount >= ? 的条件：扣回可以让可提现变成负数。
			if err := AdjustAccountTx(tx, account, decimal.Zero, amount.Neg(), decimal.Zero, decimal.Zero, model.ReferralCommissionLedger{
				CommissionId: commission.Id, Type: "commission_clawback", RefType: "refund", RefId: topUp.TradeNo,
				ExternalRefId: fmt.Sprintf("clawback:%d", commission.Id), Remark: "order refunded",
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// clawBackCommissionPartTx 部分扣回一笔佣金（退款只退了一部分本金）：状态不改。pending 的把佣金额和待结算一起减掉；
// 可提现、冻结、已提现的从可提现里扣（可以为负）；void、clawed_back 不动。流水的外部引用带佣金 id，只记一次。
func clawBackCommissionPartTx(tx *gorm.DB, account *model.ReferralCommissionAccount, commission *model.ReferralCommission, part decimal.Decimal, tradeNo string) error {
	if !part.IsPositive() {
		return nil
	}
	entry := model.ReferralCommissionLedger{
		CommissionId: commission.Id, Type: "commission_clawback_partial", RefType: "refund", RefId: tradeNo,
		ExternalRefId: fmt.Sprintf("clawback_partial:%d", commission.Id), Remark: "order partially refunded",
	}
	switch commission.Status {
	case model.ReferralCommissionStatusPending:
		kept := moneyDec(commission.CommissionAmount).Sub(part)
		if err := tx.Model(&model.ReferralCommission{}).Where("id = ?", commission.Id).Update("commission_amount", moneyF(kept)).Error; err != nil {
			return err
		}
		return AdjustAccountTx(tx, account, part.Neg(), decimal.Zero, decimal.Zero, decimal.Zero, entry)
	case model.ReferralCommissionStatusAvailable, model.ReferralCommissionStatusFrozen, model.ReferralCommissionStatusPaid:
		return AdjustAccountTx(tx, account, decimal.Zero, part.Neg(), decimal.Zero, decimal.Zero, entry)
	}
	return nil
}

func init() {
	AdtyReferralSnapshotHook = adtyReferralSnapshot
	// 入账和退款的事务钩子只有一个，P6 和分站（11.5 的加赠扣减）一起挂上。
	model.AdtyAfterTopUpCreditedTx = func(tx *gorm.DB, topUp *model.TopUp, ext *model.AdtyTopupOrder) error {
		if err := adtyReferralAfterCreditTx(tx, topUp, ext); err != nil {
			return err
		}
		return adtySiteBonusAfterCreditTx(tx, topUp, ext)
	}
	model.AdtyAfterTopUpRefundedTx = func(tx *gorm.DB, topUp *model.TopUp, ext *model.AdtyTopupOrder) error {
		if err := adtyReferralAfterRefundTx(tx, topUp, ext); err != nil {
			return err
		}
		return adtySiteBonusAfterRefundTx(tx, topUp, ext)
	}
	adtyMasterTasks = append(adtyMasterTasks, startReferralSettlementTask)
}

// startReferralSettlementTask 在主节点启动结算任务：间隔读 REFERRAL_SETTLEMENT_INTERVAL_SECONDS，默认 60，最少 10。
func startReferralSettlementTask() {
	interval := max(common.GetEnvOrDefault("REFERRAL_SETTLEMENT_INTERVAL_SECONDS", 60), 10)
	go func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := RunReferralSettlementOnce(time.Now().Unix(), 200); err != nil {
				common.SysError(common.LogText("referral settlement: %s", err.Error()))
			}
		}
	}()
}
