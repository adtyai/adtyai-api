package controller

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 推广管理（任务 6.7 的后端）。所有接口都用 handlePermissionRoute 按推广返佣的权限注册。

func referralPage(c *gin.Context, query *gorm.DB, out any) bool {
	pageInfo := common.GetPageQuery(c)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return false
	}
	if err := query.Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(out).Error; err != nil {
		common.ApiError(c, err)
		return false
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(out)
	common.ApiSuccess(c, pageInfo)
	return true
}

// GetReferralAdminBadges 是侧边栏「推广管理」旁的待处理数：合作伙伴申请 + 待审核提现 + 待打款提现。
// 计数按调用者的角色与站点过滤：Root 全局，非 Root 管理员只看主站，不再做未过滤的全局合计。
func GetReferralAdminBadges(c *gin.Context) {
	badges, err := service.CollectAdtyReferralAdminBadges(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"partners": badges.Partners, "pending_withdrawals": badges.PendingWithdrawals, "approved_withdrawals": badges.ApprovedWithdrawals,
		"total": badges.Partners + badges.PendingWithdrawals + badges.ApprovedWithdrawals,
	})
}

// GetReferralOverview 是推广的总览数字（美元）。
func GetReferralOverview(c *gin.Context) {
	var affiliates, bindings int64
	if err := model.DB.Model(&model.ReferralAffiliate{}).Count(&affiliates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Model(&model.ReferralBinding{}).Count(&bindings).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var sums struct {
		Pending, Available, Frozen, Withdrawn decimal.Decimal
	}
	if err := model.DB.Model(&model.ReferralCommissionAccount{}).Select(
		"COALESCE(SUM(pending_amount),0) AS pending, COALESCE(SUM(available_amount),0) AS available, COALESCE(SUM(frozen_amount),0) AS frozen, COALESCE(SUM(withdrawn_amount),0) AS withdrawn",
	).Scan(&sums).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"affiliates": affiliates, "bindings": bindings, "pending": sums.Pending.StringFixed(2),
		"available": sums.Available.StringFixed(2), "frozen": sums.Frozen.StringFixed(2), "withdrawn": sums.Withdrawn.StringFixed(2),
	})
}

// GetReferralSettings 返回推广设置；提现汇率直接查库。
func GetReferralSettings(c *gin.Context) {
	cfg := *adty_setting.GetReferralSetting()
	if rate, updatedAt, err := service.CurrentWithdrawCNYRate(); err == nil {
		cfg.WithdrawCNYRate, cfg.WithdrawCNYRateUpdatedAt = rate.String(), updatedAt
	} else {
		cfg.WithdrawCNYRate, cfg.WithdrawCNYRateUpdatedAt = "", 0
	}
	common.ApiSuccess(c, cfg)
}

