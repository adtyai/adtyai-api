package relay_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/ali"
	"github.com/QuantumNous/new-api/relay/channel/aws"
	"github.com/QuantumNous/new-api/relay/channel/baidu"
	"github.com/QuantumNous/new-api/relay/channel/cloudflare"
	"github.com/QuantumNous/new-api/relay/channel/cohere"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/jina"
	"github.com/QuantumNous/new-api/relay/channel/mistral"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/QuantumNous/new-api/relay/channel/perplexity"
	"github.com/QuantumNous/new-api/relay/channel/siliconflow"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	"github.com/QuantumNous/new-api/relay/channel/zhipu"
	zhipu4v "github.com/QuantumNous/new-api/relay/channel/zhipu_4v"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultModelCatalogueLifecycle(t *testing.T) {
	tests := []struct {
		name      string
		models    []string
		removed   []string
		available []string
	}{
		{"openai", (&openai.Adaptor{}).GetModelList(), []string{"o3-mini", "o3-mini-high", "dall-e-2"}, []string{"gpt-6.1-sol", "gpt-image-2.6-flare", "gpt-4.1"}},
		{"gemini", (&gemini.Adaptor{}).GetModelList(), []string{"gemini-2.0-flash", "gemini-3-pro-preview"}, []string{"gemini-3.8-flash", "gemini-embedding-2", "gemini-2.5-pro"}},
		{"cohere", (&cohere.Adaptor{}).GetModelList(), []string{"rerank-english-v2.0", "rerank-multilingual-v2.0"}, []string{"rerank-v4.0-pro", "embed-v4.0"}},
		{"mistral", (&mistral.Adaptor{}).GetModelList(), []string{"open-mistral-7b"}, []string{"mistral-large-4", "mistral-embed-2312"}},
		{"ali", (&ali.Adaptor{}).GetModelList(), nil, []string{"qwen3.8-max", "qwen3.6-plus", "text-embedding-v4"}},
		{"aws", (&aws.Adaptor{}).GetModelList(), nil, []string{"claude-3-haiku-20240307", "anthropic.claude-sonnet-5-5"}},
		{"perplexity", (&perplexity.Adaptor{}).GetModelList(), nil, []string{"sonar", "openai/gpt-6.1-sol", "perplexity/sonar"}},
		{"vertex", (&vertex.Adaptor{}).GetModelList(), []string{"claude-mythos-5", "claude-mythos-5-1"}, []string{"gemini-2.0-flash", "gemini-embedding-001"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEmpty(t, tt.models)
			for _, model := range tt.removed {
				assert.NotContains(t, tt.models, model)
			}
			for _, model := range tt.available {
				assert.Contains(t, tt.models, model)
			}
		})
	}
}

type contractAdaptor interface {
	GetRequestURL(*relaycommon.RelayInfo) (string, error)
	SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error
	DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error)
	DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError)
}

type contractTransport struct {
	base      http.RoundTripper
	fromHost  string
	targetURL string
}

func (t contractTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != t.fromHost {
		return t.base.RoundTrip(req)
	}
	target, err := url.Parse(t.targetURL)
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	copyURL := *req.URL
	copyURL.Scheme, copyURL.Host = target.Scheme, target.Host
	clone.URL = &copyURL
	clone.Host = ""
	return t.base.RoundTrip(clone)
}

func contractContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept", "application/json")
	return c, w
}

func contractInfo(baseURL, apiKey, model string, mode int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    baseURL,
			ApiKey:            apiKey,
			ApiVersion:        "account-contract",
			UpstreamModelName: model,
			ChannelSetting:    dto.ChannelSettings{},
		},
		RelayMode:      mode,
		RequestURLPath: "/v1/chat/completions",
		RelayFormat:    types.RelayFormatOpenAI,
		StartTime:      time.Now(),
	}
}

func runContractRoundTrip(t *testing.T, adaptor contractAdaptor, c *gin.Context, info *relaycommon.RelayInfo, converted any) (any, *types.NewAPIError) {
	t.Helper()
	body, err := common.Marshal(converted)
	if err != nil {
		t.Fatalf("marshal converted request: %v", err)
	}
	responseValue, err := adaptor.DoRequest(c, info, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("DoRequest: %v", err)
	}
	response, ok := responseValue.(*http.Response)
	if !ok || response == nil {
		t.Fatalf("DoRequest returned %T, want *http.Response", responseValue)
	}
	return adaptor.DoResponse(c, response, info)
}

