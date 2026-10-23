package model

import (
	"testing"
	"time"
)

func TestUsdtExpireAtFromBepusdtRemainingSeconds(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	got := UsdtExpireAtFromBepusdtRemaining(900, now)
	if got != now.Add(900*time.Second).Unix() {
		t.Fatalf("remaining seconds must not be stored as a unix timestamp, got %d", got)
	}
	// A value that already looks like a unix timestamp is not remaining seconds.
	if UsdtExpireAtFromBepusdtRemaining(now.Unix(), now) <= now.Unix() {
		t.Fatal("timestamp-shaped expiration_time must fall back to a future window")
	}
	if UsdtExpireAtFromBepusdtRemaining(0, now) <= now.Unix() {
		t.Fatal("missing expiration_time must fall back to a future window")
	}
}

func TestDecideUsdtPaidCallbackCreditsSnapshotDespiteLocalExpiryAndShortPay(t *testing.T) {
	order := &TopupOrder{
		Status:       TopupOrderStatusPending,
		PayUSDT:      10,
		UsdtExpireAt: 1_700_000_000,
	}
	expired := DecideUsdtPaidCallback(order, 10)
	if expired.Action != UsdtPaidCallbackCredit {
		t.Fatalf("status 2 after local expiry must credit, got %+v", expired)
	}
	short := DecideUsdtPaidCallback(order, 1)
	if short.Action != UsdtPaidCallbackCredit {
		t.Fatalf("status 2 short pay must credit the snapshot, got %+v", short)
	}
	if err := EvaluateUsdtCallbackCredit(order, 1, order.UsdtExpireAt); err != nil {
		t.Fatalf("local expiry and short pay must not block credit, got %v", err)
	}
}

func TestDecideUsdtPaidCallbackDoesNotCreditLedgerConflicts(t *testing.T) {
	paid := DecideUsdtPaidCallback(&TopupOrder{Status: TopupOrderStatusPaid, PayUSDT: 10}, 10)
	if paid.Action != UsdtPaidCallbackIdempotent {
		t.Fatalf("already paid must be idempotent, got %+v", paid)
	}
	manual := DecideUsdtPaidCallback(&TopupOrder{Status: TopupOrderStatusNeedManual, PayUSDT: 10}, 10)
	if manual.Action != UsdtPaidCallbackManual {
		t.Fatalf("existing manual order must not auto-credit, got %+v", manual)
	}
	failed := DecideUsdtPaidCallback(&TopupOrder{Status: TopupOrderStatusFailed, PayUSDT: 10}, 10)
	if failed.Action != UsdtPaidCallbackManual || failed.Reason != "paid_after_failed" {
		t.Fatalf("paid after failed is a ledger conflict, got %+v", failed)
	}
	missing := DecideUsdtPaidCallback(&TopupOrder{Status: TopupOrderStatusPending, PayUSDT: 10}, -1)
	if missing.Action != UsdtPaidCallbackRetry {
		t.Fatalf("missing actual_amount must retry, got %+v", missing)
	}
}

func TestTopupOrderNextAction(t *testing.T) {
	now := int64(1_000)
	pending := &TopupOrder{
		Status: TopupOrderStatusPending, PaymentMethod: "usdt",
		UsdtExpireAt: now + 60, UsdtPayAddress: "Taddr", UsdtPaymentAmount: "10.01",
	}
	if action, _ := TopupOrderNextAction(pending, now); action != TopupNextContinue {
		t.Fatalf("inside window: %s", action)
	}
	if action, hint := TopupOrderNextAction(pending, pending.UsdtExpireAt); action != TopupNextCreateNew || hint != "window_ended" {
		t.Fatalf("window ended: %s %s", action, hint)
	}
	review := &TopupOrder{Status: TopupOrderStatusNeedManual, PaymentMethod: "usdt"}
	if action, hint := TopupOrderNextAction(review, now); action != TopupNextContact || hint != "exception_review" {
		t.Fatalf("review: %s %s", action, hint)
	}
	paid := &TopupOrder{Status: TopupOrderStatusPaid}
	if action, _ := TopupOrderNextAction(paid, now); action != TopupNextViewCredit {
		t.Fatalf("paid: %s", action)
	}
}
