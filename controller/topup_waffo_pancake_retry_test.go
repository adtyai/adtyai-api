package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPancakeWebhookRetriesWhenOrderLookupFails(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	originalVerify := verifyWaffoPancakeWebhook
	originalMerchant, originalKey, originalProduct := setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMain, previousLog)
		verifyWaffoPancakeWebhook = originalVerify
		setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID = originalMerchant, originalKey, originalProduct
	})
	setting.WaffoPancakeMerchantID = "merchant"
	setting.WaffoPancakePrivateKey = "key"
	setting.WaffoPancakeProductID = "product"
	verifyWaffoPancakeWebhook = func(string, string) (*service.WaffoPancakeWebhookEvent, error) {
		return &service.WaffoPancakeWebhookEvent{
			EventType: "order.completed",
			Mode:      "test",
			Data:      service.WaffoPancakeWebhookData{OrderMerchantExternalID: "PAN-MISSING"},
		}, nil
	}

	engine := gin.New()
	engine.POST("/api/waffo-pancake/webhook/:env", WaffoPancakeWebhook)
	req := httptest.NewRequest(http.MethodPost, "/api/waffo-pancake/webhook/test", bytes.NewReader([]byte("{}")))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestPancakeCatalogIgnoresQueryPrivateKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalMerchant, originalKey := setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey = originalMerchant, originalKey
	})
	setting.WaffoPancakeMerchantID = ""
	setting.WaffoPancakePrivateKey = ""
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/option/waffo-pancake/catalog?merchant_id=m&private_key=SECRET", nil)
	ListWaffoPancakeCatalog(ctx)
	require.Contains(t, recorder.Body.String(), "Waffo Pancake credentials are not configured")
	require.NotContains(t, recorder.Body.String(), "SECRET")
}
