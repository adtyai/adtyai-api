package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 官方价公开接口（任务 8.4）和上游 /api/pricing 的现有行为。

func withHeaderNavModules(t *testing.T, raw string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	wasNil := common.OptionMap == nil
	if wasNil {
		common.OptionMap = map[string]string{}
	}
	old, had := common.OptionMap["HeaderNavModules"]
	common.OptionMap["HeaderNavModules"] = raw
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if wasNil {
			common.OptionMap = nil
		} else if had {
			common.OptionMap["HeaderNavModules"] = old
		} else {
			delete(common.OptionMap, "HeaderNavModules")
		}
	})
}

func TestOfficialPricesPublicEndpoint(t *testing.T) {
	db := adtyTestDB(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AuditLog{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	input, output, perCall := decimal.RequireFromString("3"), decimal.RequireFromString("15"), decimal.RequireFromString("0.04")
	require.NoError(t, db.Create(&[]model.AdtyOfficialPrice{
		{ModelName: "claude-sonnet-4-5", SourceId: "anthropic/claude-sonnet-4-5", InputPerMTok: &input, OutputPerMTok: &output, Source: model.AdtyOfficialPriceSourceModelsDev},
		{ModelName: "gpt-image-1", PerCall: &perCall, Source: model.AdtyOfficialPriceSourceManual, Override: true},
		{ModelName: "deepseek-chat", SourceId: "deepseek/deepseek-chat", Source: model.AdtyOfficialPriceSourceManual},
	}).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/adty/pricing/official", middleware.HeaderNavModuleAuth("pricing"), GetAdtyOfficialPrices)
	// 鉴权由路由上的 RootAuth 负责，这里直接挂处理函数。gin 要求在第一次请求前注册完所有路由。
	engine.PUT("/api/adty/admin/official-prices/*model", PutAdtyOfficialPrice)
	get := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/adty/pricing/official", nil))
		return recorder
	}

	withHeaderNavModules(t, `{"pricing":{"enabled":true,"requireAuth":false}}`)
	recorder := get()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Prices    map[string]map[string]*string `json:"prices"`
			UpdatedAt int64                         `json:"updated_at"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	prices := envelope.Data.Prices
	require.Len(t, prices, 2, "没有任何官方价的模型不返回")
	require.Contains(t, prices, "claude-sonnet-4-5")
	sonnet := prices["claude-sonnet-4-5"]
	require.NotNil(t, sonnet["input"])
	assert.Equal(t, "3", *sonnet["input"])
	assert.Equal(t, "15", *sonnet["output"])
	assert.Contains(t, sonnet, "cache_read", "没有的价格返回 null，不省略")
	assert.Nil(t, sonnet["cache_read"])
	assert.Nil(t, sonnet["per_call"])
	require.NotNil(t, prices["gpt-image-1"]["per_call"])
	assert.Equal(t, "0.04", *prices["gpt-image-1"]["per_call"])
	assert.NotZero(t, envelope.Data.UpdatedAt)
	assert.NotContains(t, recorder.Body.String(), "source_id", "公开接口不带映射信息")

	// 后台保存：模型名带斜杠也能改，保存后公开接口立即看到；负数价格被拒。
	put := func(body string) string {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/adty/admin/official-prices/deepseek-ai/DeepSeek-V3", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusOK, recorder.Code)
		return recorder.Body.String()
	}
	assert.Contains(t, put(`{"input_per_mtok":"-1"}`), "Prices cannot be negative")
	assert.Contains(t, put(`{"source_id":"deepseek"}`), "The models.dev ID must look like provider/model")
	assert.Contains(t, put(`{"source_id":"deepseek/deepseek-chat","input_per_mtok":0.27,"override":true}`), `"success":true`)
	var saved model.AdtyOfficialPrice
	require.NoError(t, db.First(&saved, "model_name = ?", "deepseek-ai/DeepSeek-V3").Error)
	assert.True(t, saved.Override)
	assert.Equal(t, model.AdtyOfficialPriceSourceManual, saved.Source)
	require.NoError(t, common.Unmarshal(get().Body.Bytes(), &envelope))
	require.Contains(t, envelope.Data.Prices, "deepseek-ai/DeepSeek-V3")
	assert.Equal(t, "0.27", *envelope.Data.Prices["deepseek-ai/DeepSeek-V3"]["input"])

	withHeaderNavModules(t, `{"pricing":{"enabled":false}}`)
	assert.Equal(t, http.StatusForbidden, get().Code, "和价格页同一个开关")
}

func TestUpstreamPricingFiltersByUsableGroups(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	oldUsable := setting.UserUsableGroups2JSONString()
	oldRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatio))
		model.InvalidatePricingCache()
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default group"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":0.5,"hidden":0.1}`))
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "zz-public-model", ChannelId: 1, Enabled: true},
		{Group: "hidden", Model: "zz-hidden-model", ChannelId: 2, Enabled: true},
	}).Error)
	model.InvalidatePricingCache()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
	GetPricing(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success     bool               `json:"success"`
		Data        []model.Pricing    `json:"data"`
		GroupRatio  map[string]float64 `json:"group_ratio"`
		UsableGroup map[string]string  `json:"usable_group"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	assert.Equal(t, map[string]float64{"default": 1}, payload.GroupRatio, "未登录只返回访客可选分组的倍率")
	assert.Equal(t, map[string]string{"default": "Default group"}, payload.UsableGroup)
	names := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		names = append(names, item.ModelName)
	}
	assert.Contains(t, names, "zz-public-model")
	assert.NotContains(t, names, "zz-hidden-model", "只在隐藏分组里的模型不返回")
}
