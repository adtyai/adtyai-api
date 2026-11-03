package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreemPayRequiresWebhook(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	originalKey, originalProducts, originalSecret := setting.CreemApiKey, setting.CreemProducts, setting.CreemWebhookSecret
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMain, previousLog)
		setting.CreemApiKey, setting.CreemProducts, setting.CreemWebhookSecret = originalKey, originalProducts, originalSecret
	})
	user := &model.User{Username: "creem-buyer", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Email: "buyer@example.com"}
	require.NoError(t, db.Create(user).Error)
	setting.CreemApiKey = "creem_key"
	setting.CreemProducts = `[{"productId":"prod_1","name":"pack","price":2,"currency":"USD","quota":1000}]`
	setting.CreemWebhookSecret = ""

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/creem/pay", nil)
	ctx.Set("id", user.Id)
	(&CreemAdaptor{}).RequestPay(ctx, &CreemPayRequest{PaymentMethod: model.PaymentMethodCreem, ProductId: "prod_1"})
	assert.Contains(t, recorder.Body.String(), "Creem payment is not available")
	var orders int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&orders).Error)
	assert.Zero(t, orders)
}
