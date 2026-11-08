package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPaymentOrphanCannotBeMarkedRefundedWithoutFinancialEvidence(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "orphan-proof", 0)
	seedAdtyOrder(t, user.Id, "orphan-proof", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	event := PaymentOrphanEvent{Provider: PaymentProviderAdtyBEpusdt, ReferenceID: "orphan-proof", EventID: "reversal:proof", EventType: "refund.completed"}
	require.NoError(t, RecordPaymentOrphanEvent(&event))
	require.Error(t, MarkPaymentOrphanResolved(event.ID, PaymentOrphanStatusRefunded, 1, "operator clicked refunded"))
	var stored PaymentOrphanEvent
	require.NoError(t, db.First(&stored, event.ID).Error)
	require.Equal(t, PaymentOrphanStatusPendingReview, stored.Status)
}

func TestPaymentOrphanRefundUsesOriginalLedgerAndRecordsUncoveredCash(t *testing.T) {
	for _, balance := range []int{40_000_000, 0} {
		t.Run(fmt.Sprint(balance), func(t *testing.T) {
			db := prepareAdtyTopUpDB(t)
			user := seedAdtyTopUpUser(t, db, "external-refund", balance)
			seedCreditedOrder(t, db, user.Id, "external-refund", PaymentProviderAdtyBEpusdt, "100", "10", "0", "90")
			event := PaymentOrphanEvent{Provider: PaymentProviderAdtyBEpusdt, ReferenceID: "external-refund", EventID: "reversal:external-refund", EventType: "refund.completed"}
			require.NoError(t, RecordPaymentOrphanEvent(&event))
			request := PaymentOrphanReviewRequest{Status: PaymentOrphanStatusRefunded, TradeNo: event.ReferenceID, ExternalAmount: "90", ExternalCurrency: "USDT", Proof: "provider-refund-receipt", Note: "verified cumulative full reversal, spent principal is not recovered"}
			_, err := ResolvePaymentOrphanReview(event.ID, 1, request)
			require.Error(t, err, "provider reversal alone must not pretend a wallet refund happened")
			preview, err := PreviewAdtyTopUpRefund(event.ReferenceID)
			require.NoError(t, err)
			refund, err := RefundAdtyTopUp(event.ReferenceID, "external reversal", 1, "external-original-refund", preview.RefundQuota)
			require.NoError(t, err)
			for _, field := range []string{"amount", "currency", "order", "proof", "note"} {
				invalid := request
				switch field {
				case "amount":
					invalid.ExternalAmount = "45"
				case "currency":
					invalid.ExternalCurrency = "USD"
				case "order":
					invalid.TradeNo = "another-order"
				case "proof":
					invalid.Proof = ""
				case "note":
					invalid.Note = ""
				}
				_, err := ResolvePaymentOrphanReview(event.ID, 1, invalid)
				require.Error(t, err, field)
			}
			review, err := ResolvePaymentOrphanReview(event.ID, 1, request)
			require.NoError(t, err)
			require.Equal(t, refund.Ledger.Id, review.RefundLedgerID)
			require.Equal(t, refund.DeductedQuota, review.DeductedQuota)
			require.Equal(t, refund.RefundQuota, review.RefundQuota)
			require.True(t, refund.RefundAmount.Equal(review.RefundAmount))
			require.True(t, decimal.NewFromInt(90).Sub(refund.RefundAmount).Equal(review.ExposureAmount))
			require.Zero(t, userQuota(t, db, user.Id))
			again, err := ResolvePaymentOrphanReview(event.ID, 2, request)
			require.NoError(t, err)
			require.JSONEq(t, common.GetJsonString(review), common.GetJsonString(again), "a repeated decision preserves its original operator and time")
			request.Proof = "different-provider-receipt"
			_, err = ResolvePaymentOrphanReview(event.ID, 1, request)
			require.ErrorIs(t, err, ErrAdtyIdempotencyConflict)
			var count int64
			require.NoError(t, db.Model(&PaymentOrphanReview{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
			events, _, err := ListPaymentOrphanEvents("all", &common.PageInfo{Page: 1, PageSize: 20})
			require.NoError(t, err)
			require.Len(t, events, 1)
			require.JSONEq(t, common.GetJsonString(review), common.GetJsonString(events[0].Review))
		})
	}
}

func TestPaymentOrphanPartialReviewIsImmutableAndDoesNotClawBackWallet(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "partial-external", 40_000_000)
	seedCreditedOrder(t, db, user.Id, "partial-external", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
	event := PaymentOrphanEvent{Provider: PaymentProviderAdtyBEpusdt, ReferenceID: "partial-external", EventID: "reversal:partial-external", EventType: "refund.completed"}
	require.NoError(t, RecordPaymentOrphanEvent(&event))
	request := PaymentOrphanReviewRequest{Status: PaymentOrphanStatusPendingReview, TradeNo: event.ReferenceID, ExternalAmount: "20", ExternalCurrency: "USDT", Proof: "partial-refund-20", Note: "partial refund requires reconciliation"}
	first, err := ResolvePaymentOrphanReview(event.ID, 1, request)
	require.NoError(t, err)
	request.ExternalAmount, request.Proof = "30", "partial-refund-30"
	second, err := ResolvePaymentOrphanReview(event.ID, 1, request)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	var preserved PaymentOrphanReview
	require.NoError(t, db.First(&preserved, first.ID).Error)
	require.JSONEq(t, common.GetJsonString(first), common.GetJsonString(&preserved))
	require.Equal(t, 40_000_000, userQuota(t, db, user.Id))
	require.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, event.ReferenceID))
	var stored PaymentOrphanEvent
	require.NoError(t, db.First(&stored, event.ID).Error)
	require.Equal(t, PaymentOrphanStatusPendingReview, stored.Status)
	require.Zero(t, stored.ResolvedAt)
}

func TestPaymentOrphanNeverCreditedPreservesFenceAndInvalidDismissalCannotReleaseIt(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "never-credited", 0)
	seedAdtyOrder(t, user.Id, "never-credited", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	event := PaymentOrphanEvent{Provider: PaymentProviderAdtyBEpusdt, ReferenceID: "never-credited", EventID: "reversal:never", EventType: "refund.completed"}
	require.NoError(t, RecordPaymentOrphanEvent(&event))
	request := PaymentOrphanReviewRequest{Status: PaymentOrphanStatusDismissed, TradeNo: event.ReferenceID, Proof: "provider-reversal", Note: "not an authorization to unblock"}
	_, err := ResolvePaymentOrphanReview(event.ID, 1, request)
	require.Error(t, err)
	request.Status, request.ExternalAmount, request.ExternalCurrency = PaymentOrphanStatusNeverCredited, "50.01", "USDT"
	_, err = ResolvePaymentOrphanReview(event.ID, 1, request)
	require.NoError(t, err)
	_, err = CompleteAdtyTopUp(event.ReferenceID, usdtFacts(event.ReferenceID, "50.01"), "")
	require.ErrorIs(t, err, ErrPaymentOrphanNotCredit)
	require.Zero(t, userQuota(t, db, user.Id))
	request.Status, request.DismissalReason = PaymentOrphanStatusDismissed, "reversal_canceled"
	_, err = ResolvePaymentOrphanReview(event.ID, 1, request)
	require.ErrorIs(t, err, ErrAdtyIdempotencyConflict)
}

func TestPaymentOrphanReviewRollbackKeepsEvidenceAndStateAtomic(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	event := PaymentOrphanEvent{Provider: PaymentProviderStripe, ReferenceID: "missing-order", EventID: "reversal:missing", EventType: "dispute.open"}
	require.NoError(t, RecordPaymentOrphanEvent(&event))
	const callback = "orphan_resolution_failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "adty_payment_orphan_events" {
			tx.AddError(gorm.ErrInvalidTransaction)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callback) })
	_, err := ResolvePaymentOrphanReview(event.ID, 1, PaymentOrphanReviewRequest{Status: PaymentOrphanStatusPendingReview, TradeNo: event.ReferenceID, ExternalAmount: "0", ExternalCurrency: "USD", Proof: "dispute-open", Note: "not yet an actual reversal"})
	require.ErrorIs(t, err, gorm.ErrInvalidTransaction)
	var count int64
	require.NoError(t, db.Model(&PaymentOrphanReview{}).Count(&count).Error)
	require.Zero(t, count)
	var stored PaymentOrphanEvent
	require.NoError(t, db.First(&stored, event.ID).Error)
	require.Zero(t, stored.ReviewID)
	require.Equal(t, PaymentOrphanStatusPendingReview, stored.Status)
}

