package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUsdtNotifyDoesNotAckWhenManualMarkFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevDB, prevLog := model.DB, model.LOG_DB
	prevMain, prevLogType := common.MainDatabaseType(), common.LogDatabaseType()
	prevToken := setting.GetBepusdtApiToken()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	pinMemorySQLite(t, db)
	require.NoError(t, db.AutoMigrate(&model.TopupOrder{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	setting.SetBepusdtApiToken("usdt-mark-fail-token")
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLog
		common.SetDatabaseTypes(prevMain, prevLogType)
		setting.SetBepusdtApiToken(prevToken)
	})

	require.NoError(t, db.Create(&model.TopupOrder{
		OrderNo:       "USDT-MARK-FAIL",
		UserId:        1,
		AmountUSD:     10,
		PaymentMethod: "usdt",
		Status:        model.TopupOrderStatusFailed,
		PayUSDT:       10,
		UsdtExpireAt:  time.Now().Add(-time.Hour).Unix(),
		RefundStatus:  model.RefundStatusNone,
	}).Error)
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("usdt-mark-fail", func(tx *gorm.DB) {
		_ = tx.AddError(errUsdtMarkFail)
	}))

	params := map[string]interface{}{
		"order_id":      "USDT-MARK-FAIL",
		"status":        "paid",
		"actual_amount": "10",
	}
	params["signature"] = service.AdtyBepusdtSign(params, "usdt-mark-fail-token")
	body := `{"order_id":"USDT-MARK-FAIL","status":"paid","actual_amount":"10","signature":"` + params["signature"].(string) + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pay/usdt/notify", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AdtyUsdtNotify(c)

	var stored model.TopupOrder
	require.NoError(t, db.Where("order_no = ?", "USDT-MARK-FAIL").First(&stored).Error)
	require.Equal(t, model.TopupOrderStatusFailed, stored.Status)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Equal(t, "fail", w.Body.String())
}

func TestUsdtNotifyAcksAfterManualMarkStored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevDB, prevLog := model.DB, model.LOG_DB
	prevMain, prevLogType := common.MainDatabaseType(), common.LogDatabaseType()
	prevToken := setting.GetBepusdtApiToken()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	pinMemorySQLite(t, db)
	require.NoError(t, db.AutoMigrate(&model.TopupOrder{}, &model.User{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	setting.SetBepusdtApiToken("usdt-mark-ok-token")
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLog
		common.SetDatabaseTypes(prevMain, prevLogType)
		setting.SetBepusdtApiToken(prevToken)
	})

	user := &model.User{Username: "payer", Quota: 1000, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&model.TopupOrder{
		OrderNo:       "USDT-MARK-OK",
		UserId:        user.Id,
		AmountUSD:     10,
		PaymentMethod: "usdt",
		Status:        model.TopupOrderStatusFailed,
		PayUSDT:       10,
		UsdtExpireAt:  time.Now().Add(-time.Hour).Unix(),
		RefundStatus:  model.RefundStatusNone,
	}).Error)

	params := map[string]interface{}{
		"order_id":      "USDT-MARK-OK",
		"status":        "paid",
		"actual_amount": "10",
	}
	params["signature"] = service.AdtyBepusdtSign(params, "usdt-mark-ok-token")
	body := `{"order_id":"USDT-MARK-OK","status":"paid","actual_amount":"10","signature":"` + params["signature"].(string) + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pay/usdt/notify", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AdtyUsdtNotify(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ok", w.Body.String())
	var stored model.TopupOrder
	require.NoError(t, db.Where("order_no = ?", "USDT-MARK-OK").First(&stored).Error)
	require.Equal(t, model.TopupOrderStatusNeedManual, stored.Status)
	require.Equal(t, "paid_after_failed", stored.PendingReason)
	var paid model.User
	require.NoError(t, db.First(&paid, user.Id).Error)
	require.Equal(t, 1000, paid.Quota)
}

func TestUsdtNotifyCreditsSnapshotWhenShortOrLocallyExpired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevDB, prevLog := model.DB, model.LOG_DB
	prevMain, prevLogType := common.MainDatabaseType(), common.LogDatabaseType()
	prevToken := setting.GetBepusdtApiToken()
	prevRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	pinMemorySQLite(t, db)
	require.NoError(t, db.AutoMigrate(&model.TopupOrder{}, &model.User{}, &model.UserBonusWallet{}))
	require.NoError(t, model.EnsureLedgerSchema(db))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	setting.SetBepusdtApiToken("usdt-credit-token")
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLog
		common.SetDatabaseTypes(prevMain, prevLogType)
		common.RedisEnabled = prevRedis
		setting.SetBepusdtApiToken(prevToken)
	})

	user := &model.User{Username: "short-payer", Quota: 0, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&model.TopupOrder{
		OrderNo:         "USDT-SHORT-CREDIT",
		UserId:          user.Id,
		AmountUSD:       10,
		PaidAmountUSD:   10,
		CreditAmountUSD: 10,
		PaymentMethod:   "usdt",
		Status:          model.TopupOrderStatusPending,
		PayUSDT:         10,
		UsdtExpireAt:    time.Now().Add(-time.Minute).Unix(),
		RefundStatus:    model.RefundStatusNone,
	}).Error)

	params := map[string]interface{}{
		"order_id":      "USDT-SHORT-CREDIT",
		"status":        "2",
		"actual_amount": "1",
		"trade_id":      "SHORT-1",
	}
	params["signature"] = service.AdtyBepusdtSign(params, "usdt-credit-token")
	body := `{"order_id":"USDT-SHORT-CREDIT","status":"2","actual_amount":"1","trade_id":"SHORT-1","signature":"` + params["signature"].(string) + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pay/usdt/notify", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AdtyUsdtNotify(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored model.TopupOrder
	require.NoError(t, db.Where("order_no = ?", "USDT-SHORT-CREDIT").First(&stored).Error)
	require.Equal(t, model.TopupOrderStatusPaid, stored.Status)
	require.NotEqual(t, "timeout", stored.PendingReason)
	var paid model.User
	require.NoError(t, db.First(&paid, user.Id).Error)
	want, err := model.DollarsToQuota(10)
	require.NoError(t, err)
	require.Equal(t, int(want), paid.Quota)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/api/pay/usdt/notify", strings.NewReader(body))
	c2.Request.Header.Set("Content-Type", "application/json")
	AdtyUsdtNotify(c2)
	require.Equal(t, http.StatusOK, w2.Code)
	require.NoError(t, db.First(&paid, user.Id).Error)
	require.Equal(t, int(want), paid.Quota)
}

func pinMemorySQLite(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
}

var errUsdtMarkFail = errString("need_manual write failed")

type errString string

func (e errString) Error() string { return string(e) }
