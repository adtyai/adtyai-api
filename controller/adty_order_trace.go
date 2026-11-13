package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Only explicitly linked records are exposed. A customer's other wallet
// operations and a site's aggregated daily earnings do not belong to an order.
type adtyOrderTraceEntry struct {
	Id             int64            `json:"id"`
	Kind           string           `json:"kind"`
	Source         string           `json:"source"`
	Status         string           `json:"status"`
	CreatedAt      int64            `json:"created_at"`
	Reference      string           `json:"reference"`
	Amount         *decimal.Decimal `json:"amount,omitempty"`
	Currency       string           `json:"currency,omitempty"`
	DeltaQuota     *int64           `json:"delta_quota,omitempty"`
	BalanceBefore  *decimal.Decimal `json:"balance_before,omitempty"`
	BalanceAfter   *decimal.Decimal `json:"balance_after,omitempty"`
	PendingDelta   *decimal.Decimal `json:"pending_delta,omitempty"`
	AvailableDelta *decimal.Decimal `json:"available_delta,omitempty"`
	FrozenDelta    *decimal.Decimal `json:"frozen_delta,omitempty"`
	WithdrawnDelta *decimal.Decimal `json:"withdrawn_delta,omitempty"`
	OperatorId     int              `json:"operator_id,omitempty"`
	Note           string           `json:"note,omitempty"`
}

func GetRechargeAuditOrder(c *gin.Context) { getAdtyOrderTrace(c, nil) }

func GetAdtyOwnerOrder(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if ok {
		getAdtyOrderTrace(c, &site.Id)
	}
}

func getAdtyOrderTrace(c *gin.Context, ownerSiteID *int) {
	tradeNo := c.Param("trade_no")
	if tradeNo == "" || len(tradeNo) > 255 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	query := model.DB.Table("top_ups AS t").
		Joins("LEFT JOIN adty_topup_orders a ON a.trade_no = t.trade_no").
		Joins("LEFT JOIN users u ON u.id = t.user_id").Where("t.trade_no = ?", tradeNo)
	if ownerSiteID != nil {
		query = query.Where("a.site_id = ?", *ownerSiteID)
	}
	var orders []rechargeAuditOrder
	if err := query.Select(rechargeAuditSelect).Limit(1).Scan(&orders).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if len(orders) == 0 {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Order not found"))
		return
	}
	order := orders[0]
	order.OrderType = "topup"
	order.Anomaly = rechargeAuditAnomaly(order)
	if order.SiteId != nil {
		names, err := service.AdtySiteNames([]int{*order.SiteId})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		order.SiteName = names[*order.SiteId]
	}
	order.ChannelName = adtyEpayChannelNames()[order.ChannelId]
	var ext model.AdtyTopupOrder
	err := model.DB.Where("trade_no = ?", tradeNo).First(&ext).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return
	}
	sections := []string{"payments", "wallet", "bonuses", "exceptions", "reviews"}
	if ownerSiteID == nil {
		sections = []string{"payments", "wallet", "bonuses", "site_wallet", "exceptions", "reviews", "commissions", "commission_movements"}
	}
	section := c.DefaultQuery("section", "payments")
	traceQuery, projection, ok := adtyOrderTraceQuery(section, order, ownerSiteID)
	if !ok {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	page := common.GetPageQuery(c)
	var total int64
	if err := traceQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	entries := []adtyOrderTraceEntry{}
	if err := traceQuery.Select(projection).Order("created_at DESC, id DESC").Limit(page.GetPageSize()).Offset(page.GetStartIdx()).Scan(&entries).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	refundCurrency := ext.PaidCurrency
	if refundCurrency == "" {
		refundCurrency = ext.PayCurrency
	}
	refund := gin.H{"refunded_at": ext.RefundedAt, "refund_quota": ext.RefundQuota, "refund_amount": ext.RefundAmount, "currency": refundCurrency}
	preview, previewErr := model.LoadSiteInviteCommissionRefundPreview(model.DB, tradeNo, ext.RefundQuota, ext.FaceQuota())
	if previewErr != nil {
		common.ApiError(c, previewErr)
		return
	}
	if preview != nil {
		refund["commission"] = preview
		if preview.ClawedUSD != "0" {
			refund["commission_clawed_usd"] = preview.ClawedUSD
			refund["commission_recovered_usd"] = preview.RecoveredUSD
		}
		if preview.WithdrawnUSD != "0" {
			refund["commission_withdrawn_usd"] = preview.WithdrawnUSD
		}
	}
	var recoveries []model.AdtyCommissionRecovery
	if err := model.DB.Where("trade_no = ?", tradeNo).Find(&recoveries).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	recovery := decimal.Zero
	for _, row := range recoveries {
		recovery = recovery.Add(row.AmountUSD)
	}
	if recovery.IsPositive() {
		refund["commission_recovery_usd"] = recovery.String()
	}
	if ownerSiteID == nil {
		refund["reason"] = ext.RefundReason
	}
	if ownerSiteID != nil {
		if order.PaidSource == model.AdtyPaymentSourceManual {
			order.PaidTxHash = ""
		}
		for i := range entries {
			entries[i].Note, entries[i].OperatorId = "", 0
			// These balances include the buyer's unrelated activity at other sites.
			if section == "wallet" {
				entries[i].BalanceBefore, entries[i].BalanceAfter = nil, nil
			}
		}
	}
	common.ApiSuccess(c, gin.H{"order": order, "refund": refund, "promo_bonus_usd": ext.PromoBonusUSD,
		"section": section, "sections": sections, "items": entries, "total": total, "page": page.GetPage(), "page_size": page.GetPageSize()})
}

