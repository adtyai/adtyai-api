package router

import (
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
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 按用户限制挂在每个中转入口上（任务 7.4）：用一个已经用光 RPM 的用户打真实路由，都返回 429。
// MJ 组的 Distribute 挂在组上、先于提交路由上的限流，要有渠道才走得到，MJ 的格式在 middleware 包里测。
func TestAdtyLimitMountedOnResponses(t *testing.T) {
	f := newAdtyGuardFixture(t)
	require.NoError(t, i18n.Init())
	setting := adty_setting.GetLimitSetting()
	saved := *setting
	*setting = adty_setting.LimitSetting{Enabled: true, DefaultRPM: 1}
	t.Cleanup(func() { *setting = saved })
	store := adtylimit.NewMemoryStore()
	now := time.Unix(1_700_000_040, 0)
	middleware.SetAdtyLimitStoreForTest(t, store, func() time.Time { return now })

	engine := gin.New()
	SetRelayRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetVideoRouter(engine)
	SetTaskRouter(engine)

	var user model.User
	adtySessionToken(t, f.db, "limited", common.RoleCommonUser)
	require.NoError(t, f.db.Where("username = ?", "limited").First(&user).Error)
	require.NoError(t, f.db.Model(&user).Update("quota", 1_000_000).Error)
	key := "sk-" + common.GetRandomString(48)
	token := &model.Token{UserId: user.Id, Key: key[3:], Name: "limit", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "default"}
	require.NoError(t, f.db.Create(token).Error)
	// Use up this minute's single request.
	_, _ = store.IncrRPM(user.Id, adtylimit.MinuteOf(now))

	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/images/generations", "/v1beta/models/gemini-pro:generateContent"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-4o","prompt":"a cat"}`))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		assert.Equal(t, http.StatusTooManyRequests, recorder.Code, "%s: %s", path, recorder.Body.String())
	}
}

// Playground uses a dashboard session, not an API key, and still spends the
// same per-user RPM budget. A used-up user must be rejected before a channel
// is chosen.
func TestAdtyLimitMountedOnPlayground(t *testing.T) {
	f := newAdtyGuardFixture(t)
	require.NoError(t, i18n.Init())
	setting := adty_setting.GetLimitSetting()
	saved := *setting
	*setting = adty_setting.LimitSetting{Enabled: true, DefaultRPM: 1}
	t.Cleanup(func() { *setting = saved })
	store := adtylimit.NewMemoryStore()
	now := time.Unix(1_700_000_040, 0)
	middleware.SetAdtyLimitStoreForTest(t, store, func() time.Time { return now })

	engine := gin.New()
	SetRelayRouter(engine)

	session := adtySessionToken(t, f.db, "pg-limited", common.RoleCommonUser)
	var user model.User
	require.NoError(t, f.db.Where("username = ?", "pg-limited").First(&user).Error)
	_, _ = store.IncrRPM(user.Id, adtylimit.MinuteOf(now))

	req := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+session)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	assert.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
	assert.NotEmpty(t, recorder.Header().Get("Retry-After"))
}
