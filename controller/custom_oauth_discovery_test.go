package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFetchCustomOAuthDiscoveryBlocksRedirectToUnlistedHost(t *testing.T) {
	secret := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"ssrf-secret-body"}`))
	}))
	t.Cleanup(secret.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target, err := url.Parse(secret.URL + "/secret")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		target.Host = "localhost:" + target.Port()
		http.Redirect(w, r, target.String(), http.StatusFound)
	}))
	t.Cleanup(redirect.Close)

	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() { *fetchSetting = original })
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.DomainFilterMode = true
	fetchSetting.DomainList = nil
	fetchSetting.IpFilterMode = false
	fetchSetting.IpList = nil
	fetchSetting.ApplyIPFilterForDomain = true
	fetchSetting.AllowedPorts = []string{
		strconv.Itoa(portOf(t, redirect.Listener.Addr().String())),
		strconv.Itoa(portOf(t, secret.Listener.Addr().String())),
	}
	service.InitHttpClient()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/custom-oauth/discovery", strings.NewReader(`{"well_known_url":"`+redirect.URL+`/discover"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	FetchCustomOAuthDiscovery(ctx)

	body := recorder.Body.String()
	require.NotContains(t, body, "ssrf-secret-body")
	require.Contains(t, body, `"success":false`)
}

func portOf(t *testing.T, address string) int {
	t.Helper()
	_, portText, ok := strings.Cut(address, ":")
	require.True(t, ok)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	return port
}
