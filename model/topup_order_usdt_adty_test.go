package model

import "testing"

func TestTopupOrderDisplayStatus(t *testing.T) {
	pending := &TopupOrder{Status: TopupOrderStatusPending, PaymentMethod: "usdt", UsdtExpireAt: 100, UsdtPayAddress: "Taddr", UsdtPaymentAmount: "10.01"}
	if got := TopupOrderDisplayStatus(pending, 99); got != TopupDisplayPending {
		t.Fatalf("before expiry: %s", got)
	}
	if !TopupOrderCanContinue(pending, 99) {
		t.Fatal("pending usdt with address should continue")
	}
	if got := TopupOrderDisplayStatus(pending, 100); got != TopupDisplayExpired {
		t.Fatalf("at expiry: %s", got)
	}
	if TopupOrderCanContinue(pending, 100) {
		t.Fatal("expired usdt must not continue")
	}

	review := &TopupOrder{Status: TopupOrderStatusNeedManual, PaymentMethod: "usdt", UsdtPayAddress: "Taddr", UsdtPaymentAmount: "1"}
	if got := TopupOrderDisplayStatus(review, 1); got != TopupDisplayReview {
		t.Fatalf("review: %s", got)
	}
	if TopupOrderCanContinue(review, 1) {
		t.Fatal("review must not continue")
	}

	epay := &TopupOrder{Status: TopupOrderStatusPending, PaymentMethod: "alipay"}
	if TopupOrderCanContinue(epay, 1) {
		t.Fatal("alipay without a stored cashier must not continue")
	}
	stored := &TopupOrder{Status: TopupOrderStatusPending, PaymentMethod: "wxpay", EpayPayURL: "https://pay.example/c", PayCNY: 72, EpayInstanceId: 3}
	if !TopupOrderCanContinue(stored, 1) {
		t.Fatal("epay with stored cashier should continue")
	}
	noAddr := &TopupOrder{Status: TopupOrderStatusPending, PaymentMethod: "usdt", UsdtExpireAt: 0}
	if TopupOrderCanContinue(noAddr, 1) {
		t.Fatal("usdt without address must not continue")
	}
}