func TestPaymentOrphanRefundRejectsUnrelatedLedgerAndUnboundedAmounts(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "refund-ledger-binding", 40_000_000)
	seedCreditedOrder(t, db, user.Id, "refund-ledger-binding", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
	event := PaymentOrphanEvent{Provider: PaymentProviderAdtyBEpusdt, ReferenceID: "refund-ledger-binding", EventID: "reversal:ledger-binding", EventType: "refund.completed"}
	require.NoError(t, RecordPaymentOrphanEvent(&event))
	refund, err := RefundAdtyTopUp(event.ReferenceID, "provider reversal", 1, "ledger-binding-refund", 35_000_000)
	require.NoError(t, err)
	request := PaymentOrphanReviewRequest{Status: PaymentOrphanStatusRefunded, TradeNo: event.ReferenceID, ExternalAmount: "100", ExternalCurrency: "USDT", Proof: "full-receipt", Note: "verified full reversal"}
	for _, field := range []string{"type", "ref_type", "ref_id", "user_id", "delta_quota"} {
		original := map[string]any{"type": refund.Ledger.Type, "ref_type": refund.Ledger.RefType, "ref_id": refund.Ledger.RefId, "user_id": refund.Ledger.UserId, "delta_quota": refund.Ledger.DeltaQuota}[field]
		invalid := map[string]any{"type": AdtyLedgerTypeTopUp, "ref_type": "manual", "ref_id": "unrelated-order", "user_id": user.Id + 1, "delta_quota": 1}[field]
		require.NoError(t, db.Model(&AdtyBalanceLedger{}).Where("id = ?", refund.Ledger.Id).Update(field, invalid).Error)
		_, err := ResolvePaymentOrphanReview(event.ID, 1, request)
		require.Error(t, err, field)
		require.NoError(t, db.Model(&AdtyBalanceLedger{}).Where("id = ?", refund.Ledger.Id).Update(field, original).Error)
	}
	for _, amount := range []string{"-1", "1e2147483647", "0.000000001", "1000000000000", "NaN"} {
		invalid := request
		invalid.ExternalAmount = amount
		_, err := ResolvePaymentOrphanReview(event.ID, 1, invalid)
		require.Error(t, err, amount)
	}
	_, err = ResolvePaymentOrphanReview(event.ID, 1, request)
	require.NoError(t, err)
}

