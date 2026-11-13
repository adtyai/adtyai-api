package controller

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	waffoutils "github.com/waffo-com/waffo-go/utils"
	"gorm.io/gorm"
)

func TestWaffoRefundNotificationClawsBackQuota(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	previousRedis := common.RedisEnabled
	previousRDB := common.RDB
	server := miniredis.RunT(t)
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = previousRDB
		common.RedisEnabled = previousRedis
	})

	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.AdtyBalanceLedger{}))
	require.NoError(t, model.MigrateAdtyTables())
	previousUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousUnit })

	privateKey, publicKey := waffoTestKeyPair(t)
	previousEnabled := setting.WaffoEnabled
	previousSandbox := setting.WaffoSandbox
	previousAPI := setting.WaffoSandboxApiKey
	previousPrivate := setting.WaffoSandboxPrivateKey
	previousPublic := setting.WaffoSandboxPublicCert
	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoSandboxApiKey = "waffo_test_f075"
	setting.WaffoSandboxPrivateKey = privateKey
	setting.WaffoSandboxPublicCert = publicKey
	t.Cleanup(func() {
		setting.WaffoEnabled = previousEnabled
		setting.WaffoSandbox = previousSandbox
		setting.WaffoSandboxApiKey = previousAPI
		setting.WaffoSandboxPrivateKey = previousPrivate
		setting.WaffoSandboxPublicCert = previousPublic
	})

	fullUser := seedWaffoRefundUser(t, db, "waffo-full", 1_000_000)
	fullOrder := seedWaffoRefundOrder(t, db, fullUser.Id, "WAFFO-FULL", 2, 2)
	inProgress := postWaffoRefund(t, privateKey, fullOrder.TradeNo, "refund-progress", "REFUND_IN_PROGRESS", "2.00")
	require.Equal(t, http.StatusForbidden, inProgress.Code)
	require.Equal(t, 1_000_000, reloadUserQuota(t, db, fullUser.Id))
	failed := postWaffoRefund(t, privateKey, fullOrder.TradeNo, "refund-failed", "ORDER_REFUND_FAILED", "2.00")
	require.Equal(t, http.StatusForbidden, failed.Code)
	require.Equal(t, 1_000_000, reloadUserQuota(t, db, fullUser.Id))
}

func seedWaffoRefundUser(t *testing.T, db *gorm.DB, username string, quota int) *model.User {
	t.Helper()
	user := &model.User{
		Username: username,
		Password: "x",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    quota,
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

func seedWaffoRefundOrder(t *testing.T, db *gorm.DB, userID int, tradeNo string, amount int64, money float64) *model.TopUp {
	t.Helper()
	order := &model.TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWaffo,
		PaymentProvider: model.PaymentProviderWaffo,
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, db.Create(order).Error)
	return order
}

func waffoOrderStatus(t *testing.T, db *gorm.DB, tradeNo string) string {
	t.Helper()
	var order model.TopUp
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&order).Error)
	return order.Status
}

func postWaffoRefund(t *testing.T, privateKey string, tradeNo string, refundID string, status string, amount string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"eventType": "REFUND_NOTIFICATION",
		"result": map[string]any{
			"refundRequestId":      refundID,
			"origPaymentRequestId": tradeNo,
			"refundStatus":         status,
			"refundAmount":         amount,
		},
	})
	require.NoError(t, err)
	signature, err := waffoutils.Sign(string(body), privateKey)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", io.NopCloser(bytes.NewReader(body)))
	req.Header.Set("X-SIGNATURE", signature)
	recorder := httptest.NewRecorder()
	engine := gin.New()
	engine.POST("/api/waffo/webhook", WaffoWebhook)
	engine.ServeHTTP(recorder, req)
	return recorder
}

func waffoTestKeyPair(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privateBytes, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	publicBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(privateBytes), base64.StdEncoding.EncodeToString(publicBytes)
}
