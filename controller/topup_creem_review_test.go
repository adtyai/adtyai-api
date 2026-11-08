package controller

import (
	"bytes"
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
)

func TestCreemQuotaLimitEntersManualReview(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.PaymentOrphanEvent{}))
	require.NoError(t, model.MigrateAdtyTables())

	previousKey := setting.CreemApiKey
	previousProducts := setting.CreemProducts
	previousSecret := setting.CreemWebhookSecret
	setting.CreemApiKey = "creem_test_f071"
	setting.CreemProducts = `[{"productId":"prod_f071","name":"pack","price":1,"currency":"USD","quota":1000}]`
	setting.CreemWebhookSecret = "creem_whsec_f071"
	t.Cleanup(func() {
		setting.CreemApiKey = previousKey
		setting.CreemProducts = previousProducts
		setting.CreemWebhookSecret = previousSecret
	})

	payer := &model.User{
		Username: "creem-ok",
		Password: "x",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Email:    "creem-ok@example.com",
		Group:    "default",
		Quota:    0,
	}
	require.NoError(t, db.Create(payer).Error)
	paid := &model.TopUp{
		UserId:          payer.Id,
		Amount:          1000,
		Money:           1,
		TradeNo:         "creem-ok-trade",
		PaymentMethod:   model.PaymentMethodCreem,
		PaymentProvider: model.PaymentProviderCreem,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(paid).Error)

	first := postCreemPaidWebhook(t, paid.TradeNo, "ord_ok")
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, 1000, reloadUserQuota(t, db, payer.Id))
	require.NoError(t, db.First(paid, paid.Id).Error)
	require.Equal(t, common.TopUpStatusSuccess, paid.Status)

	again := postCreemPaidWebhook(t, paid.TradeNo, "ord_ok")
	require.Equal(t, http.StatusOK, again.Code)
	require.Equal(t, 1000, reloadUserQuota(t, db, payer.Id))
	require.NoError(t, model.RechargeCreem(paid.TradeNo, "buyer@example.com", "Buyer", "127.0.0.1"))
	require.Equal(t, 1000, reloadUserQuota(t, db, payer.Id))

	limited := &model.User{
		Username: "creem-limit",
		Password: "x",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Email:    "creem-limit@example.com",
		Group:    "default",
		Quota:    100,
	}
	require.NoError(t, db.Create(limited).Error)
	blocked := &model.TopUp{
		UserId:          limited.Id,
		Amount:          int64(common.MaxWalletQuota) - 50,
		Money:           1,
		TradeNo:         "creem-limit-trade",
		PaymentMethod:   model.PaymentMethodCreem,
		PaymentProvider: model.PaymentProviderCreem,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(blocked).Error)

	rejected := postCreemPaidWebhook(t, blocked.TradeNo, "ord_limit")
	require.Equal(t, http.StatusOK, rejected.Code)
	require.Equal(t, 100, reloadUserQuota(t, db, limited.Id))
	require.NoError(t, db.First(blocked, blocked.Id).Error)
	require.Equal(t, common.TopUpStatusPending, blocked.Status)

	var orphan model.PaymentOrphanEvent
	require.NoError(t, db.Where("reference_id = ?", blocked.TradeNo).First(&orphan).Error)
	require.Equal(t, model.PaymentProviderCreem, orphan.Provider)
	require.Equal(t, model.PaymentOrphanStatusPendingReview, orphan.Status)
	require.Contains(t, orphan.Error, "Top-up quota limit exceeded")
	var extra int64
	require.NoError(t, db.Model(&model.PaymentOrphanEvent{}).Where("reference_id = ?", paid.TradeNo).Count(&extra).Error)
	require.Equal(t, int64(0), extra)
}

func TestCreemCallbackDoesNotClaimTakenEmail(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))
	require.NoError(t, model.MigrateAdtyTables())
	previousKey := setting.CreemApiKey
	previousProducts := setting.CreemProducts
	previousSecret := setting.CreemWebhookSecret
	setting.CreemApiKey = "creem_test_f072"
	setting.CreemProducts = `[{"productId":"prod_f072","name":"pack","price":1,"currency":"USD","quota":50}]`
	setting.CreemWebhookSecret = "creem_whsec_f072"
	t.Cleanup(func() {
		setting.CreemApiKey = previousKey
		setting.CreemProducts = previousProducts
		setting.CreemWebhookSecret = previousSecret
	})

	holder := &model.User{Username: "creem-holder", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Email: "taken@example.com", Group: "default"}
	require.NoError(t, db.Create(holder).Error)
	empty := &model.User{Username: "creem-empty", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Email: "", Group: "default", Quota: 0}
	require.NoError(t, db.Create(empty).Error)
	order := &model.TopUp{UserId: empty.Id, Amount: 50, Money: 1, TradeNo: "creem-email-taken", PaymentMethod: model.PaymentMethodCreem, PaymentProvider: model.PaymentProviderCreem, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(order).Error)

	taken := postCreemPaidWebhookForEmail(t, order.TradeNo, "ord_taken", "Taken@Example.COM")
	require.Equal(t, http.StatusOK, taken.Code)
	require.Equal(t, 50, reloadUserQuota(t, db, empty.Id))
	var stored model.User
	require.NoError(t, db.First(&stored, empty.Id).Error)
	require.Equal(t, "", stored.Email)

	freshUser := &model.User{Username: "creem-fresh", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Email: "", Group: "default", Quota: 0}
	require.NoError(t, db.Create(freshUser).Error)
	freshOrder := &model.TopUp{UserId: freshUser.Id, Amount: 50, Money: 1, TradeNo: "creem-email-fresh", PaymentMethod: model.PaymentMethodCreem, PaymentProvider: model.PaymentProviderCreem, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(freshOrder).Error)
	fresh := postCreemPaidWebhookForEmail(t, freshOrder.TradeNo, "ord_fresh", "Fresh-Creem@Example.com")
	require.Equal(t, http.StatusOK, fresh.Code)
	require.Equal(t, 50, reloadUserQuota(t, db, freshUser.Id))
	var freshStored model.User
	require.NoError(t, db.First(&freshStored, freshUser.Id).Error)
	require.Equal(t, "fresh-creem@example.com", freshStored.Email)
}

func postCreemPaidWebhook(t *testing.T, tradeNo string, orderID string) *httptest.ResponseRecorder {
	t.Helper()
	return postCreemPaidWebhookForEmail(t, tradeNo, orderID, "buyer@example.com")
}

func postCreemPaidWebhookForEmail(t *testing.T, tradeNo string, orderID string, email string) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(`{"id":"evt_` + tradeNo + `","eventType":"checkout.completed","object":{"request_id":"` + tradeNo + `","order":{"id":"` + orderID + `","status":"paid","type":"onetime","amount_paid":100},"product":{"name":"pack"},"customer":{"email":"` + email + `","name":"Buyer"}}}`)
	signature := generateCreemSignature(string(body), setting.CreemWebhookSecret)
	req := httptest.NewRequest(http.MethodPost, "/api/creem/webhook", io.NopCloser(bytes.NewReader(body)))
	req.Header.Set(CreemSignatureHeader, signature)
	recorder := httptest.NewRecorder()
	engine := gin.New()
	engine.POST("/api/creem/webhook", CreemWebhook)
	engine.ServeHTTP(recorder, req)
	return recorder
}
