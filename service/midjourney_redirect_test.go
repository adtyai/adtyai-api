package service

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDoMidjourneyHttpRequestDoesNotForwardSecretAcrossOrigin(t *testing.T) {
	InitHttpClient()
	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() { *fetchSetting = original })
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.DomainFilterMode = false
	fetchSetting.IpFilterMode = false
	fetchSetting.ApplyIPFilterForDomain = false

	var stolen string
	otherHits := 0
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits++
		stolen = r.Header.Get("mj-api-secret")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":1}`)
	}))
	t.Cleanup(other.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(origin.Close)
	fetchSetting.AllowedPorts = []string{
		strconv.Itoa(portOf(t, origin.Listener.Addr().String())),
		strconv.Itoa(portOf(t, other.Listener.Addr().String())),
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", strings.NewReader(`{"prompt":"cat"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(string(constant.ContextKeyChannelKey), "mj-secret-value")

	_, _, err := DoMidjourneyHttpRequest(ctx, time.Second, origin.URL+"/mj/submit/imagine")
	require.Error(t, err)
	require.Zero(t, otherHits)
	require.Empty(t, stolen)

	permanentHits := 0
	permanent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		permanentHits++
		stolen = r.Header.Get("mj-api-secret")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(permanent.Close)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, permanent.URL+"/steal", http.StatusPermanentRedirect)
	}))
	t.Cleanup(source.Close)
	fetchSetting.AllowedPorts = append(fetchSetting.AllowedPorts,
		strconv.Itoa(portOf(t, source.Listener.Addr().String())),
		strconv.Itoa(portOf(t, permanent.Listener.Addr().String())),
	)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", strings.NewReader(`{"prompt":"cat"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	_, _, err = DoMidjourneyHttpRequest(ctx, time.Second, source.URL+"/mj/submit/imagine")
	require.Error(t, err)
	require.Zero(t, permanentHits)
	require.NotContains(t, stolen, "mj-secret-value")
}

func portOf(t *testing.T, addr string) int {
	t.Helper()
	_, portText, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	return port
}
