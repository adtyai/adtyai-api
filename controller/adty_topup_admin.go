package controller

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// QueryAdtyTopUp may credit a verified payment, so its route requires the same
// explicit write permission as manual completion. Provider errors may contain
// credential-bearing URLs and must not reach API responses or audit metadata.
func QueryAdtyTopUp(c *gin.Context) {
	tradeNo := c.Param("trade_no")
	var topUp model.TopUp
	if err := model.DB.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = model.ErrTopUpNotFound
		}
		writeAdtyFinanceError(c, err)
		return
	}
	ext, err := model.GetAdtyTopupOrder(tradeNo)
	if err != nil {
		writeAdtyFinanceError(c, err)
		return
	}
	if ext == nil || !model.IsAdtyPaymentProvider(topUp.PaymentProvider) || topUp.PaymentProvider != ext.Provider || topUp.PaymentMethod != ext.Method || topUp.UserId != ext.UserId {
		writeAdtyFinanceError(c, model.ErrPaymentMethodMismatch)
		return
	}
	if _, ok := service.AdtyPaymentProviderByName(ext.Provider).(service.AdtyPaymentOrderQuerier); !ok {
		common.ApiErrorT(c, "This payment provider does not support order queries")
		return
	}
	switch topUp.Status {
	case common.TopUpStatusSuccess:
		// A repeated request observes the committed credit without querying or
		// applying another payment to an already completed order.
	case common.TopUpStatusPending, common.TopUpStatusExpired, common.TopUpStatusFailed, model.TopUpStatusCanceled:
		if err := model.UpdateAdtyTopupOrderFields(tradeNo, map[string]any{"last_query_at": common.GetTimestamp()}); err != nil {
			common.ApiError(c, err)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		err := service.RecoverAdtyHostedPaymentAs(ctx, ext, c.GetInt("id"))
		cancel()
		if err != nil {
			common.ApiErrorT(c, "Payment query failed. Check the provider console or try again.")
			return
		}
	default:
		writeAdtyFinanceError(c, model.ErrTopUpStatusInvalid)
		return
	}
	if err := model.DB.Select("status").Where("id = ?", topUp.Id).First(&topUp).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, topUp.UserId, "adty.topup.query", map[string]any{
		"trade_no": tradeNo, "provider": ext.Provider, "status": topUp.Status,
	})
	common.ApiSuccess(c, gin.H{"status": topUp.Status})
}

