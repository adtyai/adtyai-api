package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMarketingSecretUpdateRollsBackWhenResendKeyIsRejected(t *testing.T) {
	require.NoError(t, i18n.Init())
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Option{}))
	previousDB := model.DB
	model.DB = database
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"EmailProvider": "resend",
		"ResendApiKey":  "re_live_old",
		"ResendFrom":    "ADTY AI <news@example.com>",
	}
	common.OptionMapRWMutex.Unlock()
	cfg := adty_setting.GetMarketingSetting()
	previousCfg := *cfg
	*cfg = previousCfg
	cfg.Enabled = false
	cfg.FromAddress = "ADTY AI <news@example.com>"
	cfg.FooterCompany = "ADTY"
	cfg.FooterAddress = "1 Market Street"
	cfg.WebhookSecret = "whsec_oldsecret"
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		*cfg = previousCfg
	})
	require.NoError(t, database.Create(&model.Option{Key: adty_setting.MarketingKeyWebhookSecret, Value: "whsec_oldsecret"}).Error)
	require.NoError(t, database.Create(&model.Option{Key: "ResendApiKey", Value: "re_live_old"}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/adty/admin/marketing/secrets", strings.NewReader(`{"webhook_secret":"whsec_newsecret","clear_resend_api_key":true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	PutAdtyMarketingSecrets(ctx)

	require.Contains(t, recorder.Body.String(), `"success":false`)
	var webhook model.Option
	require.NoError(t, database.Where("key = ?", adty_setting.MarketingKeyWebhookSecret).First(&webhook).Error)
	require.Equal(t, "whsec_oldsecret", webhook.Value)
	var resend model.Option
	require.NoError(t, database.Where("key = ?", "ResendApiKey").First(&resend).Error)
	require.Equal(t, "re_live_old", resend.Value)
	require.Equal(t, "whsec_oldsecret", cfg.WebhookSecret)
}
