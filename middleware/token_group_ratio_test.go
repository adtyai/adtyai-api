package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupTokenGroupRatioDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	previousMaster, previousMemory, previousSQLite := common.IsMasterNode, common.MemoryCacheEnabled, common.SQLitePath
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = false, false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "token-group-ratio.db")
	require.NoError(t, model.InitDB())
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}))
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousType, previousLogType)
		common.RedisEnabled = previousRedis
		common.IsMasterNode, common.MemoryCacheEnabled, common.SQLitePath = previousMaster, previousMemory, previousSQLite
		require.NoError(t, sqlDB.Close())
	})
}

func TestTokenAuthRejectsIdentityMissingFromBillingTable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	setupTokenGroupRatioDB(t)
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))

	cases := []struct {
		name       string
		userGroup  string
		tokenGroup string
		wantStatus int
		wantUsing  string
		wantBody   string
	}{
		{name: "default token", userGroup: "orphan", tokenGroup: "", wantStatus: http.StatusForbidden, wantBody: "Group orphan has been deprecated"},
		{name: "auto token", userGroup: "orphan", tokenGroup: "auto", wantStatus: http.StatusForbidden, wantBody: "Group orphan has been deprecated"},
		{name: "pinned billing group", userGroup: "orphan", tokenGroup: "default", wantStatus: http.StatusOK, wantUsing: "default"},
		{name: "hidden identity still billed", userGroup: "vip", tokenGroup: "", wantStatus: http.StatusOK, wantUsing: "vip"},
		{name: "auto identity", userGroup: "auto", tokenGroup: "", wantStatus: http.StatusOK, wantUsing: "auto"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := &model.User{
				Username: fmt.Sprintf("tgratio%d", i), Password: "unused-password",
				Status: common.UserStatusEnabled, Group: tc.userGroup, AffCode: fmt.Sprintf("tgraff%d", i),
			}
			require.NoError(t, model.DB.Create(user).Error)
			token := &model.Token{
				UserId: user.Id, Key: fmt.Sprintf("tgratiokey%d", i), Name: tc.name,
				Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: tc.tokenGroup,
			}
			require.NoError(t, model.DB.Create(token).Error)

			called := false
			using := "unset"
			engine := gin.New()
			engine.POST("/v1/chat/completions", TokenAuth(), func(c *gin.Context) {
				called = true
				using = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
				c.Status(http.StatusOK)
			})
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			request.Header.Set("Authorization", "Bearer sk-"+token.Key)
			request.Header.Set("Accept-Language", "en")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			if tc.wantStatus == http.StatusOK {
				require.True(t, called)
				require.Equal(t, tc.wantUsing, using)
				return
			}
			require.False(t, called)
			require.Contains(t, recorder.Body.String(), tc.wantBody)
		})
	}
}

func TestDistributeRejectsUsingGroupMissingFromBillingTable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	originalRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	called := false
	engine := gin.New()
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "orphan")
		c.Next()
	}, Distribute(), func(c *gin.Context) {
		called = true
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", "en")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	require.False(t, called)
	require.Contains(t, recorder.Body.String(), "Group orphan has been deprecated")
}
