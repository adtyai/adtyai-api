package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestReplayStoredEpayCheckoutDoesNotNeedGateway(t *testing.T) {
	url, params, err := replayStoredEpayCheckout(&model.TopupOrder{
		PaymentMethod:  "alipay",
		EpayPayURL:     "https://pay.example/cashier",
		EpayPayParams:  `{"pid":"1","out_trade_no":"ADTY1"}`,
		PayCNY:         72,
		EpayInstanceId: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://pay.example/cashier" {
		t.Fatalf("url = %s", url)
	}
	if params["out_trade_no"] != "ADTY1" {
		t.Fatalf("params = %#v", params)
	}

	if _, _, err := replayStoredEpayCheckout(&model.TopupOrder{PaymentMethod: "wxpay", PayCNY: 72, EpayInstanceId: 3}); err == nil {
		t.Fatal("missing stored url must not open another gateway order")
	}
}

func TestUserTopupOrderViewOmitsDeviceAndIP(t *testing.T) {
	view := userTopupOrderView(&model.TopupOrder{
		OrderNo: "X", UserId: 7, DeviceFp: "fp", ClientIP: "1.2.3.4",
		PaidAmountUSD: 10, RefundedAmount: 1,
	}, 1)
	if _, ok := view["device_fp"]; ok {
		t.Fatal("device_fp leaked")
	}
	if _, ok := view["client_ip"]; ok {
		t.Fatal("client_ip leaked")
	}
	if view["user_id"] != 7 || view["paid_amount_usd"] != 10.0 || view["refunded_amount"] != 1.0 {
		t.Fatalf("refund fields missing: %#v", view)
	}
}

func TestUserTopupOrderViewHidesCashierWhenNotPayable(t *testing.T) {
	const addr = "TOLDADDRESS123"
	const page = "https://pay.example/old"
	open := userTopupOrderView(&model.TopupOrder{
		PaymentMethod:     "usdt",
		Status:            model.TopupOrderStatusPending,
		UsdtExpireAt:      2_000,
		UsdtPayAddress:    addr,
		UsdtPaymentURL:    page,
		UsdtPaymentAmount: "10.01",
	}, 1_000)
	if open["usdt_pay_address"] != addr || open["usdt_payment_url"] != page {
		t.Fatalf("open window must keep cashier: %#v", open)
	}

	closed := []model.TopupOrder{
		{PaymentMethod: "usdt", Status: model.TopupOrderStatusPending, UsdtExpireAt: 500, UsdtPayAddress: addr, UsdtPaymentURL: page, UsdtPaymentAmount: "10.01"},
		{PaymentMethod: "usdt", Status: model.TopupOrderStatusPaid, UsdtExpireAt: 2_000, UsdtPayAddress: addr, UsdtPaymentURL: page, UsdtPaymentAmount: "10.01"},
		{PaymentMethod: "usdt", Status: model.TopupOrderStatusFailed, UsdtExpireAt: 2_000, UsdtPayAddress: addr, UsdtPaymentURL: page},
		{PaymentMethod: "usdt", Status: model.TopupOrderStatusNeedManual, UsdtExpireAt: 2_000, UsdtPayAddress: addr, UsdtPaymentURL: page},
	}
	for i, order := range closed {
		view := userTopupOrderView(&order, 1_000)
		if view["usdt_pay_address"] == addr || view["usdt_payment_url"] == page {
			t.Fatalf("case %d leaked cashier: %#v", i, view)
		}
	}
}