// UpdateReferralSettings 保存推广设置。提现汇率和这些设置同一次提交，汇率不合法或写库失败时一起回滚。
func UpdateReferralSettings(c *gin.Context) {
	var body struct {
		DefaultRate              *float64 `json:"default_rate"`
		CookieTTLDays            *int     `json:"cookie_ttl_days"`
		FreezeDays               *int     `json:"freeze_days"`
		MinWithdrawUSD           *float64 `json:"min_withdraw_usd"`
		USDTWithdrawFee          *float64 `json:"usdt_withdraw_fee"`
		CancelWindowMinutes      *int     `json:"cancel_window_minutes"`
		SelfInviteWindowDays     *int     `json:"self_invite_window_days"`
		BatchRegisterThreshold   *int     `json:"batch_register_threshold"`
		BatchRegisterWindowHours *int     `json:"batch_register_window_hours"`
		Enabled                  *bool    `json:"enabled"`
		WithdrawCNYRate          string   `json:"withdraw_cny_rate"`
		// Clear 列出要改回「未设置」的钱款项：default_rate、min_withdraw_usd、usdt_withdraw_fee（约定 I）。
		Clear []string `json:"clear"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	values := map[string]string{}
	putFloat := func(key string, value *float64, minimum float64) bool {
		if value == nil {
			return true
		}
		if *value < minimum {
			common.ApiErrorT(c, "Invalid parameters")
			return false
		}
		values["adty_referral_setting."+key] = strconv.FormatFloat(*value, 'f', -1, 64)
		return true
	}
	putInt := func(key string, value *int, minimum int) bool {
		if value == nil {
			return true
		}
		if *value < minimum {
			common.ApiErrorT(c, "Invalid parameters")
			return false
		}
		values["adty_referral_setting."+key] = strconv.Itoa(*value)
		return true
	}
	if body.DefaultRate != nil && *body.DefaultRate > 100 {
		common.ApiErrorT(c, "The rate must be between 0 and 100")
		return
	}
	if !putFloat("default_rate", body.DefaultRate, 0) || !putInt("cookie_ttl_days", body.CookieTTLDays, 1) ||
		!putInt("freeze_days", body.FreezeDays, 0) || !putFloat("min_withdraw_usd", body.MinWithdrawUSD, 0) ||
		!putFloat("usdt_withdraw_fee", body.USDTWithdrawFee, 0) || !putInt("cancel_window_minutes", body.CancelWindowMinutes, 0) ||
		!putInt("self_invite_window_days", body.SelfInviteWindowDays, 1) || !putInt("batch_register_threshold", body.BatchRegisterThreshold, 0) ||
		!putInt("batch_register_window_hours", body.BatchRegisterWindowHours, 1) {
		return
	}
	for _, key := range body.Clear {
		switch key {
		case "default_rate", "min_withdraw_usd", "usdt_withdraw_fee":
			values["adty_referral_setting."+key] = "null"
		default:
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	}
	if body.Enabled != nil {
		values["adty_referral_setting.enabled"] = strconv.FormatBool(*body.Enabled)
	}
	if err := service.Referral.SaveReferralSettings(values, body.WithdrawCNYRate, c.GetInt("id"), c.ClientIP(), c.Request.UserAgent()); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.referral.settings", map[string]any{"keys": len(values)})
	GetReferralSettings(c)
}

// GetReferralAffiliates 列出推广员，可按状态、合作伙伴申请、关键词筛选。
func GetReferralAffiliates(c *gin.Context) {
	query := model.DB.Model(&model.ReferralAffiliate{})
	if partner := strings.TrimSpace(c.Query("partner_status")); partner != "" {
		query = query.Where("partner_status = ?", partner)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		query = query.Where("invite_code = ? OR user_id IN (?)", strings.ToUpper(keyword),
			model.DB.Model(&model.User{}).Select("id").Where("username LIKE ?", "%"+keyword+"%"))
	}
	var items []model.ReferralAffiliate
	referralPage(c, query, &items)
}

// GetReferralBindings 列出一个推广员带来的用户。
func GetReferralBindings(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var items []model.ReferralBinding
	referralPage(c, model.DB.Model(&model.ReferralBinding{}).Where("inviter_user_id = ?", userID), &items)
}

// ReviewReferralPartner 通过（带 15–20 的比例）或驳回合作伙伴申请（推广返佣·审核）。
func ReviewReferralPartner(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		Approve bool            `json:"approve"`
		Rate    decimal.Decimal `json:"rate"`
		Reason  string          `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.Referral.ReviewPartner(userID, c.GetInt("id"), body.Approve, body.Rate, body.Reason, c.ClientIP(), c.Request.UserAgent()); err != nil {
		writeReferralError(c, err)
		return
	}
	recordManageAuditFor(c, userID, "adty.referral.partner_review", map[string]any{"approve": body.Approve, "rate": body.Rate.String()})
	common.ApiSuccess(c, nil)
}

// UpdateReferralAffiliate 调整推广员（推广返佣·调整）：设比例、停用/恢复、冻结/恢复结算和提现、调佣金。
func UpdateReferralAffiliate(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		Action         string          `json:"action"` // rate | disable | restore | freeze_settlement | restore_settlement | freeze_withdrawal | restore_withdrawal | restore_referral | adjust
		Rate           *float64        `json:"rate"`
		Amount         decimal.Decimal `json:"amount"`
		Reason         string          `json:"reason"`
		IdempotencyKey string          `json:"idempotency_key"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	adminID := c.GetInt("id")
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var affiliate model.ReferralAffiliate
		if err := model.AdtyLockForUpdate(tx).Where("user_id = ?", userID).First(&affiliate).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		switch body.Action {
		case "rate":
			if body.Rate != nil && (*body.Rate < 0 || *body.Rate > 100) {
				return common.NewMessage("The rate must be between 0 and 100")
			}
			updates["rate_override"] = body.Rate
		case "disable":
			updates["status"], updates["disabled_at"], updates["disabled_by"] = model.ReferralAffiliateStatusDisabled, time.Now().Unix(), adminID
		case "restore":
			updates["status"], updates["disabled_at"], updates["disabled_by"] = model.ReferralAffiliateStatusApproved, 0, 0
		case "freeze_settlement", "restore_settlement":
			updates["settlement_enabled"] = body.Action == "restore_settlement"
			if body.Action == "restore_settlement" {
				updates["risk_reason"] = ""
			}
		case "freeze_withdrawal", "restore_withdrawal":
			updates["withdrawal_enabled"] = body.Action == "restore_withdrawal"
		case "restore_referral":
			// 只有名下没有未关闭的站才能恢复邀请返佣。active、paused、closing 都还是站长。
			var openSites int64
			if err := tx.Model(&model.AdtySite{}).Where("owner_user_id = ? AND status <> ?", userID, model.AdtySiteStatusClosed).Count(&openSites).Error; err != nil {
				return err
			}
			if openSites > 0 {
				return common.NewMessage("Close the site before restoring referral commissions")
			}
			updates["site_owner_since"] = 0
		case "adjust":
			key := strings.TrimSpace(body.IdempotencyKey)
			if body.Amount.IsZero() || strings.TrimSpace(body.Reason) == "" || key == "" || len(key) > 80 || strings.ContainsAny(key, " \t\r\n") {
				return common.NewMessage("Enter an amount, a reason, and an idempotency key")
			}
			ref := "admin-adjust:" + strconv.Itoa(adminID) + ":" + key
			var seen int64
			if err := tx.Model(&model.ReferralCommissionLedger{}).Where("external_ref_id = ?", ref).Count(&seen).Error; err != nil {
				return err
			}
			if seen == 0 {
				var acc model.ReferralCommissionAccount
				if err := model.AdtyLockForUpdate(tx).Where("affiliate_id = ?", affiliate.Id).First(&acc).Error; err != nil {
					return err
				}
				if err := service.AdjustAccountTx(tx, &acc, decimal.Zero, body.Amount.Round(2), decimal.Zero, decimal.Zero, model.ReferralCommissionLedger{
					Type: "admin_adjust", RefType: "admin", RefId: fmt.Sprint(adminID), ExternalRefId: ref, Remark: strings.TrimSpace(body.Reason), Operator: fmt.Sprint(adminID),
				}); err != nil {
					return err
				}
			}
		default:
			return common.NewMessage("Invalid parameters")
		}
		if len(updates) > 0 {
			if err := tx.Model(&model.ReferralAffiliate{}).Where("id = ?", affiliate.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.ReferralAdminAuditLog{
			Action: body.Action, TargetUserId: userID, AffiliateId: affiliate.Id, AdminUserId: adminID,
			Reason: body.Reason, Ip: c.ClientIP(), UserAgent: c.Request.UserAgent(), NewValue: common.GetJsonString(body),
		}).Error
	})
	if err != nil {
		writeReferralError(c, err)
		return
	}
	recordManageAuditFor(c, userID, "adty.referral.affiliate_update", map[string]any{"action": body.Action})
	common.ApiSuccess(c, nil)
}

// GetAdminReferralCommissions 列出佣金（含作废、扣回）。
func GetAdminReferralCommissions(c *gin.Context) {
	query := model.DB.Model(&model.ReferralCommission{})
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if raw := c.Query("affiliate_id"); raw != "" {
		query = query.Where("affiliate_id = ?", raw)
	}
	var items []model.ReferralCommission
	referralPage(c, query, &items)
}

// GetReferralCommissionJobs 列出计佣任务。
func GetReferralCommissionJobs(c *gin.Context) {
	query := model.DB.Model(&model.ReferralCommissionJob{})
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	var items []model.ReferralCommissionJob
	referralPage(c, query, &items)
}

// RetryReferralCommissionJob 立即重试一个计佣任务。
func RetryReferralCommissionJob(c *gin.Context) {
	var body struct {
		TradeNo string `json:"trade_no"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.TradeNo == "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.ProcessAdtyTopUpCommission(body.TradeNo); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// RunReferralSettlement 手动跑一轮结算。
func RunReferralSettlement(c *gin.Context) {
	settled, err := service.RunReferralSettlementOnce(time.Now().Unix(), 500)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"settled": settled})
}

