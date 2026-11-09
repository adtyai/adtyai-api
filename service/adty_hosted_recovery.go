package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	pancake "github.com/waffo-com/waffo-pancake-sdk-go"
	"gorm.io/gorm"
)

type AdtyPaymentOrderQuerier interface {
	Query(context.Context, *model.AdtyTopupOrder) (*model.AdtyPaymentFacts, error)
}

// QueryWaffoPancakePayment first finds the payment by the merchant's original
// business number, because authenticated checkout does not return an order ID.
// It then verifies that order's store, product, environment and price snapshot.
func QueryWaffoPancakePayment(ctx context.Context, ext *model.AdtyTopupOrder) (*model.AdtyPaymentFacts, error) {
	client, err := newWaffoPancakeClient()
	if err != nil {
		return nil, errors.New("original Pancake credentials are unavailable")
	}
	var paymentResult struct {
		Payments []struct {
			ID                      string `json:"id"`
			OrderID                 string `json:"orderId"`
			Status                  string `json:"status"`
			RefundStatus            string `json:"refundStatus"`
			OrderMerchantExternalID string `json:"orderMerchantExternalId"`
		} `json:"payments"`
	}
	result, err := client.GraphQL.Query(ctx, pancake.GraphQLParams{
		Query:     `query ($ref: String!) { payments(filter: { orderMerchantExternalId: { eq: $ref } }) { id orderId status refundStatus orderMerchantExternalId } }`,
		Variables: map[string]any{"ref": ext.TradeNo},
	})
	if err != nil || result == nil || len(result.Errors) > 0 || common.Unmarshal(result.Data, &paymentResult) != nil {
		return nil, errors.New("Pancake payment query failed")
	}
	orderID := ""
	for _, payment := range paymentResult.Payments {
		if payment.OrderMerchantExternalID != ext.TradeNo {
			return nil, model.ErrPaymentMethodMismatch
		}
		if payment.Status != "succeeded" {
			continue
		}
		if payment.OrderID == "" || orderID != "" || (payment.RefundStatus != "" && payment.RefundStatus != "none") {
			return nil, errors.New("Pancake payment requires reconciliation")
		}
		orderID = payment.OrderID
	}
	if orderID == "" {
		return nil, nil
	}
	var orderResult struct {
		Order *struct {
			ID                      string `json:"id"`
			OrderMerchantExternalID string `json:"orderMerchantExternalId"`
			TestMode                *bool  `json:"testMode"`
			Currency                string `json:"currency"`
			Store                   struct {
				ID string `json:"id"`
			} `json:"store"`
			Product struct {
				ID string `json:"id"`
			} `json:"onetimeProduct"`
			Price struct {
				Currency string `json:"currency"`
				Subtotal string `json:"subtotal"`
				Tax      string `json:"taxAmount"`
				Total    string `json:"total"`
			} `json:"priceSnapshot"`
		} `json:"onetimeOrder"`
	}
	result, err = client.GraphQL.Query(ctx, pancake.GraphQLParams{
		Query:     `query ($id: ID!) { onetimeOrder(id: $id) { id orderMerchantExternalId testMode currency store { id } onetimeProduct { id } priceSnapshot { currency subtotal taxAmount total } } }`,
		Variables: map[string]any{"id": orderID},
	})
	if err != nil || result == nil || len(result.Errors) > 0 || common.Unmarshal(result.Data, &orderResult) != nil || orderResult.Order == nil {
		return nil, errors.New("Pancake order query failed")
	}
	purchase := orderResult.Order
	if purchase.ID != orderID || purchase.OrderMerchantExternalID != ext.TradeNo || purchase.TestMode == nil || purchase.Store.ID != ext.ProviderMerchantId || purchase.Product.ID != ext.ProviderProductId || (ext.GatewayTradeId != "" && ext.GatewayTradeId != orderID) {
		return nil, model.ErrPaymentMethodMismatch
	}
	environment := "prod"
	if *purchase.TestMode {
		environment = "test"
	}
	subtotal, subtotalErr := decimal.NewFromString(purchase.Price.Subtotal)
	tax, taxErr := decimal.NewFromString(purchase.Price.Tax)
	total, totalErr := decimal.NewFromString(purchase.Price.Total)
	if subtotalErr != nil || taxErr != nil || totalErr != nil || !subtotal.IsPositive() || tax.IsNegative() || !total.IsPositive() || !subtotal.Add(tax).Equal(total) || purchase.Currency != purchase.Price.Currency {
		return nil, model.ErrPaymentAmountMismatch
	}
	return &model.AdtyPaymentFacts{
		Provider: ext.Provider, Method: ext.Method, PaidAmount: total, PurchaseAmount: subtotal, PaidCurrency: purchase.Currency,
		ProductId: purchase.Product.ID, MerchantId: purchase.Store.ID, Environment: environment,
		GatewayTradeId: orderID, TxHash: orderID, Source: model.AdtyPaymentSourceQuery, Payload: string(result.Data),
	}, nil
}

