package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type canonicalOAuthTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (transport canonicalOAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme, clone.URL.Host = transport.target.Scheme, transport.target.Host
	return transport.base.RoundTrip(clone)
}

func TestOAuthExchangesUseCanonicalCallbackWithTrailingSlash(t *testing.T) {
	previousAddress, previousTransport := system_setting.ServerAddress, http.DefaultTransport
	system_setting.ServerAddress = "https://main.example///"
	t.Cleanup(func() { system_setting.ServerAddress, http.DefaultTransport = previousAddress, previousTransport })
	for _, name := range []string{"discord", "custom-provider"} {
		t.Run(name, func(t *testing.T) {
			var redirectURI string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				require.NoError(t, request.ParseForm())
				redirectURI = request.Form.Get("redirect_uri")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"test-access"}`))
			}))
			t.Cleanup(server.Close)
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			http.DefaultTransport = canonicalOAuthTransport{target: target, base: previousTransport}
			var provider Provider = &DiscordProvider{}
			if name != "discord" {
				provider = NewGenericOAuthProvider(&model.CustomOAuthProvider{Slug: name, TokenEndpoint: server.URL}).WithClient(server.Client())
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "http://shop.example/api/oauth/"+name, nil)
			token, err := provider.ExchangeToken(context.Background(), "test-code", c)
			require.NoError(t, err)
			assert.Equal(t, "test-access", token.AccessToken)
			assert.Equal(t, "https://main.example/oauth/"+name, redirectURI)
		})
	}
}