// GetPaymentOrphans 分页列出对不上的支付回调（任务 3.5）。
func GetPaymentOrphans(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	events, total, err := model.ListPaymentOrphanEvents(c.Query("status"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(events)
	common.ApiSuccess(c, pageInfo)
}

// adtyFactsFromOrphan 按支付商解析 orphan 里存的回调参数，字段映射和回调相同。
func adtyFactsFromOrphan(orphan *model.PaymentOrphanEvent, method string) (model.AdtyPaymentFacts, error) {
	payload := map[string]any{}
	if err := common.UnmarshalJsonStr(orphan.Payload, &payload); err != nil {
		return model.AdtyPaymentFacts{}, model.ErrPaymentOrphanPayloadInvalid
	}
	get := func(key string) string { return model.PaymentOrphanPayloadString(payload, key) }
	parse := func(key string) (decimal.Decimal, error) {
		value, err := decimal.NewFromString(get(key))
		if err != nil {
			return decimal.Zero, model.ErrPaymentOrphanPayloadInvalid
		}
		return value, nil
	}
	facts := model.AdtyPaymentFacts{Provider: orphan.Provider, Method: method, Payload: orphan.Payload, Source: model.AdtyPaymentSourceCallback}
	switch orphan.Provider {
	case model.PaymentProviderAdtyBEpusdt:
		paid, err := parse("actual_amount")
		if err != nil {
			return facts, err
		}
		fiat, err := parse("amount")
		if err != nil {
			return facts, err
		}
		facts.PaidAmount, facts.FiatAmount, facts.PaidCurrency = paid, fiat, "USDT"
		facts.PayAddress = get("token")
		facts.GatewayTradeId = get("trade_id")
		facts.TxHash = get("block_transaction_id")
	case model.PaymentProviderAdtyEpay:
		paid, err := parse("money")
		if err != nil {
			return facts, err
		}
		facts.PaidAmount, facts.PaidCurrency = paid, "CNY"
		facts.GatewayTradeId = get("trade_no")
		facts.TxHash = get("trade_no")
	default:
		return facts, model.ErrPaymentOrphanNotCredit
	}
	return facts, nil
}

// CreditPaymentOrphan 按 orphan 里的回调参数补入账：金额照常核对，和标记已处理在同一个事务里，
// 加赠和返佣钩子都会执行。金额不符的不能在这里补，走手动补单。
func CreditPaymentOrphan(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = common.DecodeJson(c.Request.Body, &body)
	var topUp *model.TopUp
	var ext *model.AdtyTopupOrder
	var credited int
	var alreadyDone bool
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		orphan, _, _, err := model.LockPaymentOrphanOrderTx(tx, id)
		if err != nil {
			return err
		}
		if !model.CanCreditPaymentOrphanEvent(orphan) {
			return model.ErrPaymentOrphanNotCredit
		}
		var topUps []model.TopUp
		if err := tx.Select("payment_method").Where("trade_no = ?", orphan.ReferenceID).Limit(1).Find(&topUps).Error; err != nil {
			return err
		}
		if len(topUps) == 0 {
			return model.ErrTopUpNotFound
		}
		facts, err := adtyFactsFromOrphan(orphan, topUps[0].PaymentMethod)
		if err != nil {
			return err
		}
		facts.OperatorId = c.GetInt("id")
		alreadyDone, credited, topUp, ext, err = model.CompleteAdtyTopUpTx(tx, orphan.ReferenceID, facts)
		if err != nil {
			return err
		}
		return model.MarkPaymentOrphanCreditedTx(tx, orphan, c.GetInt("id"), body.Note)
	})
	if err != nil {
		writeAdtyFinanceError(c, err)
		return
	}
	if !alreadyDone {
		model.FinishAdtyTopUpCredit(topUp, ext, credited, adtyCallerIP(c))
	}
	recordManageAudit(c, "adty.payment_orphan.credit", map[string]any{"id": id, "note": strings.TrimSpace(body.Note)})
	common.ApiSuccess(c, nil)
}

// ResolvePaymentOrphan records verified external facts and an audited disposition.
func ResolvePaymentOrphan(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body model.PaymentOrphanReviewRequest
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	review, err := model.ResolvePaymentOrphanReview(id, c.GetInt("id"), body)
	if err != nil {
		writeAdtyFinanceError(c, err)
		return
	}
	recordManageAudit(c, "adty.payment_orphan.resolve", map[string]any{"id": id, "review_id": review.ID, "status": review.Status})
	common.ApiSuccess(c, review)
}