// RecoverAdtyHostedPayment shares the callback's transaction and immutable
// credit ledger. A successful retry also closes its pending query review.
func RecoverAdtyHostedPayment(ctx context.Context, ext *model.AdtyTopupOrder) error {
	return RecoverAdtyHostedPaymentAs(ctx, ext, 0)
}

// RecoverAdtyHostedPaymentAs records the authenticated administrator for manual
// queries; background recovery retains actor zero.
func RecoverAdtyHostedPaymentAs(ctx context.Context, ext *model.AdtyTopupOrder, operatorID int) error {
	if ext == nil {
		return model.ErrTopUpNotFound
	}
	provider, ok := AdtyPaymentProviderByName(ext.Provider).(AdtyPaymentOrderQuerier)
	if !ok {
		return errors.New("payment provider does not support order queries")
	}
	facts, err := provider.Query(ctx, ext)
	if err != nil {
		if recordErr := model.RecordAdtyPaymentAttempt(ext.TradeNo, ext.Provider, model.AdtyPaymentSourceQuery, "failed", operatorID); recordErr != nil {
			return recordErr
		}
		return err
	}
	if facts == nil {
		return model.RecordAdtyPaymentAttempt(ext.TradeNo, ext.Provider, model.AdtyPaymentSourceQuery, "unpaid", operatorID)
	}
	facts.Source = model.AdtyPaymentSourceQuery
	facts.OperatorId = operatorID
	var topUp *model.TopUp
	var frozen *model.AdtyTopupOrder
	var credited int
	var alreadyDone bool
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		alreadyDone, credited, topUp, frozen, err = model.CompleteAdtyTopUpTx(tx, ext.TradeNo, *facts)
		if err != nil {
			return err
		}
		return tx.Model(&model.PaymentOrphanEvent{}).Where("event_id = ? AND status = ?", "hosted-query:"+ext.TradeNo, model.PaymentOrphanStatusPendingReview).Updates(map[string]any{
			"status": model.PaymentOrphanStatusCredited, "resolution": model.PaymentOrphanStatusCredited,
			"resolution_note": "Recovered by authenticated provider query", "resolved_at": common.GetTimestamp(), "update_time": common.GetTimestamp(),
		}).Error
	})
	if err != nil {
		if recordErr := model.RecordAdtyPaymentAttempt(ext.TradeNo, ext.Provider, model.AdtyPaymentSourceQuery, "rejected", operatorID); recordErr != nil {
			return recordErr
		}
	}
	if err == nil && !alreadyDone {
		model.FinishAdtyTopUpCredit(topUp, frozen, credited, "")
	}
	return err
}

// The scan is bounded to twenty orders per minute and a seven-day recovery
// window. Older orders remain available for audited manual completion.
func recoverAdtyHostedPayments(now int64) {
	var orders []model.AdtyTopupOrder
	err := model.DB.Table("adty_topup_orders AS a").Select("a.*").Joins("JOIN top_ups t ON t.trade_no = a.trade_no").
		Where("a.provider IN ? AND a.created_at <= ? AND a.created_at >= ? AND a.last_query_at <= ? AND t.status IN ?", []string{
			model.PaymentProviderStripe, model.PaymentProviderCreem, model.PaymentProviderWaffo, model.PaymentProviderWaffoPancake, model.PaymentProviderAdtyBEpusdt,
		}, now-120, now-7*24*60*60, now-60, []string{common.TopUpStatusPending, common.TopUpStatusExpired, common.TopUpStatusFailed, model.TopUpStatusCanceled}).
		Order("a.last_query_at, a.id").Limit(20).Scan(&orders).Error
	if err != nil {
		common.SysError(common.LogText("hosted payment recovery scan failed: %s", err.Error()))
		return
	}
	for _, ext := range orders {
		claim := model.DB.Model(&model.AdtyTopupOrder{}).Where("id = ? AND last_query_at <= ?", ext.Id, now-60).Update("last_query_at", now)
		if claim.Error != nil || claim.RowsAffected != 1 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		err := RecoverAdtyHostedPayment(ctx, &ext)
		cancel()
		if err != nil {
			if reviewErr := model.RecordPaymentOrphanEvent(&model.PaymentOrphanEvent{
				Provider: ext.Provider, EventID: "hosted-query:" + ext.TradeNo, EventType: "payment_query", ReferenceID: ext.TradeNo, SessionID: ext.GatewayTradeId,
				Reason: "Payment recovery requires reconciliation", Error: err.Error(),
			}); reviewErr != nil {
				common.SysError(common.LogText("hosted payment recovery review failed: %s", reviewErr.Error()))
			}
		}
	}
}

func init() {
	RegisterAdtyMaintenanceHook("hosted-payment-recovery", recoverAdtyHostedPayments)
}
