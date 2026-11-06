package oauth

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOIDCProvider_GetName(t *testing.T) {
	settings := system_setting.GetOIDCSettings()
	originalDisplayName := settings.DisplayName
	defer func() { settings.DisplayName = originalDisplayName }()

	p := &OIDCProvider{}

	settings.DisplayName = ""
	assert.Equal(t, "OIDC", p.GetName())

	settings.DisplayName = "  Acme SSO  "
	assert.Equal(t, "Acme SSO", p.GetName())
}

func TestRegisteredOIDCProviderUsesProtectedTransport(t *testing.T) {
	provider, ok := GetProvider("oidc").(*OIDCProvider)
	require.True(t, ok)
	require.Nil(t, provider.client)
}

func TestOIDCEmailVerifiedAcceptsOnlyExplicitTrue(t *testing.T) {
	assert.True(t, oidcEmailVerified(json.RawMessage(`true`)))
	assert.True(t, oidcEmailVerified(json.RawMessage(`"true"`)))
	assert.False(t, oidcEmailVerified(json.RawMessage(`false`)))
	assert.False(t, oidcEmailVerified(json.RawMessage(`"false"`)))
	assert.False(t, oidcEmailVerified(json.RawMessage(`"yes"`)))
	assert.False(t, oidcEmailVerified(nil))
}

func TestOIDCIdentityTokenIsRequiredWhenItCannotBeVerified(t *testing.T) {
	provider := NewOIDCProvider(nil)
	_, err := provider.verifyIdentity(context.Background(), &system_setting.OIDCSettings{}, "", "")
	require.Error(t, err)
	_, err = provider.verifyIdentity(context.Background(), &system_setting.OIDCSettings{WellKnown: "https://issuer.example/.well-known/openid-configuration"}, "", "nonce")
	require.Error(t, err)
	_, err = provider.verifyIdentity(context.Background(), &system_setting.OIDCSettings{WellKnown: "https://issuer.example/.well-known/openid-configuration", ClientId: "oidc-client"}, "not-a-token", "")
	require.Error(t, err)
}
