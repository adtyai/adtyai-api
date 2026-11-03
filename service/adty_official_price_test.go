package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 官方价（任务 8.4）：同步 models.dev，测试用 testdata 里的精简样本，不联网。

func loadModelsDevSample(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/models-dev-api.json")
	require.NoError(t, err)
	return raw
}

// useModelsDevServer 把同步地址换成本地服务器，status 不是 200 时返回错误码。
func useModelsDevServer(t *testing.T, status int) {
	t.Helper()
	body := loadModelsDevSample(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "boom", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	oldURL, oldClient := adtyModelsDevURL, adtyModelsDevClient
	adtyModelsDevURL, adtyModelsDevClient = server.URL+"/api.json", server.Client()
	t.Cleanup(func() {
		adtyModelsDevURL, adtyModelsDevClient = oldURL, oldClient
		server.Close()
	})
}

func setupOfficialPriceDB(t *testing.T, enabledModels ...string) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Ability{}))
	require.NoError(t, model.MigrateAdtyTables())
	for i, name := range enabledModels {
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: name, ChannelId: i + 1, Enabled: true}).Error)
	}
	return db
}

func officialDec(v string) *decimal.Decimal {
	d := decimal.RequireFromString(v)
	return &d
}

func assertOfficialPrice(t *testing.T, want string, got *decimal.Decimal, field string) {
	t.Helper()
	if want == "" {
		assert.Nil(t, got, field)
		return
	}
	require.NotNil(t, got, field)
	assert.True(t, decimal.RequireFromString(want).Equal(*got), "%s: want %s got %s", field, want, got.String())
}

func TestParseModelsDevCosts(t *testing.T) {
	costs, err := parseModelsDevCosts(loadModelsDevSample(t))
	require.NoError(t, err)

	sonnet, ok := costs["anthropic/claude-sonnet-4-5"]
	require.True(t, ok, "带 tiers 和 context_over_200k 的条目要能解析")
	assertOfficialPrice(t, "3", sonnet.Input, "input")
	assertOfficialPrice(t, "15", sonnet.Output, "output")
	assertOfficialPrice(t, "0.3", sonnet.CacheRead, "cache_read")
	assertOfficialPrice(t, "3.75", sonnet.CacheWrite, "cache_write")

	gpt, ok := costs["openai/gpt-4o"]
	require.True(t, ok)
	assertOfficialPrice(t, "2.5", gpt.Input, "input")
	assertOfficialPrice(t, "", gpt.CacheWrite, "cache_write")

	_, ok = costs["openrouter/anthropic/claude-3.5-haiku"]
	assert.True(t, ok, "模型 ID 自己带斜杠")
	_, ok = costs["openai/gpt-free"]
	assert.False(t, ok, "没有价格的条目不算")

	_, err = parseModelsDevCosts([]byte(`{}`))
	assert.Error(t, err, "空数据当失败")
}

func TestAutoMapExactID(t *testing.T) {
	db := setupOfficialPriceDB(t, "claude-sonnet-4-5", "deepseek-reasoner", "deepseek-chat")
	useModelsDevServer(t, http.StatusOK)

	updated, err := SyncOfficialPrices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, updated)

	var sonnet model.AdtyOfficialPrice
	require.NoError(t, db.First(&sonnet, "model_name = ?", "claude-sonnet-4-5").Error)
	assert.Equal(t, "anthropic/claude-sonnet-4-5", sonnet.SourceId, "按 provider 顺序先找到 anthropic，不用 openai 的同名条目")
	assert.Equal(t, model.AdtyOfficialPriceSourceModelsDev, sonnet.Source)
	assertOfficialPrice(t, "3", sonnet.InputPerMTok, "input")
	assertOfficialPrice(t, "3.75", sonnet.CacheWritePerMTok, "cache_write")
	assert.NotZero(t, sonnet.SyncedAt)

	var reasoner model.AdtyOfficialPrice
	require.NoError(t, db.First(&reasoner, "model_name = ?", "deepseek-reasoner").Error)
	assert.Equal(t, "deepseek/deepseek-reasoner", reasoner.SourceId)

	var count int64
	require.NoError(t, db.Model(&model.AdtyOfficialPrice{}).Where("model_name = ?", "deepseek-chat").Count(&count).Error)
	assert.Zero(t, count, "找不到的模型不建行，等手动映射")
}

func TestSyncKeepsOverride(t *testing.T) {
	db := setupOfficialPriceDB(t, "claude-sonnet-4-5", "gpt-4o", "my-haiku")
	require.NoError(t, db.Create(&[]model.AdtyOfficialPrice{
		{ModelName: "claude-sonnet-4-5", SourceId: "anthropic/claude-sonnet-4-5", InputPerMTok: officialDec("1"), Source: model.AdtyOfficialPriceSourceManual, Override: true},
		{ModelName: "gpt-4o", SourceId: "openai/gpt-4o", InputPerMTok: officialDec("1"), PerCall: officialDec("0.04"), Source: model.AdtyOfficialPriceSourceModelsDev},
		{ModelName: "my-haiku", SourceId: "openrouter/anthropic/claude-3.5-haiku", Source: model.AdtyOfficialPriceSourceManual},
	}).Error)
	useModelsDevServer(t, http.StatusOK)

	updated, err := SyncOfficialPrices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, updated)

	var sonnet model.AdtyOfficialPrice
	require.NoError(t, db.First(&sonnet, "model_name = ?", "claude-sonnet-4-5").Error)
	assertOfficialPrice(t, "1", sonnet.InputPerMTok, "手动覆盖的不动")
	assertOfficialPrice(t, "", sonnet.OutputPerMTok, "手动覆盖的不动")
	assert.True(t, sonnet.Override)

	var gpt model.AdtyOfficialPrice
	require.NoError(t, db.First(&gpt, "model_name = ?", "gpt-4o").Error)
	assertOfficialPrice(t, "2.5", gpt.InputPerMTok, "input")
	assertOfficialPrice(t, "10", gpt.OutputPerMTok, "output")
	assertOfficialPrice(t, "0.04", gpt.PerCall, "按次价是手填的，同步不动")

	var haiku model.AdtyOfficialPrice
	require.NoError(t, db.First(&haiku, "model_name = ?", "my-haiku").Error)
	assertOfficialPrice(t, "0.8", haiku.InputPerMTok, "手动映射、没覆盖的照样同步")
	assert.Equal(t, model.AdtyOfficialPriceSourceModelsDev, haiku.Source)
}

func TestSyncFailureKeepsOldData(t *testing.T) {
	db := setupOfficialPriceDB(t, "gpt-4o", "claude-opus-4-1")
	require.NoError(t, db.Create(&model.AdtyOfficialPrice{
		ModelName: "gpt-4o", SourceId: "openai/gpt-4o", InputPerMTok: officialDec("7"), Source: model.AdtyOfficialPriceSourceModelsDev, SyncedAt: 100,
	}).Error)
	useModelsDevServer(t, http.StatusInternalServerError)

	updated, err := SyncOfficialPrices(t.Context())
	require.Error(t, err)
	assert.Zero(t, updated)

	var rows []model.AdtyOfficialPrice
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1, "失败时不自动映射新模型")
	assertOfficialPrice(t, "7", rows[0].InputPerMTok, "旧价格保留")
	assert.Equal(t, int64(100), rows[0].SyncedAt)
}
