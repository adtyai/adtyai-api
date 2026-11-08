package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

const oidcTestIssuer = "https://example.com"

type oidcRewriteTransport struct{ target *url.URL }

func (transport oidcRewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != "example.com" {
		return nil, fmt.Errorf("unexpected OIDC host: %s", request.URL.Host)
	}
	clone := request.Clone(request.Context())
	clone.URL.Scheme, clone.URL.Host = transport.target.Scheme, transport.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

type oidcTestGrant struct {
	challenge string
	nonce     string
}

type oidcOAuthFixture struct {
	user          *model.User
	key           *rsa.PrivateKey
	mutex         sync.Mutex
	grants        map[string]oidcTestGrant
	emailVerified any
	nonceOverride string
	subject       string
}

func setupOIDCOauthTest(t *testing.T) *oidcOAuthFixture {
	t.Helper()
	user, _ := setupSecurityEnrollmentTest(t)
	previous := *system_setting.GetOIDCSettings()
	previousAddress := system_setting.ServerAddress
	previousProvider := oauth.GetProvider("oidc")
	*system_setting.GetOIDCSettings() = system_setting.OIDCSettings{
		Enabled:               true,
		ClientId:              "oidc-client",
		ClientSecret:          "oidc-secret",
		WellKnown:             oidcTestIssuer + "/.well-known/openid-configuration",
		AuthorizationEndpoint: oidcTestIssuer + "/authorize",
		TokenEndpoint:         oidcTestIssuer + "/token",
		UserInfoEndpoint:      oidcTestIssuer + "/userinfo",
	}
	system_setting.ServerAddress = "https://app.example"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	fixture := &oidcOAuthFixture{
		user: user, key: key, grants: map[string]oidcTestGrant{},
		emailVerified: true, subject: "oidc-subject-1",
	}
	require.NoError(t, model.DB.Model(user).Update("oidc_id", fixture.subject).Error)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		fixture.serve(t, w, request)
	}))
	t.Cleanup(func() {
		server.Close()
		*system_setting.GetOIDCSettings() = previous
		system_setting.ServerAddress = previousAddress
		oauth.Register("oidc", previousProvider)
	})
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	oauth.Register("oidc", oauth.NewOIDCProvider(&http.Client{
		Transport: oidcRewriteTransport{target: target},
		Timeout:   5 * time.Second,
	}))
	return fixture
}

