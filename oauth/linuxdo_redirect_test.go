package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinuxDOExchangeUsesCanonicalFrontendCallback(t *testing.T) {
	previous := system_setting.ServerAddress
	system_setting.ServerAddress = "https://main.example/"
	t.Cleanup(func() { system_setting.ServerAddress = previous })
	var redirectURI string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.NoError(t, request.ParseForm())
		redirectURI = request.Form.Get("redirect_uri")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-access"}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("LINUX_DO_TOKEN_ENDPOINT", server.URL)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "http://shop.example/api/oauth/linuxdo", nil)
	token, err := (&LinuxDOProvider{}).ExchangeToken(context.Background(), "test-code", c)
	require.NoError(t, err)
	assert.Equal(t, "test-access", token.AccessToken)
	assert.Equal(t, "https://main.example/oauth/linuxdo", redirectURI)
}
