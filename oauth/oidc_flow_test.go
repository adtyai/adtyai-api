package oauth

import (
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestOIDCAuthorizationURLKeepsTheVerifierOnTheServer(t *testing.T) {
	settings := system_setting.GetOIDCSettings()
	previous := *settings
	previousAddress := system_setting.ServerAddress
	t.Cleanup(func() {
		*settings = previous
		system_setting.ServerAddress = previousAddress
	})
	*settings = system_setting.OIDCSettings{
		Enabled:               true,
		ClientId:              "oidc-client",
		ClientSecret:          "oidc-secret",
		WellKnown:             "https://example.com/.well-known/openid-configuration",
		AuthorizationEndpoint: "https://example.com/authorize",
		TokenEndpoint:         "https://example.com/token",
		UserInfoEndpoint:      "https://example.com/userinfo",
	}
	system_setting.ServerAddress = "https://app.example"
	flow, err := NewOIDCOAuthFlow()
	require.NoError(t, err)
	raw, err := flow.AuthorizationURL("state-1")
	require.NoError(t, err)
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "example.com", parsed.Host)
	assert.Equal(t, "/authorize", parsed.Path)
	assert.Equal(t, oauth2.S256ChallengeFromVerifier(flow.CodeVerifier), parsed.Query().Get("code_challenge"))
	assert.Equal(t, "S256", parsed.Query().Get("code_challenge_method"))
	assert.Equal(t, flow.Nonce, parsed.Query().Get("nonce"))
	assert.Equal(t, "state-1", parsed.Query().Get("state"))
	assert.Equal(t, "https://app.example/oauth/oidc", parsed.Query().Get("redirect_uri"))
	assert.Empty(t, parsed.Query().Get("code_verifier"))
	assert.NotContains(t, raw, flow.CodeVerifier)
	assert.NotContains(t, raw, "oidc-secret")
}
