package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestWaffoTokenAmountIsNotTruncated(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))
	user := &model.User{Username: "waffo-tokens", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 0, Email: "waffo-tokens@example.com"}
	require.NoError(t, db.Create(user).Error)

	previousDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	previousUnit := common.QuotaPerUnit
	previousPrice := setting.WaffoUnitPrice
	previousEnabled := setting.WaffoEnabled
	previousMin := setting.WaffoMinTopUp
	previousRedis := common.RedisEnabled
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	common.QuotaPerUnit = 500000
	setting.WaffoUnitPrice = 1
	setting.WaffoEnabled = true
	setting.WaffoMinTopUp = 1
	common.RedisEnabled = false
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplay
		common.QuotaPerUnit = previousUnit
		setting.WaffoUnitPrice = previousPrice
		setting.WaffoEnabled = previousEnabled
		setting.WaffoMinTopUp = previousMin
		common.RedisEnabled = previousRedis
	})

	partialQuote := callWaffo(t, RequestWaffoAmount, user.Id, `{"amount":750000}`)
	require.Contains(t, partialQuote.Body.String(), "whole number of quota units")
	partialPay := callWaffo(t, RequestWaffoPay, user.Id, `{"amount":750000}`)
	require.Contains(t, partialPay.Body.String(), "whole number of quota units")
	var orders int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&orders).Error)
	require.Zero(t, orders)

	quoted := callWaffo(t, RequestWaffoAmount, user.Id, `{"amount":1000000}`)
	require.Contains(t, quoted.Body.String(), `"data":"2.00"`)
	paid := callWaffo(t, RequestWaffoPay, user.Id, `{"amount":1000000}`)
	require.NotContains(t, paid.Body.String(), "whole number of quota units")
	var order model.TopUp
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&order).Error)
	require.Equal(t, int64(2), order.Amount)
	require.True(t, decimal.NewFromFloat(order.Money).Equal(decimal.NewFromInt(2)))
}

func callWaffo(t *testing.T, handler gin.HandlerFunc, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/waffo/pay", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	handler(ctx)
	return recorder
}
