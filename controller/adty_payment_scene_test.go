package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func sceneTestSetting[T any](t *testing.T, target *T, value T) {
	t.Helper()
	previous := *target
	*target = value
	t.Cleanup(func() { *target = previous })
}

func TestPaymentScenesConstrainNativeProviderEntries(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	user := model.User{Username: "scene-native", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Email: "scene@example.com"}
	require.NoError(t, db.Create(&user).Error)
	sceneTestSetting(t, &common.RedisEnabled, false)
	sceneTestSetting(t, &operation_setting.GetGeneralSetting().QuotaDisplayType, operation_setting.QuotaDisplayTypeUSD)
	sceneTestSetting(t, &common.QuotaPerUnit, 500000.0)
	sceneTestSetting(t, &operation_setting.MinTopUp, 1)
	sceneTestSetting(t, setting.GetBEpusdtSetting(), setting.BEpusdtSetting{Enabled: true, BaseURL: "https://scene.example", Secret: "synthetic-scene-secret", TradeTypes: []string{"usdt.trc20"}})
	sceneTestSetting(t, &setting.StripeApiSecret, "sk_test_synthetic_scene")
	sceneTestSetting(t, &setting.StripeWebhookSecret, "synthetic-stripe-webhook")
	sceneTestSetting(t, &setting.StripePriceId, "price_scene")
	sceneTestSetting(t, &setting.StripeMinTopUp, 1)
	sceneTestSetting(t, &setting.StripeUnitPrice, 1.0)
	sceneTestSetting(t, &setting.CreemApiKey, "synthetic-creem-key")
	sceneTestSetting(t, &setting.CreemWebhookSecret, "synthetic-creem-webhook")
	sceneTestSetting(t, &setting.CreemProducts, `[{"productId":"prod_scene","name":"Scene pack","price":100,"currency":"EUR","quota":50000000}]`)
	sceneTestSetting(t, &setting.WaffoEnabled, true)
	sceneTestSetting(t, &setting.WaffoSandbox, true)
	sceneTestSetting(t, &setting.WaffoSandboxApiKey, "synthetic-waffo-key")
	sceneTestSetting(t, &setting.WaffoSandboxPrivateKey, "synthetic-waffo-private")
	sceneTestSetting(t, &setting.WaffoSandboxPublicCert, "synthetic-waffo-public")
	sceneTestSetting(t, &setting.WaffoCurrency, "USD")
	sceneTestSetting(t, &setting.WaffoUnitPrice, 1.0)
	sceneTestSetting(t, &setting.WaffoMinTopUp, 1)
	sceneTestSetting(t, &setting.WaffoPancakeMerchantID, "synthetic-merchant")
	sceneTestSetting(t, &setting.WaffoPancakePrivateKey, "synthetic-private")
	sceneTestSetting(t, &setting.WaffoPancakeProductID, "synthetic-product")
	sceneTestSetting(t, &setting.WaffoPancakeEnvironment, "test")
	sceneTestSetting(t, &setting.WaffoPancakeUnitPrice, 1.0)
	sceneTestSetting(t, &setting.WaffoPancakeMinTopUp, 1)
	externalCalls := 0
	sceneTestSetting[http.RoundTripper](t, &http.DefaultTransport, emailHTTPTransport(func(*http.Request) (*http.Response, error) {
		externalCalls++
		return nil, errors.New("a denied scene must not call a provider")
	}))
	policy := []model.AdtyPaymentScene{{Key: "restricted", Name: "Restricted", Business: "wallet_topup", Enabled: true, Providers: []string{"bepusdt"}, SiteScope: "all", MinUSD: decimal.Zero, MaxUSD: decimal.NewFromInt(50)}}
	payload, err := common.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.AdtyPaymentSceneConfig{}).Where("id = ?", 1).Updates(map[string]any{"payload": string(payload), "version": 2}).Error)
	for _, name := range []string{model.PaymentProviderStripe, model.PaymentProviderCreem, model.PaymentProviderWaffo, model.PaymentProviderWaffoPancake} {
		provider := service.AdtyPaymentProviderByName(name)
		require.NotNil(t, provider, "retired providers stay registered for historical admin operations")
		require.Empty(t, provider.Methods())
	}
	for _, method := range []string{"stripe", "creem:prod_scene", "waffo", "waffo_pancake"} {
		require.Nil(t, service.AdtyPaymentProviderForMethod(method), "retired methods are not in the customer catalog")
	}
	for _, entry := range []struct {
		name, method, body string
		handler            gin.HandlerFunc
	}{
		{"bepusdt native", "usdt.trc20", `{"amount":100,"chain":"usdt.trc20","payment_scene":"restricted"}`, RequestBEpusdtPay},
		{"stripe native", "stripe", `{"amount":100,"payment_method":"stripe","payment_scene":"restricted"}`, RequestStripePay},
		{"creem native", "creem:prod_scene", `{"product_id":"prod_scene","payment_method":"creem","payment_scene":"restricted"}`, RequestCreemPay},
		{"waffo native", "waffo", `{"amount":100,"payment_scene":"restricted"}`, RequestWaffoPay},
		{"pancake native", "waffo_pancake", `{"amount":100,"payment_scene":"restricted"}`, RequestWaffoPancakePay},
		{"stripe unified", "stripe", `{"amount":100,"method":"stripe","payment_scene":"restricted"}`, PostAdtyHostedPayment},
		{"creem unified", "creem:prod_scene", `{"amount":100,"method":"creem:prod_scene","payment_scene":"restricted"}`, PostAdtyHostedPayment},
		{"waffo unified", "waffo", `{"amount":100,"method":"waffo","payment_scene":"restricted"}`, PostAdtyHostedPayment},
		{"pancake unified", "waffo_pancake", `{"amount":100,"method":"waffo_pancake","payment_scene":"restricted"}`, PostAdtyHostedPayment},
	} {
		t.Run(entry.name, func(t *testing.T) {
			if entry.method == "usdt.trc20" {
				provider := service.AdtyPaymentProviderForMethod(entry.method)
				require.NotNil(t, provider)
				require.True(t, provider.AvailableFor(entry.method, &service.AdtyQuote{FaceUSD: decimal.NewFromInt(100), PayAmount: decimal.NewFromInt(100)}), "provider must be configured; the scene is what rejects this request")
			}
			response := callStripe(t, entry.handler, user.Id, entry.body)
			require.NotContains(t, response.Body.String(), `"success":true`)
			var body struct {
				Success bool
				Message string
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			require.False(t, body.Success)
			require.NotEmpty(t, body.Message)
			var count int64
			require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
			require.Zero(t, count, "denied native and unified calls cannot create orders")
			require.Zero(t, externalCalls)
		})
	}
	for _, entry := range []struct {
		name    string
		handler gin.HandlerFunc
		body    string
	}{
		{"creem native leftover method", RequestCreemPay, `{"product_id":"prod_scene","payment_method":"creem","payment_scene":"restricted"}`},
	} {
		response := callStripe(t, entry.handler, user.Id, entry.body)
		require.NotContains(t, response.Body.String(), `"success":true`, entry.name)
		var count int64
		require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
		require.Zero(t, count, "retired hosted pay cannot create orders")
		require.Zero(t, externalCalls)
	}
}