// ManualCompleteAdtyTopUp 手动补单：管理员在支付商后台核实到账后操作，必须填凭证和备注。
// 跳过金额核对，按下单时的面额加赠入账，实付写管理员填的凭证。
func ManualCompleteAdtyTopUp(c *gin.Context) {
	var body struct {
		Proof string `json:"proof"`
		Note  string `json:"note"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	body.Proof = strings.TrimSpace(body.Proof)
	body.Note = strings.TrimSpace(body.Note)
	if body.Proof == "" || body.Note == "" {
		common.ApiErrorT(c, "Proof and note are required")
		return
	}
	tradeNo := c.Param("trade_no")
	if middleware.RequireSecurityProof(c, service.AdtyTopUpManualCompleteOperation(tradeNo, body.Proof, body.Note)) == nil {
		return
	}
	var topUps []model.TopUp
	if err := model.DB.Where("trade_no = ?", tradeNo).Limit(1).Find(&topUps).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if len(topUps) == 0 {
		writeAdtyFinanceError(c, model.ErrTopUpNotFound)
		return
	}
	facts := model.AdtyPaymentFacts{
		Provider:   topUps[0].PaymentProvider,
		Method:     topUps[0].PaymentMethod,
		TxHash:     body.Proof,
		Source:     model.AdtyPaymentSourceManual,
		Payload:    common.GetJsonString(map[string]any{"proof": body.Proof, "note": body.Note, "operator": c.GetInt("id")}),
		OperatorId: c.GetInt("id"),
	}
	var topUp *model.TopUp
	var ext *model.AdtyTopupOrder
	var credited int
	var alreadyDone bool
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		alreadyDone, credited, topUp, ext, err = model.CompleteAdtyTopUpTx(tx, tradeNo, facts)
		if err != nil || alreadyDone {
			return err
		}
		// 实付没有网关数据，记成应付，币种照下单时。
		return tx.Model(&model.AdtyTopupOrder{}).Where("id = ?", ext.Id).Updates(map[string]any{
			"paid_amount":   ext.PayAmount,
			"paid_currency": ext.PayCurrency,
		}).Error
	})
	if err != nil {
		writeAdtyFinanceError(c, err)
		return
	}
	if !alreadyDone {
		ext.PaidAmount, ext.PaidCurrency = ext.PayAmount, ext.PayCurrency
		model.FinishAdtyTopUpCredit(topUp, ext, credited, adtyCallerIP(c))
	}
	recordManageAuditFor(c, topUps[0].UserId, "adty.topup.manual_complete", map[string]any{
		"trade_no": tradeNo, "proof": body.Proof, "note": body.Note, "already_done": alreadyDone,
	})
	common.ApiSuccess(c, gin.H{"already_done": alreadyDone})
}

// RefundAdtyTopUp 退款：按当前余额扣回这笔订单还剩的额度，赠金不退，只退没花掉的本金；佣金按比例扣回。
// 返回扣回额度、退的本金额度和应退金额，真正退钱在线下做。
func PreviewAdtyTopUpRefund(c *gin.Context) {
	tradeNo := c.Param("trade_no")
	result, err := model.PreviewAdtyTopUpRefund(tradeNo)
	if err != nil {
		writeAdtyFinanceError(c, err)
		return
	}
	if raw := strings.TrimSpace(c.Query("refund_usd")); raw != "" {
		request, parseErr := decimal.NewFromString(raw)
		if parseErr != nil || !request.IsPositive() {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		facts, factErr := model.LoadAdtyRefundPageFacts(tradeNo, request)
		if factErr != nil {
			writeAdtyFinanceError(c, factErr)
			return
		}
		result.Facts = facts
		if facts != nil && facts.Commission != nil {
			result.Commission = facts.Commission
			result.CommissionIssuedUSD = facts.Commission.IssuedUSD
			result.CommissionWithdrawnUSD = facts.Commission.WithdrawnUSD
			result.CommissionClawbackUSD = facts.Commission.ExpectedClawbackUSD
		}
	}
	common.ApiSuccess(c, result)
}

func RefundAdtyTopUp(c *gin.Context) {
	var body struct {
		Reason         string `json:"reason"`
		IdempotencyKey string `json:"idempotency_key"`
		RefundQuota    *int64 `json:"refund_quota"`
		RefundUSD      string `json:"refund_usd"`
		Proof          string `json:"proof"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	tradeNo := c.Param("trade_no")
	refundUSD := strings.TrimSpace(body.RefundUSD)
	var refundQuota *int64
	if refundUSD != "" {
		if _, parseErr := decimal.NewFromString(refundUSD); parseErr != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	} else if body.RefundQuota != nil {
		refundQuota = body.RefundQuota
	} else {
		common.ApiErrorT(c, "Enter the refund amount in USD")
		return
	}
	if middleware.RequireSecurityProof(c, service.AdtyTopUpRefundOperation(tradeNo, body.Reason, refundUSD, refundQuota)) == nil {
		return
	}
	var result *model.AdtyTopUpRefundResult
	var err error
	if refundUSD != "" {
		amount, parseErr := decimal.NewFromString(refundUSD)
		if parseErr != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		applied, applyErr := model.ApplyAdtyTopUpManualRefund(tradeNo, body.Reason, c.GetInt("id"), body.IdempotencyKey, amount)
		if applyErr != nil {
			writeAdtyFinanceError(c, applyErr)
			return
		}
		recordManageAuditFor(c, applied.Refund.UserId, "adty.topup.refund.apply", map[string]any{
			"trade_no": tradeNo, "refund_id": applied.Refund.RefundId,
			"reason":        strings.TrimSpace(body.Reason),
			"request_usd":   applied.Refund.RequestUSD.String(),
			"request_quota": applied.Refund.RequestQuota,
		})
		common.ApiSuccess(c, applied)
		return
	}
	result, err = model.RefundAdtyTopUp(tradeNo, body.Reason, c.GetInt("id"), body.IdempotencyKey, *refundQuota)
	if err != nil {
		writeAdtyFinanceError(c, err)
		return
	}
	recordManageAuditFor(c, result.Ledger.UserId, "adty.topup.refund", map[string]any{
		"trade_no": tradeNo, "reason": strings.TrimSpace(body.Reason),
		"deducted_quota": result.DeductedQuota, "refund_quota": result.RefundQuota,
		"refund_amount": result.RefundAmount.String(), "currency": result.Currency,
	})
	common.ApiSuccess(c, result)
}

func ListAdtyTopUpManualRefunds(c *gin.Context) {
	result, err := model.ListAdtyTopUpManualRefunds(c.Param("trade_no"))
	if err != nil {
		writeAdtyFinanceError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetAdtyTopUpManualRefund(c *gin.Context) {
	result, err := model.GetAdtyTopUpManualRefund(c.Param("trade_no"), c.Param("refund_id"))
	if err != nil {
		writeAdtyFinanceError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func writeManualRefundStep(c *gin.Context, step string, run func(tradeNo, refundID string, operatorID int, idemKey string) (*model.AdtyManualRefundResult, error)) {
	var body struct {
		IdempotencyKey string `json:"idempotency_key"`
		Note           string `json:"note"`
		Status         string `json:"status"`
		Currency       string `json:"currency"`
		Amount         string `json:"amount"`
		PaidAt         int64  `json:"paid_at"`
		Voucher        string `json:"voucher"`
		Proof          string `json:"proof"`
	}
	if c.Request.ContentLength > 0 {
		if err := common.DecodeJson(c.Request.Body, &body); err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	}
	tradeNo := c.Param("trade_no")
	refundID := c.Param("refund_id")
	payout := model.AdtyManualRefundPayout{
		Status:   body.Status,
		Currency: body.Currency,
		PaidAt:   body.PaidAt,
		Voucher:  strings.TrimSpace(body.Voucher),
	}
	if payout.Voucher == "" {
		payout.Voucher = strings.TrimSpace(body.Proof)
	}
	if strings.TrimSpace(body.Amount) != "" {
		amount, err := decimal.NewFromString(strings.TrimSpace(body.Amount))
		if err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		payout.Amount = amount
	}
	amount := ""
	if strings.TrimSpace(body.Amount) != "" {
		amount = payout.Amount.String()
	}
	op := service.AdtyTopUpManualRefundStepOperation(tradeNo, refundID, step, body.Note, service.AdtyManualRefundStepPayout{
		Status: payout.Status, Currency: payout.Currency, Amount: amount,
		Voucher: payout.Voucher, PaidAt: payout.PaidAt,
	})
	if middleware.RequireSecurityProof(c, op) == nil {
		return
	}
	var result *model.AdtyManualRefundResult
	var err error
	switch step {
	case model.AdtyManualRefundStepPayout:
		result, err = model.RecordAdtyTopUpManualRefundPayout(tradeNo, refundID, c.GetInt("id"), body.IdempotencyKey, payout)
	case model.AdtyManualRefundStepApprove:
		result, err = model.ApproveAdtyTopUpManualRefund(tradeNo, refundID, c.GetInt("id"), body.IdempotencyKey, body.Note)
	case model.AdtyManualRefundStepTerminate:
		result, err = model.TerminateAdtyTopUpManualRefund(tradeNo, refundID, c.GetInt("id"), body.IdempotencyKey, body.Note)
	default:
		result, err = run(tradeNo, refundID, c.GetInt("id"), body.IdempotencyKey)
	}
	if err != nil {
		writeAdtyFinanceError(c, err)
		return
	}
	audit := map[string]any{"trade_no": tradeNo, "refund_id": refundID, "step": step}
	if result.Refund != nil {
		audit["status"] = result.Refund.Status
		audit["shortfall_quota"] = result.Refund.ShortfallQuota
		audit["payout_status"] = result.Refund.PayoutStatus
	}
	recordManageAuditFor(c, result.Refund.UserId, "adty.topup.refund."+step, audit)
	common.ApiSuccess(c, result)
}

func ApproveAdtyTopUpManualRefund(c *gin.Context) {
	writeManualRefundStep(c, model.AdtyManualRefundStepApprove, nil)
}

func ReclaimAdtyTopUpManualRefund(c *gin.Context) {
	writeManualRefundStep(c, model.AdtyManualRefundStepReclaim, model.ReclaimAdtyTopUpManualRefund)
}

func PlanAdtyTopUpManualRefundBonus(c *gin.Context) {
	writeManualRefundStep(c, model.AdtyManualRefundStepBonus, model.PlanAdtyTopUpManualRefundBonus)
}

func PlanAdtyTopUpManualRefundCommission(c *gin.Context) {
	writeManualRefundStep(c, model.AdtyManualRefundStepCommission, model.PlanAdtyTopUpManualRefundCommission)
}

func RecordAdtyTopUpManualRefundPayout(c *gin.Context) {
	writeManualRefundStep(c, model.AdtyManualRefundStepPayout, nil)
}

func CompleteAdtyTopUpManualRefund(c *gin.Context) {
	writeManualRefundStep(c, model.AdtyManualRefundStepComplete, model.CompleteAdtyTopUpManualRefund)
}

func TerminateAdtyTopUpManualRefund(c *gin.Context) {
	writeManualRefundStep(c, model.AdtyManualRefundStepTerminate, nil)
}

func writeAdtyFinanceError(c *gin.Context, err error) {
	var message *common.Message
	switch {
	case errors.As(err, &message):
		common.ApiError(c, err)
	case errors.Is(err, model.ErrTopUpNotFound), errors.Is(err, model.ErrPaymentOrphanNotFound):
		common.ApiErrorT(c, "Order not found")
	case errors.Is(err, model.ErrPaymentOrphanNotCredit):
		common.ApiErrorT(c, "This event cannot be credited automatically. Check the payment and complete the order manually")
	case errors.Is(err, model.ErrPaymentOrphanPayloadInvalid):
		common.ApiErrorT(c, "The callback data of this event is incomplete")
	case errors.Is(err, model.ErrPaymentAmountMismatch):
		common.ApiErrorT(c, "The paid amount does not match the order")
	case errors.Is(err, model.ErrPaymentMethodMismatch):
		common.ApiErrorT(c, "The payment provider does not match the order")
	case errors.Is(err, model.ErrTopUpStatusInvalid):
		common.ApiErrorT(c, "The order status does not allow this operation")
	case errors.Is(err, model.ErrAdtyIdempotencyConflict):
		common.ApiErrorT(c, "This request was already used for another operation")
	case errors.Is(err, model.ErrAdtyManualRefundTableMissing):
		common.ApiErrorT(c, "Manual refund records are unavailable")
	default:
		common.ApiError(c, err)
	}
}