func TestPaymentOrphanConcurrentNeverCreditAndPaymentRemainConsistent(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	if db.Dialector.Name() != "postgres" {
		t.Skip("row-lock race requires PostgreSQL")
	}
	user := seedAdtyTopUpUser(t, db, "never-credit-race", 0)
	seedAdtyOrder(t, user.Id, "never-credit-race", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	// An ordinary failed-delivery review has no pre-existing reversal fence.
	// Whichever operation locks the order first must exclude the other result.
	event := PaymentOrphanEvent{Provider: PaymentProviderAdtyBEpusdt, ReferenceID: "never-credit-race", EventID: "payment-review:race", EventType: "callback"}
	require.NoError(t, RecordPaymentOrphanEvent(&event))
	var wg sync.WaitGroup
	var reviewErr, creditErr error
	wg.Go(func() {
		_, reviewErr = ResolvePaymentOrphanReview(event.ID, 1, PaymentOrphanReviewRequest{Status: PaymentOrphanStatusNeverCredited, TradeNo: event.ReferenceID, ExternalAmount: "50.01", ExternalCurrency: "USDT", Proof: "race-receipt", Note: "confirmed refund"})
	})
	wg.Go(func() { _, creditErr = CompleteAdtyTopUp(event.ReferenceID, usdtFacts(event.ReferenceID, "50.01"), "") })
	wg.Wait()
	if reviewErr == nil {
		require.ErrorIs(t, creditErr, ErrPaymentOrphanNotCredit)
		require.Zero(t, userQuota(t, db, user.Id))
	} else {
		require.NoError(t, creditErr)
		require.Equal(t, 51_250_000, userQuota(t, db, user.Id))
		var stored PaymentOrphanEvent
		require.NoError(t, db.First(&stored, event.ID).Error)
		require.Equal(t, PaymentOrphanStatusPendingReview, stored.Status)
		require.Zero(t, stored.ReviewID)
	}
}

func TestPaymentOrphanDeliveryCanFillMissingBindingButCannotRebindReviewedEvidence(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "orphan-binding", 0)
	seedAdtyOrder(t, user.Id, "orphan-binding", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	event := PaymentOrphanEvent{Provider: PaymentProviderAdtyBEpusdt, EventID: "reversal:binding", EventType: "refund.completed", Payload: `{"refund":"original"}`}
	require.NoError(t, RecordPaymentOrphanEvent(&event))
	incoming := event
	incoming.ID, incoming.ReferenceID, incoming.SessionID = 0, "orphan-binding", "provider-payment"
	require.NoError(t, RecordPaymentOrphanEvent(&incoming), "a redelivery can identify a formerly unknown order")
	review, err := ResolvePaymentOrphanReview(event.ID, 1, PaymentOrphanReviewRequest{Status: PaymentOrphanStatusNeverCredited, TradeNo: incoming.ReferenceID, ExternalAmount: "50.01", ExternalCurrency: "usdt", Proof: "full-refund", Note: "confirmed no credit"})
	require.NoError(t, err)
	require.Equal(t, "USDT", review.ExternalCurrency)
	require.Equal(t, incoming.Provider, review.Provider)
	require.Equal(t, incoming.EventID, review.EventID)
	require.Len(t, review.PayloadHash, 64)
	for _, field := range []string{"provider", "event_type", "reference_id", "session_id"} {
		conflict := incoming
		conflict.ID = 0
		switch field {
		case "provider":
			conflict.Provider = PaymentProviderStripe
		case "event_type":
			conflict.EventType = "different-event"
		case "reference_id":
			conflict.ReferenceID = "another-order"
		case "session_id":
			conflict.SessionID = "another-payment"
		}
		require.ErrorIs(t, RecordPaymentOrphanEvent(&conflict), ErrAdtyIdempotencyConflict, field)
	}
	incoming.ID, incoming.ReferenceID, incoming.SessionID = 0, "", ""
	incoming.Payload, incoming.Reason = `{"refund":"different"}`, "replacement narrative"
	require.NoError(t, RecordPaymentOrphanEvent(&incoming))
	var stored PaymentOrphanEvent
	require.NoError(t, db.First(&stored, event.ID).Error)
	require.Equal(t, "orphan-binding", stored.ReferenceID)
	require.Equal(t, "provider-payment", stored.SessionID)
	require.Equal(t, event.Payload, stored.Payload)
	require.Equal(t, review.ID, stored.ReviewID)
	require.Equal(t, PaymentOrphanStatusNeverCredited, stored.Status)
}