func (fixture *oidcOAuthFixture) serve(t *testing.T, w http.ResponseWriter, request *http.Request) {
	t.Helper()
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch request.URL.Path {
	case "/.well-known/openid-configuration":
		payload, err := common.Marshal(map[string]any{
			"issuer":                                oidcTestIssuer,
			"authorization_endpoint":                oidcTestIssuer + "/authorize",
			"token_endpoint":                        oidcTestIssuer + "/token",
			"userinfo_endpoint":                     oidcTestIssuer + "/userinfo",
			"jwks_uri":                              oidcTestIssuer + "/keys",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
		if assert.NoError(t, err) {
			_, _ = w.Write(payload)
		}
	case "/keys":
		payload, err := common.Marshal(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": "oidc-test-key", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(fixture.key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(fixture.key.E)).Bytes()),
		}}})
		if assert.NoError(t, err) {
			_, _ = w.Write(payload)
		}
	case "/token":
		if request.ParseForm() != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		code := request.Form.Get("code")
		grant, ok := fixture.grants[code]
		if !ok || request.Form.Get("grant_type") != "authorization_code" ||
			request.Form.Get("client_id") != "oidc-client" ||
			request.Form.Get("client_secret") != "oidc-secret" ||
			request.Form.Get("redirect_uri") != "https://app.example/oauth/oidc" ||
			oauth2.S256ChallengeFromVerifier(request.Form.Get("code_verifier")) != grant.challenge {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		delete(fixture.grants, code)
		nonce := grant.nonce
		if fixture.nonceOverride != "" {
			nonce = fixture.nonceOverride
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": oidcTestIssuer, "aud": "oidc-client", "sub": fixture.subject,
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
			"nonce": nonce, "email": "person@example.com", "email_verified": fixture.emailVerified,
		})
		token.Header["kid"] = "oidc-test-key"
		signed, err := token.SignedString(fixture.key)
		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		payload, err := common.Marshal(map[string]any{
			"access_token": "oidc-access", "id_token": signed, "token_type": "Bearer", "expires_in": 60,
		})
		if assert.NoError(t, err) {
			_, _ = w.Write(payload)
		}
	case "/userinfo":
		if request.Header.Get("Authorization") != "Bearer oidc-access" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		payload, err := common.Marshal(map[string]any{
			"sub": fixture.subject, "email": "person@example.com", "email_verified": fixture.emailVerified,
			"name": "OIDC Person", "preferred_username": "oidc-person",
		})
		if assert.NoError(t, err) {
			_, _ = w.Write(payload)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (fixture *oidcOAuthFixture) start(t *testing.T) (string, string) {
	t.Helper()
	body, err := common.Marshal(oauthStateRequest{Provider: "oidc", Intent: "login"})
	require.NoError(t, err)
	response := securityEnrollmentRequest("POST", "/api/oauth/state", string(body), "", service.AuthIdentity{}, withOAuthTestBrowser(GenerateOAuthCode))
	var envelope securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Success, response.Body.String())
	var data struct {
		State string `json:"flow_token"`
		URL   string `json:"authorization_url"`
	}
	require.NoError(t, common.Unmarshal(envelope.Data, &data))
	parsed, err := url.Parse(data.URL)
	require.NoError(t, err)
	assert.Equal(t, "S256", parsed.Query().Get("code_challenge_method"))
	assert.Equal(t, data.State, parsed.Query().Get("state"))
	assert.NotEmpty(t, parsed.Query().Get("nonce"))
	assert.Empty(t, parsed.Query().Get("code_verifier"))
	assert.NotContains(t, response.Body.String(), "oidc-secret")
	code := "code-" + data.State
	fixture.mutex.Lock()
	fixture.grants[code] = oidcTestGrant{challenge: parsed.Query().Get("code_challenge"), nonce: parsed.Query().Get("nonce")}
	fixture.mutex.Unlock()
	return data.State, code
}

func oidcCallback(state, code string) *httptest.ResponseRecorder {
	path := "/api/oauth/oidc?" + url.Values{"state": {state}, "code": {code}}.Encode()
	return securityEnrollmentRequest("GET", path, "", "", service.AuthIdentity{}, func(c *gin.Context) {
		c.Params = gin.Params{{Key: "provider", Value: "oidc"}}
		withOAuthTestBrowser(HandleOAuth)(c)
	})
}

func TestOIDCLoginVerifiesPKCENonceAndEmail(t *testing.T) {
	fixture := setupOIDCOauthTest(t)
	state, code := fixture.start(t)
	response := oidcCallback(state, code)
	var body securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success, response.Body.String())
	var login struct {
		User struct {
			ID int `json:"id"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(body.Data, &login))
	assert.Equal(t, fixture.user.Id, login.User.ID)

	replay := oidcCallback(state, code)
	assert.Contains(t, replay.Body.String(), "State parameter is empty or mismatched")

	fixture.emailVerified = false
	badState, badCode := fixture.start(t)
	rejected := oidcCallback(badState, badCode)
	assert.Contains(t, rejected.Body.String(), "OIDC email is not verified")

	fixture.emailVerified = true
	fixture.nonceOverride = "replaced-nonce"
	mismatchState, mismatchCode := fixture.start(t)
	mismatch := oidcCallback(mismatchState, mismatchCode)
	assert.Contains(t, mismatch.Body.String(), "OIDC login could not be verified")
}
