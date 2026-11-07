package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

const OIDCOAuthFlowContextKey = "oidc_oauth_flow"

// ErrOIDCNotConfigured is returned when an enabled OIDC login cannot be started
// from the current server settings.
var ErrOIDCNotConfigured = errors.New("OIDC login is not configured or enabled. Please contact your administrator.")

var errOIDCEmailUnverified = errors.New("oidc email is not verified")

// OIDCOAuthFlow keeps the PKCE verifier and nonce on the server. Only
// AuthorizationURL is sent to the browser.
type OIDCOAuthFlow struct {
	CodeVerifier          string `json:"code_verifier"`
	Nonce                 string `json:"nonce"`
	ClientID              string `json:"client_id"`
	RedirectURI           string `json:"redirect_uri"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"user_info_endpoint"`
	WellKnown             string `json:"well_known"`
}

func init() {
	Register("oidc", &OIDCProvider{})
}

// OIDCProvider implements OAuth for OIDC. A non-nil client is only for tests
// that rewrite the provider host; production uses the SSRF-protected client.
type OIDCProvider struct {
	client *http.Client
}

// NewOIDCProvider returns a provider that uses client for token, discovery, and
// userinfo requests. A nil client keeps the protected outbound client.
func NewOIDCProvider(client *http.Client) *OIDCProvider {
	return &OIDCProvider{client: client}
}

type oidcOAuthResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type oidcUser struct {
	OpenID            string          `json:"sub"`
	Email             string          `json:"email"`
	EmailVerified     json.RawMessage `json:"email_verified"`
	Name              string          `json:"name"`
	PreferredUsername string          `json:"preferred_username"`
	Picture           string          `json:"picture"`
}

func NewOIDCOAuthFlow() (*OIDCOAuthFlow, error) {
	settings := system_setting.GetOIDCSettings()
	if settings == nil || !settings.Enabled || strings.TrimSpace(settings.ClientId) == "" {
		return nil, ErrOIDCNotConfigured
	}
	for _, endpoint := range []string{settings.AuthorizationEndpoint, settings.TokenEndpoint, settings.UserInfoEndpoint, settings.WellKnown} {
		if err := ValidateEndpointURL(endpoint); err != nil {
			return nil, ErrOIDCNotConfigured
		}
	}
	redirectURI := strings.TrimRight(system_setting.ServerAddress, "/") + "/oauth/oidc"
	parsed, err := url.Parse(redirectURI)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, ErrOIDCNotConfigured
	}
	return &OIDCOAuthFlow{
		CodeVerifier:          oauth2.GenerateVerifier(),
		Nonce:                 oauth2.GenerateVerifier(),
		ClientID:              strings.TrimSpace(settings.ClientId),
		RedirectURI:           redirectURI,
		AuthorizationEndpoint: strings.TrimSpace(settings.AuthorizationEndpoint),
		TokenEndpoint:         strings.TrimSpace(settings.TokenEndpoint),
		UserInfoEndpoint:      strings.TrimSpace(settings.UserInfoEndpoint),
		WellKnown:             strings.TrimSpace(settings.WellKnown),
	}, nil
}

func (flow *OIDCOAuthFlow) AuthorizationURL(state string) (string, error) {
	if flow == nil || flow.CodeVerifier == "" || flow.Nonce == "" || strings.TrimSpace(state) == "" {
		return "", ErrOIDCNotConfigured
	}
	parsed, err := url.Parse(flow.AuthorizationEndpoint)
	if err != nil || parsed.Host == "" {
		return "", ErrOIDCNotConfigured
	}
	query := parsed.Query()
	query.Set("client_id", flow.ClientID)
	query.Set("redirect_uri", flow.RedirectURI)
	query.Set("response_type", "code")
	query.Set("scope", "openid profile email")
	query.Set("state", state)
	query.Set("nonce", flow.Nonce)
	query.Set("code_challenge", oauth2.S256ChallengeFromVerifier(flow.CodeVerifier))
	query.Set("code_challenge_method", "S256")
	parsed.RawQuery = query.Encode()
	if parsed.Query().Get("code_verifier") != "" {
		return "", ErrOIDCNotConfigured
	}
	return parsed.String(), nil
}

func (flow *OIDCOAuthFlow) matches(settings *system_setting.OIDCSettings) bool {
	if flow == nil || settings == nil || flow.CodeVerifier == "" || flow.Nonce == "" {
		return false
	}
	redirectURI := strings.TrimRight(system_setting.ServerAddress, "/") + "/oauth/oidc"
	return flow.ClientID == strings.TrimSpace(settings.ClientId) &&
		flow.RedirectURI == redirectURI &&
		flow.AuthorizationEndpoint == strings.TrimSpace(settings.AuthorizationEndpoint) &&
		flow.TokenEndpoint == strings.TrimSpace(settings.TokenEndpoint) &&
		flow.UserInfoEndpoint == strings.TrimSpace(settings.UserInfoEndpoint) &&
		flow.WellKnown == strings.TrimSpace(settings.WellKnown)
}

