package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 改名（任务 8.9）：调用方和浏览器能看到的报错类型、崩溃响应、登录 cookie 和
// 后台 JWT 的签发方、受众。

func TestErrorTypeIsAdtyai(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assert.Equal(t, types.ErrorType("adtyai_error"), types.ErrorTypeNewAPIError)

	for _, tc := range []struct {
		name  string
		abort func(c *gin.Context)
	}{
		{"openai", func(c *gin.Context) { abortWithOpenAiMessage(c, http.StatusForbidden, "denied") }},
		{"midjourney", func(c *gin.Context) { abortWithMidjourneyMessage(c, http.StatusForbidden, 4, "denied") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/", tc.abort)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

			require.Equal(t, http.StatusForbidden, recorder.Code)
			var body map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			if inner, ok := body["error"].(map[string]any); ok {
				body = inner
			}
			assert.Equal(t, "adtyai_error", body["type"])
		})
	}
}

func TestPanicResponseHidesDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RelayPanicRecover())
	router.GET("/", func(c *gin.Context) { panic("secret-upstream-key sk-live-123") })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "secret-upstream-key")
	assert.NotContains(t, recorder.Body.String(), "github.com")
	assert.JSONEq(t, `{"error":{"message":"Internal server error, please contact support","type":"adtyai_panic"}}`, recorder.Body.String())
}

func TestAuthCookieNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 不是合法的刷新令牌，WriteRefreshCookie 不会去查库，按默认有效期写 cookie。
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/auth/refresh", nil)
	service.WriteRefreshCookie(c, "not-a-refresh-token")

	cookies := map[string]*http.Cookie{}
	for _, cookie := range recorder.Result().Cookies() {
		cookies[cookie.Name] = cookie
	}
	require.Len(t, cookies, 2)
	refresh := cookies["adtyai_refresh"]
	require.NotNil(t, refresh)
	hint := cookies["adtyai_has_session"]
	require.NotNil(t, hint)
	// 改名不改属性：刷新 cookie 仍然 HttpOnly、只发给 /api/user/auth、SameSite=Strict。
	assert.True(t, refresh.HttpOnly)
	assert.Equal(t, "/api/user/auth", refresh.Path)
	assert.Equal(t, http.SameSiteStrictMode, refresh.SameSite)
	assert.False(t, hint.HttpOnly)
	assert.Equal(t, "/", hint.Path)
	assert.Equal(t, "1", hint.Value)
	assert.Equal(t, http.SameSiteStrictMode, hint.SameSite)

	clear := httptest.NewRecorder()
	c, _ = gin.CreateTestContext(clear)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/auth/logout", nil)
	service.ClearRefreshCookie(c)
	cleared := map[string]int{}
	for _, cookie := range clear.Result().Cookies() {
		cleared[cookie.Name] = cookie.MaxAge
	}
	assert.Equal(t, map[string]int{"adtyai_refresh": -1, "adtyai_has_session": -1}, cleared)

	// 后台登录 JWT 的签发方和受众；上游名字签发的令牌不再被当成后台令牌。
	previousSecret := common.SessionSecret
	common.SessionSecret = "adty-brand-test-secret"
	t.Cleanup(func() { common.SessionSecret = previousSecret })
	identity := service.AuthIdentity{UserID: 7, SessionID: "sid-7", UserAuthVersion: 1, SessionVersion: 1}
	token, _, err := service.IssueAccessToken(identity)
	require.NoError(t, err)
	claims := jwt.MapClaims{}
	_, _, err = jwt.NewParser().ParseUnverified(token, claims)
	require.NoError(t, err)
	assert.Equal(t, "adtyai", claims["iss"])
	assert.Equal(t, []any{"adtyai-dashboard"}, claims["aud"])

	parsed, internal, err := service.ParseDashboardAccessToken(token)
	require.NoError(t, err)
	assert.True(t, internal)
	assert.Equal(t, identity, parsed)
}
