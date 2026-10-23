package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEpaySnapshotFailureMarksOrderFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevDB, prevLog := model.DB, model.LOG_DB
	prevMain, prevLogType := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopupOrder{}, &model.EpayInstance{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLog
		common.SetDatabaseTypes(prevMain, prevLogType)
		startEpayPurchaseForOrder = StartEpayPurchaseForOrder
	})

	user := &model.User{
		Username: "payer",
		Password: "placeholder",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "payer-aff",
	}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&model.EpayInstance{
		Name:              "primary",
		PartnerId:         "1",
		SecretCiphertext:  "cipher",
		SecretFingerprint: "abcd",
		PayAddress:        "https://pay.example",
		Status:            model.EpayInstanceStatusEnabled,
		IsPrimary:         true,
	}).Error)

	startEpayPurchaseForOrder = func(order *model.TopupOrder, payMethod string) (string, map[string]string, *model.EpayInstance, error) {
		return "", map[string]string{"out_trade_no": order.OrderNo}, &model.EpayInstance{Id: 1}, nil
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/topup/order", strings.NewReader(`{"amount":10,"payment_method":"alipay"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", user.Id)
	CreateTopupOrder(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), "epay_snapshot_failed")
	require.Contains(t, w.Body.String(), `"status":"failed"`)

	var stored model.TopupOrder
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&stored).Error)
	require.Equal(t, model.TopupOrderStatusFailed, stored.Status)
	require.Empty(t, stored.EpayPayURL)
}

func TestEpaySnapshotSaveFailureMarksOrderFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevDB, prevLog := model.DB, model.LOG_DB
	prevMain, prevLogType := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopupOrder{}, &model.EpayInstance{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLog
		common.SetDatabaseTypes(prevMain, prevLogType)
		startEpayPurchaseForOrder = StartEpayPurchaseForOrder
	})

	user := &model.User{
		Username: "payer-save",
		Password: "placeholder",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "payer-save-aff",
	}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&model.EpayInstance{
		Name:              "primary",
		PartnerId:         "1",
		SecretCiphertext:  "cipher",
		SecretFingerprint: "abcd",
		PayAddress:        "https://pay.example",
		Status:            model.EpayInstanceStatusEnabled,
		IsPrimary:         true,
	}).Error)

	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("epay-snapshot-fail", func(tx *gorm.DB) {
		dest, ok := tx.Statement.Dest.(map[string]any)
		if !ok {
			return
		}
		if _, has := dest["epay_pay_url"]; has {
			_ = tx.AddError(errors.New("snapshot write failed"))
		}
	}))

	startEpayPurchaseForOrder = func(order *model.TopupOrder, payMethod string) (string, map[string]string, *model.EpayInstance, error) {
		return "https://pay.example/cashier", map[string]string{"out_trade_no": order.OrderNo}, &model.EpayInstance{Id: 1}, nil
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/topup/order", strings.NewReader(`{"amount":10,"payment_method":"wxpay"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", user.Id)
	CreateTopupOrder(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), "epay_snapshot_failed")
	require.NotContains(t, w.Body.String(), "pay.example/cashier")

	var stored model.TopupOrder
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&stored).Error)
	require.Equal(t, model.TopupOrderStatusFailed, stored.Status)
	require.Empty(t, stored.EpayPayURL)
	require.Zero(t, stored.EpayInstanceId)
}
