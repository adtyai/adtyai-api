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
	"github.com/stretchr/testify/require"
)

func TestWaffoPancakeTokenAmountIsNotTruncated(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))
	require.NoError(t, model.MigrateAdtyTables())
	user := &model.User{Username: "pancake-tokens", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 0}
	require.NoError(t, db.Create(user).Error)

	previousDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	previousUnit := common.QuotaPerUnit
	previousPrice := setting.WaffoPancakeUnitPrice
	previousMin := setting.WaffoPancakeMinTopUp
	previousMerchant := setting.WaffoPancakeMerchantID
	previousKey := setting.WaffoPancakePrivateKey
	previousProduct := setting.WaffoPancakeProductID
	previousStore := setting.WaffoPancakeStoreID
	previousRedis := common.RedisEnabled
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	common.QuotaPerUnit = 500000
	setting.WaffoPancakeUnitPrice = 1
	setting.WaffoPancakeMinTopUp = 1
	setting.WaffoPancakeMerchantID = "merchant-f081"
	setting.WaffoPancakePrivateKey = "private-f081"
	setting.WaffoPancakeProductID = "product-f081"
	setting.WaffoPancakeStoreID = ""
	common.RedisEnabled = false
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplay
		common.QuotaPerUnit = previousUnit
		setting.WaffoPancakeUnitPrice = previousPrice
		setting.WaffoPancakeMinTopUp = previousMin
		setting.WaffoPancakeMerchantID = previousMerchant
		setting.WaffoPancakePrivateKey = previousKey
		setting.WaffoPancakeProductID = previousProduct
		setting.WaffoPancakeStoreID = previousStore
		common.RedisEnabled = previousRedis
	})

	partialQuote := callPancake(t, RequestWaffoPancakeAmount, user.Id, `{"amount":750000}`)
	require.Contains(t, partialQuote.Body.String(), "Waffo Pancake configuration is incomplete")
	partialPay := callPancake(t, RequestWaffoPancakePay, user.Id, `{"amount":750000}`)
	require.Contains(t, partialPay.Body.String(), "Waffo Pancake configuration is incomplete")
	var orders int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&orders).Error)
	require.Zero(t, orders)
	quoted := callPancake(t, RequestWaffoPancakeAmount, user.Id, `{"amount":1000000}`)
	require.Contains(t, quoted.Body.String(), "Waffo Pancake configuration is incomplete")
	paid := callPancake(t, RequestWaffoPancakePay, user.Id, `{"amount":1000000}`)
	require.Contains(t, paid.Body.String(), "Waffo Pancake configuration is incomplete")
	require.NoError(t, db.Model(&model.TopUp{}).Count(&orders).Error)
	require.Zero(t, orders)
}

func callPancake(t *testing.T, handler gin.HandlerFunc, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/waffo-pancake/pay", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	handler(ctx)
	return recorder
}
