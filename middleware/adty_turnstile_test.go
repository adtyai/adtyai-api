package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TurnstileCheck 按当前站点（11.2 的中间件放进 gin 上下文的 adty_site）选 secret，并比对 hostname（任务 11.8）。
func TestTurnstileCheckUsesSiteSecret(t *testing.T) {
	var secrets []string
	verifyHost := "partner.sites.example.com"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		secrets = append(secrets, r.PostForm.Get("secret"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"hostname":"`+verifyHost+`"}`)
	}))
	defer server.Close()

	oldEnabled, oldSecret := common.TurnstileCheckEnabled, common.TurnstileSecretKey
	oldURL, oldChoose := adtyTurnstileSiteverifyURL, AdtyTurnstileSecretFor
	common.TurnstileCheckEnabled, common.TurnstileSecretKey = true, "main-secret"
	adtyTurnstileSiteverifyURL = server.URL
	AdtyTurnstileSecretFor = func(site *model.AdtySite, host string) string {
		if site != nil && site.Id == 7 {
			return "site-secret"
		}
		return ""
	}
	t.Cleanup(func() {
		common.TurnstileCheckEnabled, common.TurnstileSecretKey = oldEnabled, oldSecret
		adtyTurnstileSiteverifyURL, AdtyTurnstileSecretFor = oldURL, oldChoose
	})

	serve := func(site *model.AdtySite, host string) *httptest.ResponseRecorder {
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		engine.GET("/login", func(c *gin.Context) {
			if site != nil {
				c.Set(AdtySiteContextKey, site)
			}
			c.Next()
		}, TurnstileCheck(), func(c *gin.Context) { c.String(http.StatusOK, "passed") })
		req := httptest.NewRequest(http.MethodGet, "/login?turnstile="+url.QueryEscape("token"), nil)
		req.Host = host
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}

	rec := serve(&model.AdtySite{Id: 7}, "partner.sites.example.com")
	assert.Equal(t, "passed", rec.Body.String())
	assert.Equal(t, []string{"site-secret"}, secrets, "分站用自己 widget 的 secret")

	// token 是在别的域名上拿到的：不放行
	verifyHost = "other.example.com"
	rec = serve(&model.AdtySite{Id: 7}, "partner.sites.example.com")
	assert.NotEqual(t, "passed", rec.Body.String())

	// 主站：用上游的 TurnstileSecretKey，照上游行为不比对 hostname
	rec = serve(nil, "adtyai.com")
	assert.Equal(t, "passed", rec.Body.String())
	assert.Equal(t, "main-secret", secrets[len(secrets)-1])
}