// Each section has its own total and pagination. Static projections omit raw
// provider payload/error fields and administrative credentials for every role.
func adtyOrderTraceQuery(section string, order rechargeAuditOrder, ownerSiteID *int) (*gorm.DB, string, bool) {
	switch section {
	case "payments":
		q := model.DB.Model(&model.AdtyOrderPaymentEvent{}).Where("trade_no = ? AND user_id = ? AND provider = ?", order.TradeNo, order.UserId, order.PaymentProvider)
		if ownerSiteID != nil {
			q = q.Where("site_id = ?", *ownerSiteID)
		}
		return q, "id, 'Payment' AS kind, source, outcome AS status, event_type AS reference, CASE WHEN outcome IN ('credited', 'replayed') THEN paid_amount ELSE NULL END AS amount, paid_currency AS currency, created_at, operator_id", true
	case "wallet":
		return model.DB.Model(&model.AdtyBalanceLedger{}).Where("ref_type = ? AND ref_id = ? AND user_id = ?", "topup", order.TradeNo, order.UserId),
			"id, 'Wallet movement' AS kind, type AS status, ref_id AS reference, delta_quota, balance_before, balance_after, operator_id, reason AS note, created_at", true
	case "bonuses":
		return model.DB.Model(&model.AdtyPromoRedemption{}).Where("trade_no = ? AND user_id = ?", order.TradeNo, order.UserId),
			"id, 'Promo bonus' AS kind, bonus_usd AS amount, 'USD' AS currency, created_at", true
	case "site_wallet":
		if ownerSiteID != nil {
			return nil, "", false
		}
		q := model.DB.Model(&model.AdtySiteWalletLedger{}).Where("ref_id = ? AND ref_type IN ?", order.TradeNo, []string{"topup", "site_invite"})
		if order.SiteId == nil {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("site_id = ?", *order.SiteId)
		}
		return q, "id, 'Site wallet movement' AS kind, wallet_kind AS source, type AS status, ref_id AS reference, delta AS amount, balance_before, balance_after, reason AS note, operator_id, created_at", true
	case "exceptions":
		return model.DB.Model(&model.PaymentOrphanEvent{}).Where("reference_id = ? AND provider = ?", order.TradeNo, order.PaymentProvider),
			"id, 'Reconciliation event' AS kind, event_type AS source, status, event_id AS reference, create_time AS created_at", true
	case "reviews":
		return model.DB.Model(&model.PaymentOrphanReview{}).Where("trade_no = ? AND provider = ?", order.TradeNo, order.PaymentProvider),
			"id, 'Financial review' AS kind, status, external_amount AS amount, external_currency AS currency, refund_ledger_id AS reference, operator_id, note, created_at", true
	case "commissions":
		if ownerSiteID != nil {
			return nil, "", false
		}
		return model.DB.Model(&model.ReferralCommission{}).Where("source_type = ? AND source_trade_no = ? AND source_order_id = ? AND invitee_user_id = ?", model.ReferralSourceTopup, order.TradeNo, order.Id, order.UserId),
			"id, 'Commission' AS kind, status, commission_amount AS amount, settlement_currency AS currency, source_trade_no AS reference, created_at", true
	case "commission_movements":
		if ownerSiteID != nil {
			return nil, "", false
		}
		ids := model.DB.Model(&model.ReferralCommission{}).Select("id").Where("source_type = ? AND source_trade_no = ? AND source_order_id = ? AND invitee_user_id = ?", model.ReferralSourceTopup, order.TradeNo, order.Id, order.UserId)
		return model.DB.Model(&model.ReferralCommissionLedger{}).Where("commission_id IN (?)", ids),
			"id, 'Commission wallet movement' AS kind, type AS status, ref_id AS reference, settlement_currency AS currency, delta_pending AS pending_delta, delta_available AS available_delta, delta_frozen AS frozen_delta, delta_withdrawn AS withdrawn_delta, remark AS note, created_at", true
	}
	return nil, "", false
}