func (p *OIDCProvider) GetName() string {
	return system_setting.GetOIDCSettings().GetEffectiveDisplayName()
}

func (p *OIDCProvider) IsEnabled() bool {
	return system_setting.GetOIDCSettings().Enabled
}

func (p *OIDCProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" || c == nil {
		return nil, NewOAuthError(common.NewMessage(msgInvalidCode))
	}
	settings := system_setting.GetOIDCSettings()
	value, _ := c.Get(OIDCOAuthFlowContextKey)
	flow, ok := value.(*OIDCOAuthFlow)
	if !ok || !flow.matches(settings) {
		return nil, NewOAuthError(common.NewMessage("OIDC login could not be verified"))
	}
	values := url.Values{}
	values.Set("client_id", flow.ClientID)
	values.Set("client_secret", settings.ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", flow.RedirectURI)
	values.Set("code_verifier", flow.CodeVerifier)

	logger.LogDebug(ctx, "[OAuth-OIDC] ExchangeToken: token_endpoint=%s, redirect_uri=%s", flow.TokenEndpoint, flow.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", flow.TokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := p.httpDo(ctx, req)
	if err != nil {
		logger.LogError(ctx, common.LogText("[OAuth-OIDC] ExchangeToken error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgConnectFailed, map[string]any{"provider": "OIDC"}), err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-OIDC] ExchangeToken response status: %d", res.StatusCode)

	var oidcResponse oidcOAuthResponse
	err = common.DecodeJson(res.Body, &oidcResponse)
	if err != nil {
		logger.LogError(ctx, common.LogText("[OAuth-OIDC] ExchangeToken decode error: %s", err.Error()))
		return nil, err
	}

	if oidcResponse.AccessToken == "" {
		logger.LogError(ctx, common.LogText("[OAuth-OIDC] ExchangeToken failed: empty access token"))
		return nil, NewOAuthError(common.NewMessage(msgTokenFailed, map[string]any{"provider": "OIDC"}))
	}
	identity, err := p.verifyIdentity(ctx, settings, oidcResponse.IDToken, flow.Nonce)
	if err != nil {
		logger.LogError(ctx, common.LogText("[OAuth-OIDC] ID token verification failed: %s", err.Error()))
		if errors.Is(err, errOIDCEmailUnverified) {
			return nil, NewOAuthError(common.NewMessage("OIDC email is not verified"))
		}
		return nil, NewOAuthError(common.NewMessage("OIDC login could not be verified"))
	}

	logger.LogDebug(ctx, "[OAuth-OIDC] ExchangeToken success: scope=%s", oidcResponse.Scope)

	return &OAuthToken{
		AccessToken:      oidcResponse.AccessToken,
		TokenType:        oidcResponse.TokenType,
		RefreshToken:     oidcResponse.RefreshToken,
		ExpiresIn:        oidcResponse.ExpiresIn,
		Scope:            oidcResponse.Scope,
		IDToken:          oidcResponse.IDToken,
		ClientID:         flow.ClientID,
		Subject:          identity.Subject,
		VerifiedEmail:    identity.Email,
		UserInfoEndpoint: flow.UserInfoEndpoint,
	}, nil
}

func (p *OIDCProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	if token == nil || token.AccessToken == "" || token.Subject == "" || token.UserInfoEndpoint == "" {
		return nil, NewOAuthError(common.NewMessage("OIDC login could not be verified"))
	}
	settings := system_setting.GetOIDCSettings()
	if token.UserInfoEndpoint != strings.TrimSpace(settings.UserInfoEndpoint) || token.ClientID != strings.TrimSpace(settings.ClientId) {
		return nil, NewOAuthError(common.NewMessage("OIDC login could not be verified"))
	}

	logger.LogDebug(ctx, "[OAuth-OIDC] GetUserInfo: userinfo_endpoint=%s", token.UserInfoEndpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", token.UserInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	res, err := p.httpDo(ctx, req)
	if err != nil {
		logger.LogError(ctx, common.LogText("[OAuth-OIDC] GetUserInfo error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgConnectFailed, map[string]any{"provider": "OIDC"}), err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-OIDC] GetUserInfo response status: %d", res.StatusCode)

	if res.StatusCode != http.StatusOK {
		logger.LogError(ctx, common.LogText("[OAuth-OIDC] GetUserInfo failed: status=%d", res.StatusCode))
		return nil, NewOAuthError(common.NewMessage(msgGetUserFailed))
	}

	var oidcUser oidcUser
	err = common.DecodeJson(res.Body, &oidcUser)
	if err != nil {
		logger.LogError(ctx, common.LogText("[OAuth-OIDC] GetUserInfo decode error: %s", err.Error()))
		return nil, err
	}

	if oidcUser.OpenID == "" || oidcUser.OpenID != token.Subject || oidcUser.Email == "" {
		logger.LogError(ctx, common.LogText("[OAuth-OIDC] GetUserInfo failed: empty or mismatched fields (sub=%s, email=%s)", oidcUser.OpenID, oidcUser.Email))
		return nil, NewOAuthError(common.NewMessage(msgUserInfoEmpty, map[string]any{"provider": "OIDC"}))
	}
	if !oidcEmailVerified(oidcUser.EmailVerified) {
		return nil, NewOAuthError(common.NewMessage("OIDC email is not verified"))
	}
	if token.VerifiedEmail != "" && !strings.EqualFold(token.VerifiedEmail, oidcUser.Email) {
		return nil, NewOAuthError(common.NewMessage("OIDC login could not be verified"))
	}

	logger.LogDebug(ctx, "[OAuth-OIDC] GetUserInfo success: sub=%s, username=%s, name=%s, email=%s", oidcUser.OpenID, oidcUser.PreferredUsername, oidcUser.Name, oidcUser.Email)

	return &OAuthUser{
		ProviderUserID: oidcUser.OpenID,
		Username:       oidcUser.PreferredUsername,
		DisplayName:    oidcUser.Name,
		Email:          oidcUser.Email,
	}, nil
}

type oidcVerifiedIdentity struct {
	Subject string
	Email   string
}

func (p *OIDCProvider) verifyIdentity(ctx context.Context, settings *system_setting.OIDCSettings, raw, nonce string) (*oidcVerifiedIdentity, error) {
	if settings == nil || strings.TrimSpace(settings.WellKnown) == "" || strings.TrimSpace(raw) == "" || strings.TrimSpace(nonce) == "" {
		return nil, errors.New("oidc id token is not verifiable")
	}
	issuer, err := oidcIssuer(settings.WellKnown)
	if err != nil {
		return nil, err
	}
	client := p.discoveryClient()
	ctx = oidc.ClientContext(ctx, client)
	if strings.HasPrefix(issuer, "http://") {
		ctx = oidc.InsecureIssuerURLContext(ctx, issuer)
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: settings.ClientId}).Verify(ctx, raw)
	if err != nil {
		return nil, err
	}
	if idToken.Subject == "" || idToken.Nonce != nonce {
		return nil, errors.New("oidc nonce mismatch")
	}
	var claims struct {
		Email         string          `json:"email"`
		EmailVerified json.RawMessage `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, err
	}
	email := strings.TrimSpace(claims.Email)
	if email != "" && !oidcEmailVerified(claims.EmailVerified) {
		return nil, errOIDCEmailUnverified
	}
	return &oidcVerifiedIdentity{Subject: idToken.Subject, Email: email}, nil
}

func (p *OIDCProvider) httpDo(ctx context.Context, req *http.Request) (*http.Response, error) {
	if p != nil && p.client != nil {
		return p.client.Do(req.WithContext(ctx))
	}
	return doOutbound(ctx, req)
}

func (p *OIDCProvider) discoveryClient() *http.Client {
	if p != nil && p.client != nil {
		return p.client
	}
	return outboundClient()
}

func oidcIssuer(wellKnown string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(wellKnown))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("oidc discovery url is invalid")
	}
	path := strings.TrimSuffix(parsed.EscapedPath(), "/")
	const suffix = "/.well-known/openid-configuration"
	if strings.HasSuffix(path, suffix) {
		parsed.Path = strings.TrimSuffix(path, suffix)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func oidcEmailVerified(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var flag bool
	if err := common.Unmarshal(raw, &flag); err == nil {
		return flag
	}
	var text string
	if err := common.Unmarshal(raw, &text); err == nil {
		return strings.EqualFold(strings.TrimSpace(text), "true")
	}
	return false
}

func (p *OIDCProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsOidcIdAlreadyTaken(providerUserID)
}

func (p *OIDCProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.OidcId = providerUserID
	return user.FillUserByOidcId()
}

func (p *OIDCProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.OidcId = providerUserID
}

func (p *OIDCProvider) GetProviderPrefix() string {
	return "oidc_"
}

// ProviderUserIDColumn returns the users-table column storing this provider's user ID.
func (p *OIDCProvider) ProviderUserIDColumn() string {
	return "oidc_id"
}
