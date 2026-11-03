package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/waffo-com/waffo-go/core"
	"gorm.io/gorm"
)

func TestWaffoIntermediateStatusLeavesOrderPending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMain, previousLog)
	})
	order := &model.TopUp{
		UserId:          1,
		Amount:          2,
		Money:           2,
		TradeNo:         "waffo-open",
		PaymentMethod:   model.PaymentMethodWaffo,
		PaymentProvider: model.PaymentProviderWaffo,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(order).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/waffo/webhook", nil)

	for _, status := range []string{core.OrderStatusPayInProgress, core.OrderStatusAuthorizationRequired} {
		applyWaffoNonSuccessStatus(ctx, order.TradeNo, status)
		var stored model.TopUp
		require.NoError(t, db.First(&stored, order.Id).Error)
		require.Equal(t, common.TopUpStatusPending, stored.Status)
	}

	applyWaffoNonSuccessStatus(ctx, order.TradeNo, core.OrderStatusOrderClose)
	var closed model.TopUp
	require.NoError(t, db.First(&closed, order.Id).Error)
	require.Equal(t, common.TopUpStatusFailed, closed.Status)
}
