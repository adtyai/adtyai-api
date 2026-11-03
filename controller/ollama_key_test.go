package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaAdminUsesEnabledKeyNotTheFirstDisabledKey(t *testing.T) {
	authorization := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"llama3"}]}`))
		default:
			_, _ = w.Write([]byte(`{"version":"0.5.1"}`))
		}
	}))
	t.Cleanup(upstream.Close)

	baseURL := upstream.URL
	channel := &model.Channel{
		Type: constant.ChannelTypeOllama, Key: "disabled-key\nenabled-key", BaseURL: &baseURL,
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true, MultiKeySize: 2,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled},
		},
	}
	models, err := fetchChannelUpstreamModelIDs(channel)
	require.NoError(t, err)
	assert.Equal(t, []string{"llama3"}, models)
	assert.Equal(t, "Bearer enabled-key", <-authorization)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMaster, previousSQLite := common.IsMasterNode, common.SQLitePath
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousType, previousLogType)
		common.IsMasterNode, common.SQLitePath = previousMaster, previousSQLite
	})
	t.Setenv("SQL_DSN", os.Getenv("TEST_CHANNEL_SQL_DSN"))
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode = true
	common.SQLitePath = filepath.Join(t.TempDir(), "ollama-key.db")
	require.NoError(t, model.InitDB())
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = model.DB
	stored := *channel
	stored.Name = "ollama-admin"
	stored.Group = "default"
	stored.Models = "llama3"
	require.NoError(t, model.DB.Create(&stored).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(stored.Id)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/"+strconv.Itoa(stored.Id)+"/ollama/version", nil)
	OllamaVersion(c)
	require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"version":"0.5.1"`)
	assert.Equal(t, "Bearer enabled-key", <-authorization)
}
