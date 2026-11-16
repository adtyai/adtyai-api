package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func callPaymentHandler(t *testing.T, handler gin.HandlerFunc, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/pay", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	handler(ctx)
	return recorder
}

func TestPaymentScenesConstrainNativeProviderEntries(t *testing.T) {
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
	externalCalls := 0
	sceneTestSetting[http.RoundTripper](t, &http.DefaultTransport, emailHTTPTransport(func(*http.Request) (*http.Response, error) {
		externalCalls++
		return nil, errors.New("a denied scene must not call a provider")
	}))
	policy := []model.AdtyPaymentScene{{Key: "restricted", Name: "Restricted", Business: "wallet_topup", Enabled: true, Providers: []string{"bepusdt"}, SiteScope: "all", MinUSD: decimal.Zero, MaxUSD: decimal.NewFromInt(50)}}
	payload, err := common.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.AdtyPaymentSceneConfig{}).Where("id = ?", 1).Updates(map[string]any{"payload": string(payload), "version": 2}).Error)

	provider := service.AdtyPaymentProviderForMethod("usdt.trc20")
	require.NotNil(t, provider)
	require.True(t, provider.AvailableFor("usdt.trc20", &service.AdtyQuote{FaceUSD: decimal.NewFromInt(100), PayAmount: decimal.NewFromInt(100)}), "provider must be configured; the scene is what rejects this request")
	response := callPaymentHandler(t, RequestBEpusdtPay, user.Id, `{"amount":100,"chain":"usdt.trc20","payment_scene":"restricted"}`)
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
	require.Zero(t, count, "a denied native call cannot create orders")
	require.Zero(t, externalCalls)
}
