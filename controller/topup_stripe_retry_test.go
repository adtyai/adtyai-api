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
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81/webhook"
	"gorm.io/gorm"
)

func TestStripeWebhookRetriesWhenCreditFails(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.User{}, &model.Log{}))
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	originalSecret := setting.StripeWebhookSecret
	originalAPI := setting.StripeApiSecret
	originalPrice := setting.StripePriceId
	logEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMain, previousLog)
		setting.StripeWebhookSecret = originalSecret
		setting.StripeApiSecret = originalAPI
		setting.StripePriceId = originalPrice
		common.LogConsumeEnabled = logEnabled
	})
	setting.StripeWebhookSecret = "whsec_retry"
	setting.StripeApiSecret = "sk_test_retry"
	setting.StripePriceId = "price_retry"

	payload := []byte(`{"id":"evt_retry","object":"event","type":"checkout.session.completed","data":{"object":{"id":"cs_retry","object":"checkout.session","client_reference_id":"STRIPE-MISSING","status":"complete","payment_status":"paid"}}}`)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: setting.StripeWebhookSecret})
	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", io.NopCloser(bytes.NewReader(signed.Payload)))
	req.Header.Set("Stripe-Signature", signed.Header)
	recorder := httptest.NewRecorder()
	engine := gin.New()
	engine.POST("/api/stripe/webhook", StripeWebhook)
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}
