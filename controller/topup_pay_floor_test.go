package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 预览和下单共用同一个金额下限判据（ISS-R4-0128）：payMoney 恰好 0.01 时预览必须和下单一样放行，
// 而不是一边报「Payment amount is too low」、一边能建单。
func TestRequestAmountAcceptsMinimumGatewayAmount(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldPrice := operation_setting.Price
	oldMinTopUp := operation_setting.MinTopUp
	oldDB := model.DB
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.Price = 0.01 // getPayMoney(1) == 1 * 0.01 * ratio(1) * discount(1) == 0.01
	operation_setting.MinTopUp = 1

	model.InitColumnNamesForTests() // GetUserGroup 用带引号的 group 列名，只有初始化过才不为空

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		operation_setting.Price = oldPrice
		operation_setting.MinTopUp = oldMinTopUp
		model.DB = oldDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	require.NoError(t, model.DB.Create(&model.User{
		Id:       42,
		Username: "topup_floor_user",
		Quota:    0,
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 42)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/amount", strings.NewReader(`{"amount":1}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAmount(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"message":"success","data":"0.01"}`, recorder.Body.String())
}

// 金额下限只有一个定义：0.01 本身可收，低于 0.01 不可收。
func TestPayMoneyFloorIsDefinedOnce(t *testing.T) {
	assert.False(t, payMoneyTooLow(0.01), "0.01 is a valid payment amount")
	assert.True(t, payMoneyTooLow(0.009999), "anything below 0.01 is rejected")
}