// GetReferralLedgers 列出返佣流水。
func GetReferralLedgers(c *gin.Context) {
	query := model.DB.Model(&model.ReferralCommissionLedger{})
	if raw := c.Query("affiliate_id"); raw != "" {
		query = query.Where("affiliate_id = ?", raw)
	}
	var items []model.ReferralCommissionLedger
	referralPage(c, query, &items)
}

// GetReferralAdminAuditLogs 列出推广管理的操作记录。
func GetReferralAdminAuditLogs(c *gin.Context) {
	var items []model.ReferralAdminAuditLog
	referralPage(c, model.DB.Model(&model.ReferralAdminAuditLog{}), &items)
}

// GetAdminReferralWithdrawals 列出提现，收款码和凭证链接重新签名。
func GetAdminReferralWithdrawals(c *gin.Context) {
	query := model.DB.Model(&model.ReferralWithdrawal{})
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	pageInfo := common.GetPageQuery(c)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var items []model.ReferralWithdrawal
	if err := query.Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&items).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	resignWithdrawalAssets(items)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// ReviewReferralWithdrawal 通过或驳回提现（推广返佣·审核）。
func ReviewReferralWithdrawal(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if !body.Approve && strings.TrimSpace(body.Reason) == "" {
		common.ApiErrorT(c, "Enter a reason for rejecting")
		return
	}
	if err := service.Referral.ReviewWithdrawal(id, c.GetInt("id"), body.Approve, body.Reason, c.ClientIP(), c.Request.UserAgent()); err != nil {
		writeReferralError(c, err)
		return
	}
	recordManageAudit(c, "adty.referral.withdrawal_review", map[string]any{"id": id, "approve": body.Approve})
	common.ApiSuccess(c, nil)
}

// PayReferralWithdrawal 标记已打款（推广返佣·打款）。
func PayReferralWithdrawal(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		TxnNo    string `json:"txn_no"`
		ProofURL string `json:"proof_url"`
		Note     string `json:"note"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.Referral.MarkWithdrawalPaid(id, c.GetInt("id"), body.TxnNo, body.ProofURL, body.Note, c.ClientIP(), c.Request.UserAgent()); err != nil {
		writeReferralError(c, err)
		return
	}
	recordManageAudit(c, "adty.referral.withdrawal_pay", map[string]any{"id": id})
	common.ApiSuccess(c, nil)
}
