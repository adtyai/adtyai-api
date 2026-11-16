package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adtyTestRewriteTransport 把翻译请求指到本地测试服务器，同时不改动被测代码里的目标域名。
type adtyTestRewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t adtyTestRewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

func adtyTestTranslationClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := server.Client()
	client.Transport = adtyTestRewriteTransport{target: target, base: client.Transport}
	return client
}

// ISS-R4-0012 的门禁：默认的 AdtyTranslateFunc 必须真的按 Root 配置的地址和密钥发 HTTP，
// 并把译文按 OpenAI 协议解出来。改回空实现时这条会在 require.NoError 和首条断言上红。
func TestAdtyTranslationDefaultFuncDialsRootConfiguredProvider(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "synthetic-encryption-key-for-local-tests-only")
	db := prepareReferralDialectDB(t, "sqlite")

	var gotPath, gotAuth, gotModel, gotSource string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		gotModel = body.Model
		gotSource = body.Messages[len(body.Messages)-1].Content
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Amount 12.50 USD order A123456 is not refunded"}}]}`))
	}))
	defer server.Close()

	var timeout time.Duration
	oldClient := adtyTranslationHTTPClient
	adtyTranslationHTTPClient = func(seconds time.Duration) *http.Client {
		timeout = seconds
		return adtyTestTranslationClient(t, server)
	}
	t.Cleanup(func() { adtyTranslationHTTPClient = oldClient })

	sealed, err := model.SealAdtyPurposeSecret("translation", "translate-secret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.AdtyTranslationConfig{
		Id: 1, Mode: model.AdtyTranslationModeOwn, Protocol: "openai",
		BaseURL: "https://translator.example/v1", ModelName: "translator",
		TokenCipher: sealed, TokenMask: "****", TimeoutSec: 7, Enabled: true,
	}).Error)

	english, err := AdtyTranslateFunc("translator", "金额 12.50 USD，订单 A123456，不退款")
	require.NoError(t, err)
	assert.Equal(t, "Amount 12.50 USD order A123456 is not refunded", english)
	assert.Equal(t, "/v1/chat/completions", gotPath)
	assert.Equal(t, "Bearer translate-secret", gotAuth)
	assert.Equal(t, "translator", gotModel)
	assert.Contains(t, gotSource, "A123456")
	assert.Equal(t, 7*time.Second, timeout)
}

// 外部服务失败时保留中文并标记英文未就绪，且不扣客户余额。
func TestAdtyTranslationProviderFailureKeepsChineseAndNoDebit(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "synthetic-encryption-key-for-local-tests-only")
	db := prepareReferralDialectDB(t, "sqlite")
	user := referralUser(t, db, "tr-provider-user")
	require.NoError(t, db.Model(user).Update("quota", 500000).Error)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream down"}`))
	}))
	defer server.Close()

	oldClient := adtyTranslationHTTPClient
	adtyTranslationHTTPClient = func(time.Duration) *http.Client {
		return adtyTestTranslationClient(t, server)
	}
	t.Cleanup(func() { adtyTranslationHTTPClient = oldClient })

	require.NoError(t, db.Create(&model.AdtyTranslationConfig{
		Id: 1, Mode: model.AdtyTranslationModeSet, Protocol: "openai",
		BaseURL: "https://translator.example/v1", ModelName: "translator",
		TimeoutSec: 5, Enabled: true,
	}).Error)

	source := "金额 12.50 USD，订单 A123456，不退款"
	result, err := TranslateAdtyDynamic("notice:provider-down", source, 0)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTranslationFailed, result.Status)
	assert.Equal(t, AdtyTranslationNotReady, result.Notice)
	assert.Empty(t, result.English)
	assert.Equal(t, source, result.Source)

	var head model.AdtyTranslationHead
	require.NoError(t, db.Where("object_key = ?", "notice:provider-down").First(&head).Error)
	assert.Equal(t, source, head.Source)
	assert.Empty(t, head.English)

}

// 配置指回本站（品牌域名、主站域名或分站根域名）时直接拒绝，不发请求、不递归。
func TestAdtyTranslationProviderRefusesOwnRelay(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "synthetic-encryption-key-for-local-tests-only")
	db := prepareReferralDialectDB(t, "sqlite")

	dialed := false
	oldClient := adtyTranslationHTTPClient
	adtyTranslationHTTPClient = func(time.Duration) *http.Client {
		dialed = true
		return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, http.ErrHandlerTimeout
		})}
	}
	t.Cleanup(func() { adtyTranslationHTTPClient = oldClient })

	require.NoError(t, db.Create(&model.AdtyTranslationConfig{
		Id: 1, Mode: model.AdtyTranslationModeSet, Protocol: "openai",
		BaseURL: "https://adtyai.com/v1", ModelName: "translator", Enabled: true,
	}).Error)

	_, err := AdtyTranslateFunc("translator", "金额 12.50 USD，订单 A123456，不退款")
	require.ErrorIs(t, err, ErrAdtyTranslationSelf)
	assert.False(t, dialed, "self translation target must be refused before any HTTP call")
}

func TestAdtyTranslationEndpointShapes(t *testing.T) {
	cases := []struct {
		name     string
		baseURL  string
		protocol string
		want     string
	}{
		{"openai root", "https://api.example.com", "openai", "https://api.example.com/v1/chat/completions"},
		{"openai v1", "https://api.example.com/v1/", "", "https://api.example.com/v1/chat/completions"},
		{"openai full path", "https://api.example.com/v1/chat/completions", "openai", "https://api.example.com/v1/chat/completions"},
		{"anthropic root", "https://api.anthropic.com", "anthropic", "https://api.anthropic.com/v1/messages"},
		{"anthropic v1", "https://api.anthropic.com/v1", "claude", "https://api.anthropic.com/v1/messages"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			protocol, err := adtyTranslationProtocolName(tc.protocol)
			require.NoError(t, err)
			got, err := adtyTranslationEndpoint(tc.baseURL, protocol)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	protocol, err := adtyTranslationProtocolName("openai")
	require.NoError(t, err)
	_, err = adtyTranslationEndpoint("http://api.example.com/v1", protocol)
	require.ErrorIs(t, err, ErrAdtyTranslationInvalid)
	_, err = adtyTranslationEndpoint("https://localhost:8443/v1", protocol)
	require.ErrorIs(t, err, ErrAdtyTranslationSelf)
	_, err = adtyTranslationProtocolName("grpc")
	require.ErrorIs(t, err, ErrAdtyTranslationInvalid)
}
