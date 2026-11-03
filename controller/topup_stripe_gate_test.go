package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStripePayRequiresWebhook(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	previousKey := setting.StripeApiSecret
	previousWebhook := setting.StripeWebhookSecret
	previousPrice := setting.StripePriceId
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMain, previousLog)
		setting.StripeApiSecret = previousKey
		setting.StripeWebhookSecret = previousWebhook
		setting.StripePriceId = previousPrice
	})
	setting.StripeApiSecret = "sk_test_f073"
	setting.StripePriceId = "price_f073"
	setting.StripeWebhookSecret = ""

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/pay", strings.NewReader(`{"amount":1,"payment_method":"stripe"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	RequestStripePay(ctx)
	require.Contains(t, recorder.Body.String(), "Stripe payment is not available")
	var orders int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&orders).Error)
	require.Zero(t, orders)
}
