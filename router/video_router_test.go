package router

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/adtylimit"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVideoRemixUsesUserLimitBeforeSubmission(t *testing.T) {
	oldAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://main.example.com"
	t.Cleanup(func() { system_setting.ServerAddress = oldAddress })
	require.NoError(t, i18n.Init())
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, opened.Close())
	})
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.AdtyUserLimit{}))
	oldRedis := common.RedisEnabled
	oldMemory := common.MemoryCacheEnabled
	oldRate := setting.ModelRequestRateLimitEnabled
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	setting.ModelRequestRateLimitEnabled = false
	limitSetting := adty_setting.GetLimitSetting()
	savedLimit := *limitSetting
	*limitSetting = adty_setting.LimitSetting{Enabled: true, DefaultRPM: 1}
	middleware.SetAdtyLimitStoreForTest(t, adtylimit.NewMemoryStore(), time.Now)
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		common.MemoryCacheEnabled = oldMemory
		setting.ModelRequestRateLimitEnabled = oldRate
		*limitSetting = savedLimit
	})

	user := &model.User{
		Username: "remix-limit",
		Password: "x",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Quota:    1000,
	}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{
		UserId:         user.Id,
		Key:            "remixkey",
		Name:           "remix",
		Status:         common.TokenStatusEnabled,
		RemainQuota:    1000,
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}
	require.NoError(t, model.DB.Create(token).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetVideoRouter(engine)

	remix := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/videos/video_1/remix", strings.NewReader(`{}`))
		request.Host = "main.example.com"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer sk-remixkey")
		engine.ServeHTTP(recorder, request)
		return recorder
	}
	first := remix()
	require.NotEqual(t, http.StatusTooManyRequests, first.Code, first.Body.String())
	second := remix()
	require.Equal(t, http.StatusTooManyRequests, second.Code, second.Body.String())
	require.Contains(t, second.Body.String(), "rate_limit_exceeded")

	for range 3 {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/v1/video/generations/task_1", nil)
		request.Host = "main.example.com"
		request.Header.Set("Authorization", "Bearer sk-remixkey")
		engine.ServeHTTP(recorder, request)
		require.NotEqual(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
	}
}