func assertMalformedResponse(t *testing.T, adaptor contractAdaptor, info *relaycommon.RelayInfo) {
	t.Helper()
	c, _ := contractContext()
	_, apiErr := adaptor.DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("{")),
	}, info)
	if apiErr == nil {
		t.Fatal("DoResponse accepted malformed upstream JSON")
	}
}

func TestNativeHTTPAdaptorContracts(t *testing.T) {
	service.InitHttpClient()
	t.Run("baidu", testBaiduContract)
	t.Run("cohere", testCohereContract)
	t.Run("jina", testJinaContract)
	t.Run("cloudflare", testCloudflareContract)
	t.Run("mistral", testMistralContract)
	t.Run("zhipu-v3", testZhipuContract)
	t.Run("zhipu-v4", testZhipu4vContract)
	t.Run("siliconflow", testSiliconFlowContract)
}

func testBaiduContract(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/2.0/token" {
			t.Errorf("OAuth request = %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("grant_type") != "client_credentials" || r.URL.Query().Get("client_id") != "contract-client" || r.URL.Query().Get("client_secret") != "contract-secret" {
			t.Errorf("unexpected OAuth query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"local-access-token","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	client := service.GetHttpClient()
	if client == nil {
		t.Fatal("service HTTP client was not initialized")
	}
	previousTransport := client.Transport
	baseTransport := previousTransport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	client.Transport = contractTransport{base: baseTransport, fromHost: "aip.baidubce.com", targetURL: tokenServer.URL}
	defer func() { client.Transport = previousTransport }()

	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc/2.0/ai_custom/v1/wenxinworkshop/chat/completions" || r.URL.Query().Get("access_token") != "local-access-token" {
			t.Errorf("Baidu URL = %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer contract-client|contract-secret" {
			t.Errorf("Authorization = %q", got)
		}
		if err := common.DecodeJson(r.Body, &receivedBody); err != nil {
			t.Errorf("decode Baidu body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"b1","created":1,"result":"answer","usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`)
	}))
	defer server.Close()
	apiKey := fmt.Sprintf("contract-client|contract-secret-%d", time.Now().UnixNano())
	parts := strings.SplitN(apiKey, "|", 2)
	apiKey = parts[0] + "|contract-secret"
	info := contractInfo(server.URL, apiKey, "ERNIE-Bot", relayconstant.RelayModeChatCompletions)
	info.ChannelMeta.ChannelSetting = dto.ChannelSettings{}
	maxTokens := uint(1)
	c, out := contractContext()
	converted, err := (&baidu.Adaptor{}).ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{
		Model: "ERNIE-Bot", MaxTokens: &maxTokens,
		Messages: []dto.Message{{Role: "system", Content: "rules"}, {Role: "user", Content: "question"}},
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest: %v", err)
	}
	usage, apiErr := runContractRoundTrip(t, &baidu.Adaptor{}, c, info, converted)
	if apiErr != nil {
		t.Fatalf("DoResponse: %v", apiErr)
	}
	gotUsage, ok := usage.(*dto.Usage)
	if !ok || gotUsage.PromptTokens != 11 || gotUsage.CompletionTokens != 5 || gotUsage.TotalTokens != 16 {
		t.Fatalf("usage = %#v", usage)
	}
	if receivedBody["system"] != "rules" || receivedBody["max_output_tokens"] != float64(2) {
		t.Fatalf("Baidu request mapping = %#v", receivedBody)
	}
	messages, _ := receivedBody["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["content"] != "question" {
		t.Fatalf("Baidu messages = %#v", messages)
	}
	if !strings.Contains(out.Body.String(), "answer") {
		t.Fatalf("normalized response = %s", out.Body.String())
	}
	assertMalformedResponse(t, &baidu.Adaptor{}, info)
}

func testCohereContract(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat" {
			t.Errorf("Cohere URL = %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer cohere-secret" {
			t.Errorf("Cohere Authorization = %q", r.Header.Get("Authorization"))
		}
		_ = common.DecodeJson(r.Body, &received)
		_, _ = io.WriteString(w, `{"response_id":"c1","text":"answer","finish_reason":"COMPLETE","meta":{"billed_units":{"input_tokens":7,"output_tokens":3}}}`)
	}))
	defer server.Close()
	info := contractInfo(server.URL, "cohere-secret", "command-r", relayconstant.RelayModeChatCompletions)
	c, out := contractContext()
	converted, err := (&cohere.Adaptor{}).ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: "command-r", Messages: []dto.Message{
		{Role: "system", Content: "rules"}, {Role: "assistant", Content: "prior"}, {Role: "user", Content: "question"},
	}})
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest: %v", err)
	}
	usage, apiErr := runContractRoundTrip(t, &cohere.Adaptor{}, c, info, converted)
	if apiErr != nil {
		t.Fatalf("DoResponse: %v", apiErr)
	}
	gotUsage, ok := usage.(*dto.Usage)
	if !ok || gotUsage.PromptTokens != 7 || gotUsage.CompletionTokens != 3 || gotUsage.TotalTokens != 10 {
		t.Fatalf("usage = %#v", usage)
	}
	if received["message"] != "question" || received["max_tokens"] != float64(4000) {
		t.Fatalf("Cohere request = %#v", received)
	}
	history, _ := received["chat_history"].([]any)
	if len(history) != 2 || history[0].(map[string]any)["role"] != "SYSTEM" || history[1].(map[string]any)["role"] != "CHATBOT" {
		t.Fatalf("Cohere history = %#v", history)
	}
	if !strings.Contains(out.Body.String(), `"finish_reason":"stop"`) {
		t.Fatalf("normalized response = %s", out.Body.String())
	}
	assertMalformedResponse(t, &cohere.Adaptor{}, info)
}

func testJinaContract(t *testing.T) {
	var received dto.RerankRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" || r.Header.Get("Authorization") != "Bearer jina-secret" {
			t.Errorf("Jina request target/auth = %s %q", r.URL.String(), r.Header.Get("Authorization"))
		}
		_ = common.DecodeJson(r.Body, &received)
		_, _ = io.WriteString(w, `{"results":[{"index":1,"relevance_score":0.91}],"usage":{"prompt_tokens":12,"completion_tokens":2,"total_tokens":14}}`)
	}))
	defer server.Close()
	info := contractInfo(server.URL, "jina-secret", "jina-reranker-v2-base-multilingual", relayconstant.RelayModeRerank)
	c, out := contractContext()
	topN := 1
	converted, err := (&jina.Adaptor{}).ConvertRerankRequest(c, info.RelayMode, dto.RerankRequest{Model: info.UpstreamModelName, Query: "q", Documents: []any{"one", "two"}, TopN: &topN})
	if err != nil {
		t.Fatalf("ConvertRerankRequest: %v", err)
	}
	usage, apiErr := runContractRoundTrip(t, &jina.Adaptor{}, c, info, converted)
	if apiErr != nil {
		t.Fatalf("DoResponse: %v", apiErr)
	}
	gotUsage, ok := usage.(*dto.Usage)
	if !ok || gotUsage.PromptTokens != 14 || gotUsage.TotalTokens != 14 {
		t.Fatalf("usage = %#v", usage)
	}
	if received.Query != "q" || len(received.Documents) != 2 || received.TopN == nil || *received.TopN != 1 {
		t.Fatalf("Jina request = %#v", received)
	}
	if !strings.Contains(out.Body.String(), `"index":1`) {
		t.Fatalf("normalized response = %s", out.Body.String())
	}
	assertMalformedResponse(t, &jina.Adaptor{}, info)
}

func testCloudflareContract(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/v4/accounts/account-contract/ai/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer cf-secret" {
			t.Errorf("Cloudflare request target/auth = %s %q", r.URL.String(), r.Header.Get("Authorization"))
		}
		_ = common.DecodeJson(r.Body, &received)
		_, _ = io.WriteString(w, `{"id":"cf1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":99,"completion_tokens":99,"total_tokens":198}}`)
	}))
	defer server.Close()
	info := contractInfo(server.URL, "cf-secret", "@cf/test-model", relayconstant.RelayModeChatCompletions)
	info.SetEstimatePromptTokens(9)
	c, out := contractContext()
	converted, err := (&cloudflare.Adaptor{}).ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: "@cf/test-model", Messages: []dto.Message{{Role: "user", Content: "question"}}})
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest: %v", err)
	}
	usage, apiErr := runContractRoundTrip(t, &cloudflare.Adaptor{}, c, info, converted)
	if apiErr != nil {
		t.Fatalf("DoResponse: %v", apiErr)
	}
	gotUsage, ok := usage.(*dto.Usage)
	if !ok || gotUsage.PromptTokens != 99 || gotUsage.CompletionTokens != 99 || gotUsage.TotalTokens != 198 {
		t.Fatalf("Cloudflare upstream usage = %#v", usage)
	}
	if _, ok := received["messages"].([]any); !ok {
		t.Fatalf("Cloudflare chat request = %#v", received)
	}
	if !strings.Contains(out.Body.String(), "answer") || !strings.Contains(out.Body.String(), `"total_tokens":198`) {
		t.Fatalf("normalized response did not preserve provider usage: %s", out.Body.String())
	}
	assertMalformedResponse(t, &cloudflare.Adaptor{}, info)
}

func testMistralContract(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer mistral-secret" {
			t.Errorf("Mistral request target/auth = %s %q", r.URL.String(), r.Header.Get("Authorization"))
		}
		_ = common.DecodeJson(r.Body, &received)
		_, _ = io.WriteString(w, `{"id":"m1","object":"chat.completion","model":"mistral-small","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`)
	}))
	defer server.Close()
	info := contractInfo(server.URL, "mistral-secret", "mistral-small", relayconstant.RelayModeChatCompletions)
	c, out := contractContext()
	maxTokens := uint(40)
	toolCalls := json.RawMessage(`[{"id":"call-invalid-id","type":"function","function":{"name":"lookup","arguments":"{}"}}]`)
	request := &dto.GeneralOpenAIRequest{Model: "mistral-small", MaxTokens: &maxTokens, Messages: []dto.Message{
		{Role: "assistant", Content: "", ToolCalls: toolCalls},
		{Role: "tool", Content: "done", ToolCallId: "call-invalid-id"},
	}}
	converted, err := (&mistral.Adaptor{}).ConvertOpenAIRequest(c, info, request)
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest: %v", err)
	}
	usage, apiErr := runContractRoundTrip(t, &mistral.Adaptor{}, c, info, converted)
	if apiErr != nil {
		t.Fatalf("DoResponse: %v", apiErr)
	}
	gotUsage, ok := usage.(*dto.Usage)
	if !ok || gotUsage.PromptTokens != 4 || gotUsage.CompletionTokens != 2 || gotUsage.TotalTokens != 6 {
		t.Fatalf("usage = %#v", usage)
	}
	messages, _ := received["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("Mistral messages = %#v", messages)
	}
	first, second := messages[0].(map[string]any), messages[1].(map[string]any)
	calls, _ := first["tool_calls"].([]any)
	call := calls[0].(map[string]any)
	callID := call["id"].(string)
	if len(callID) != 9 || second["tool_call_id"] != callID || received["max_tokens"] != float64(40) {
		t.Fatalf("Mistral call ID mapping / request = %#v", received)
	}
	if !strings.Contains(out.Body.String(), "answer") {
		t.Fatalf("normalized response = %s", out.Body.String())
	}
	assertMalformedResponse(t, &mistral.Adaptor{}, info)
}

func verifyZhipuJWT(t *testing.T, token string) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("Zhipu Authorization is not a compact JWT: %q", token)
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var header map[string]any
	if err = common.Unmarshal(headerBytes, &header); err != nil {
		t.Fatal(err)
	}
	if header["alg"] != "HS256" || header["sign_type"] != "SIGN" {
		t.Fatalf("Zhipu JWT header = %#v", header)
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err = common.Unmarshal(claimsBytes, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["api_key"] != "contract-zhipu-id" || claims["timestamp"] == nil || claims["exp"].(float64) < claims["timestamp"].(float64) {
		t.Fatalf("Zhipu JWT claims = %#v", claims)
	}
	mac := hmac.New(sha256.New, []byte("contract-zhipu-secret"))
	_, _ = io.WriteString(mac, parts[0]+"."+parts[1])
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(want)) {
		t.Fatal("Zhipu JWT signature did not verify")
	}
}

func testZhipuContract(t *testing.T) {
	var auth string
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/paas/v3/model-api/chatglm_std/invoke" {
			t.Errorf("Zhipu URL = %s", r.URL.String())
		}
		auth = r.Header.Get("Authorization")
		_ = common.DecodeJson(r.Body, &received)
		_, _ = io.WriteString(w, `{"success":true,"data":{"task_id":"z1","choices":[{"role":"assistant","content":"answer"}],"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}}}`)
	}))
	defer server.Close()
	info := contractInfo(server.URL, "contract-zhipu-id.contract-zhipu-secret", "chatglm_std", relayconstant.RelayModeChatCompletions)
	c, out := contractContext()
	topP := 1.0
	converted, err := (&zhipu.Adaptor{}).ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: "chatglm_std", TopP: &topP, Messages: []dto.Message{
		{Role: "system", Content: "rules"}, {Role: "user", Content: "question"},
	}})
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest: %v", err)
	}
	usage, apiErr := runContractRoundTrip(t, &zhipu.Adaptor{}, c, info, converted)
	if apiErr != nil {
		t.Fatalf("DoResponse: %v", apiErr)
	}
	verifyZhipuJWT(t, auth)
	gotUsage, ok := usage.(*dto.Usage)
	if !ok || gotUsage.PromptTokens != 8 || gotUsage.CompletionTokens != 4 || gotUsage.TotalTokens != 12 {
		t.Fatalf("usage = %#v", usage)
	}
	if (received["incremental"] != nil && received["incremental"] != false) || received["top_p"] != 0.99 {
		t.Fatalf("Zhipu request = %#v", received)
	}
	prompt, _ := received["prompt"].([]any)
	if len(prompt) != 3 || prompt[1].(map[string]any)["content"] != "Okay" {
		t.Fatalf("Zhipu prompt mapping = %#v", prompt)
	}
	if !strings.Contains(out.Body.String(), "answer") {
		t.Fatalf("normalized response = %s", out.Body.String())
	}
	assertMalformedResponse(t, &zhipu.Adaptor{}, info)
}

