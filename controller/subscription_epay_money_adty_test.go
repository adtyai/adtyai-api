package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSubscriptionEpayChargeAmountConvertsUSD(t *testing.T) {
	sent, payload, err := subscriptionEpayChargeAmount(10, "USD", 7.2)
	require.NoError(t, err)
	require.Equal(t, "72.00", sent)
	expected, ok := expectedSubscriptionEpayMoney(payload)
	require.True(t, ok)
	require.Equal(t, 72.0, expected)

	sent, _, err = subscriptionEpayChargeAmount(10, "CNY", 7.2)
	require.NoError(t, err)
	require.Equal(t, "10.00", sent)

	_, _, err = subscriptionEpayChargeAmount(10, "EUR", 7.2)
	require.Error(t, err)
	_, _, err = subscriptionEpayChargeAmount(10, "USD", 0)
	require.Error(t, err)
}

func TestSubscriptionEpayNotifyRejectsUnderpay(t *testing.T) {
	prevAddr, prevID, prevKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "2002"
	operation_setting.EpayKey = "sk-sub-epay"
	t.Cleanup(func() {
		operation_setting.PayAddress = prevAddr
		operation_setting.EpayId = prevID
		operation_setting.EpayKey = prevKey
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionOrder{}))
	prev := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = prev })

	_, snapshot, err := subscriptionEpayChargeAmount(10, "USD", 7.2)
	require.NoError(t, err)
	order := &model.SubscriptionOrder{
		UserId:          1,
		PlanId:          1,
		Money:           10,
		TradeNo:         "SUB-UNDERPAY",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		ProviderPayload: snapshot,
	}
	require.NoError(t, order.Insert())

	body := callSubscriptionEpayNotify(t, map[string]string{
		"pid": "2002", "trade_no": "GW-1", "out_trade_no": "SUB-UNDERPAY",
		"type": "alipay", "name": "sub", "money": "10.00", "trade_status": "TRADE_SUCCESS",
	})
	require.Equal(t, "fail", body)

	var got model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", "SUB-UNDERPAY").First(&got).Error)
	require.Equal(t, common.TopUpStatusPending, got.Status)
}

func callSubscriptionEpayNotify(t *testing.T, p map[string]string) string {
	t.Helper()
	p["sign"] = epayTestSign(p, "sk-sub-epay")
	p["sign_type"] = "MD5"
	q := url.Values{}
	for k, v := range p {
		q.Set(k, v)
	}
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/epay/notify?"+q.Encode(), nil)
	SubscriptionEpayNotify(c)
	return rec.Body.String()
}