func testZhipu4vContract(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/paas/v4/chat/completions" || r.Header.Get("Authorization") != "Bearer zhipu4v-secret" {
			t.Errorf("Zhipu4v request target/auth = %s %q", r.URL.String(), r.Header.Get("Authorization"))
		}
		_ = common.DecodeJson(r.Body, &received)
		_, _ = io.WriteString(w, `{"id":"z4","object":"chat.completion","model":"glm-4","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
	}))
	defer server.Close()
	info := contractInfo(server.URL, "zhipu4v-secret", "glm-4", relayconstant.RelayModeChatCompletions)
	c, out := contractContext()
	topP := 1.0
	converted, err := (&zhipu4v.Adaptor{}).ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{
		Model: "glm-4", TopP: &topP, Stop: "END", Messages: []dto.Message{{Role: "user", Content: "question"}},
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest: %v", err)
	}
	usage, apiErr := runContractRoundTrip(t, &zhipu4v.Adaptor{}, c, info, converted)
	if apiErr != nil {
		t.Fatalf("DoResponse: %v", apiErr)
	}
	gotUsage, ok := usage.(*dto.Usage)
	if !ok || gotUsage.PromptTokens != 3 || gotUsage.CompletionTokens != 2 || gotUsage.TotalTokens != 5 {
		t.Fatalf("usage = %#v", usage)
	}
	if received["top_p"] != 0.99 || received["stop"].([]any)[0] != "END" {
		t.Fatalf("Zhipu4v request = %#v", received)
	}
	if !strings.Contains(out.Body.String(), "answer") {
		t.Fatalf("normalized response = %s", out.Body.String())
	}
	assertMalformedResponse(t, &zhipu4v.Adaptor{}, info)
}

func testSiliconFlowContract(t *testing.T) {
	var received dto.RerankRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" || r.Header.Get("Authorization") != "Bearer sf-secret" {
			t.Errorf("SiliconFlow request target/auth = %s %q", r.URL.String(), r.Header.Get("Authorization"))
		}
		_ = common.DecodeJson(r.Body, &received)
		_, _ = io.WriteString(w, `{"results":[{"index":0,"relevance_score":0.88}],"meta":{"tokens":{"input_tokens":6,"output_tokens":2}}}`)
	}))
	defer server.Close()
	info := contractInfo(server.URL, "sf-secret", "BAAI/bge-reranker-v2-m3", relayconstant.RelayModeRerank)
	info.RequestURLPath = "/v1/rerank"
	c, out := contractContext()
	converted, err := (&siliconflow.Adaptor{}).ConvertRerankRequest(c, info.RelayMode, dto.RerankRequest{Model: info.UpstreamModelName, Query: "q", Documents: []any{"doc"}})
	if err != nil {
		t.Fatalf("ConvertRerankRequest: %v", err)
	}
	usage, apiErr := runContractRoundTrip(t, &siliconflow.Adaptor{}, c, info, converted)
	if apiErr != nil {
		t.Fatalf("DoResponse: %v", apiErr)
	}
	gotUsage, ok := usage.(*dto.Usage)
	if !ok || gotUsage.PromptTokens != 6 || gotUsage.CompletionTokens != 2 || gotUsage.TotalTokens != 8 {
		t.Fatalf("usage = %#v", usage)
	}
	if received.Query != "q" || len(received.Documents) != 1 || !strings.Contains(out.Body.String(), `"total_tokens":8`) {
		t.Fatalf("SiliconFlow request/response = %#v / %s", received, out.Body.String())
	}
	assertMalformedResponse(t, &siliconflow.Adaptor{}, info)
}
