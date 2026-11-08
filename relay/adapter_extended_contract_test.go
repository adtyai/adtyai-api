package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type extendedAdaptorContract struct {
	name         string
	apiType      int
	concreteType string
	mode         int
	format       types.RelayFormat
	stream       bool
	path         string
	apiKey       string
	model        string
	convert      func(*gin.Context, *relaycommon.RelayInfo, interfaceAdaptor) (any, error)
	responseBody string
	checkRequest func(*testing.T, *http.Request, []byte)
	checkOutput  func(*testing.T, string)
	checkUsage   func(*testing.T, any)
}

type interfaceAdaptor interface {
	ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error)
	ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error)
}

func TestExtendedAdaptorDispatchedHTTPContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	chatResponse := `{"id":"resp_1","object":"chat.completion","created":1,"model":"upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`
	checkChatUsage := func(t *testing.T, usage any) {
		got, ok := usage.(*dto.Usage)
		require.True(t, ok)
		assert.Equal(t, 2, got.PromptTokens)
		assert.Equal(t, 3, got.CompletionTokens)
		assert.Equal(t, 5, got.TotalTokens)
	}
	contracts := []extendedAdaptorContract{
		{
			name: "baidu_v2", apiType: constant.APITypeBaiduV2, concreteType: "*baidu_v2.Adaptor", mode: relayconstant.RelayModeChatCompletions,
			format: types.RelayFormatOpenAI, path: "/v2/chat/completions", apiKey: "secret-token|app-17", model: "ernie-4.0-search", responseBody: chatResponse,
			checkUsage: checkChatUsage,
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, Messages: []dto.Message{{Role: "user", Content: "hello"}}})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
				assert.Equal(t, "app-17", r.Header.Get("appid"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, "ernie-4.0", got["model"])
				assert.Equal(t, map[string]any{"enable": true, "enable_citation": true, "enable_trace": true, "enable_status": false}, got["web_search"])
			},
		},
		{
			name: "coze", apiType: constant.APITypeCoze, concreteType: "*coze.Adaptor", mode: relayconstant.RelayModeChatCompletions,
			format: types.RelayFormatOpenAI, stream: true, path: "/v3/chat", apiKey: "coze-key", model: "bot-model",
			responseBody: "event: conversation.message.delta\ndata: {\"content\":\"\\\"answer\\\"\",\"type\":\"answer\"}\n\nevent: conversation.chat.completed\ndata: {\"usage\":{\"input_count\":2,\"output_count\":3,\"token_count\":5}}\n\n",
			checkUsage:   checkChatUsage,
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				c.Set("bot_id", "bot-17")
				stream := true
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, Stream: &stream, User: json.RawMessage(`"test-user"`), Messages: []dto.Message{{Role: "user", Content: "hello"}}})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer coze-key", r.Header.Get("Authorization"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, "bot-17", got["bot_id"])
				assert.Equal(t, true, got["stream"])
				messages, ok := got["additional_messages"].([]any)
				require.True(t, ok)
				require.Len(t, messages, 1)
				assert.Equal(t, "hello", messages[0].(map[string]any)["content"])
			},
		},
		{
			name: "dify", apiType: constant.APITypeDify, concreteType: "*dify.Adaptor", mode: relayconstant.RelayModeChatCompletions,
			format: types.RelayFormatOpenAI, path: "/v1/chat-messages", apiKey: "dify-key", model: "dify-model",
			responseBody: `{"conversation_id":"conv-1","answer":"answer","metadata":{"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}}`,
			checkUsage:   checkChatUsage,
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, User: json.RawMessage(`"test-user"`), Messages: []dto.Message{{Role: "user", Content: "hello"}}})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer dify-key", r.Header.Get("Authorization"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, "test-user", got["user"])
				assert.Equal(t, "blocking", got["response_mode"])
				assert.Contains(t, got["query"], "USER: \nhello")
			},
		},
		{
			name: "jimeng", apiType: constant.APITypeJimeng, concreteType: "*jimeng.Adaptor", mode: relayconstant.RelayModeImagesGenerations,
			format: types.RelayFormatOpenAIImage, path: "/", apiKey: "jimeng-ak|jimeng-sk", model: "jimeng_high_aes_general_v21_L",
			responseBody: `{"code":10000,"data":{"binary_data_base64":["aGVsbG8="]}}`,
			checkOutput:  func(t *testing.T, output string) { assert.Contains(t, output, "aGVsbG8=") },
			checkUsage:   func(t *testing.T, usage any) { assert.Equal(t, &dto.Usage{}, usage) },
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				return a.ConvertImageRequest(c, info, dto.ImageRequest{Model: info.UpstreamModelName, Prompt: "a test image"})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "/", r.URL.Path)
				assert.Equal(t, "CVProcess", r.URL.Query().Get("Action"))
				assert.Equal(t, "2022-08-31", r.URL.Query().Get("Version"))
				assert.Contains(t, r.Header.Get("Authorization"), "HMAC-SHA256 Credential=jimeng-ak/")
				assert.Contains(t, r.Header.Get("Authorization"), "SignedHeaders=content-type;host;x-content-sha256;x-date")
				assert.NotEmpty(t, r.Header.Get("X-Date"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, "jimeng_high_aes_general_v21_L", got["req_key"])
				assert.Equal(t, "a test image", got["prompt"])
				assert.Equal(t, true, got["return_url"])
			},
		},
		{
			name: "mokaai", apiType: constant.APITypeMokaAI, concreteType: "*mokaai.Adaptor", mode: relayconstant.RelayModeEmbeddings,
			format: types.RelayFormatEmbedding, path: "/embeddings", apiKey: "moka-key", model: "m3e-large",
			responseBody: `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"model":"m3e-large","usage":{"prompt_tokens":2,"total_tokens":2}}`,
			checkOutput: func(t *testing.T, output string) {
				var got map[string]any
				require.NoError(t, common.Unmarshal([]byte(output), &got))
				assert.Equal(t, "baidu-embedding", got["model"])
				assert.Equal(t, float64(2), got["usage"].(map[string]any)["prompt_tokens"])
			},
			checkUsage: func(t *testing.T, usage any) {
				got, ok := usage.(*dto.Usage)
				require.True(t, ok)
				assert.Equal(t, 2, got.PromptTokens)
				assert.Equal(t, 2, got.TotalTokens)
			},
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, Input: "hello"})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer moka-key", r.Header.Get("Authorization"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, []any{"hello"}, got["input"])
				assert.Equal(t, "m3e-large", got["model"])
			},
		},
		{
			name: "newapi", apiType: constant.APITypeNewAPI, concreteType: "*newapi.Adaptor", mode: relayconstant.RelayModeChatCompletions,
			format: types.RelayFormatOpenAI, path: "/v1/chat/completions", apiKey: "newapi-key", model: "upstream-model", responseBody: chatResponse,
			checkUsage: checkChatUsage,
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, Messages: []dto.Message{{Role: "user", Content: "hello"}}})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer newapi-key", r.Header.Get("Authorization"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, "upstream-model", got["model"])
			},
		},
		{
			name: "palm", apiType: constant.APITypePaLM, concreteType: "*palm.Adaptor", mode: relayconstant.RelayModeChatCompletions,
			format: types.RelayFormatOpenAI, path: "/v1beta2/models/chat-bison-001:generateMessage", apiKey: "palm-key", model: "chat-bison-001",
			responseBody: `{"candidates":[{"author":"assistant","content":"answer"}]}`,
			checkUsage: func(t *testing.T, usage any) {
				got, ok := usage.(*dto.Usage)
				require.True(t, ok)
				assert.Equal(t, 2, got.PromptTokens)
				assert.Positive(t, got.CompletionTokens)
			},
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, Messages: []dto.Message{{Role: "user", Content: "hello"}}})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "palm-key", r.Header.Get("x-goog-api-key"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, "chat-bison-001", got["model"])
			},
		},
		{
			name: "perplexity", apiType: constant.APITypePerplexity, concreteType: "*perplexity.Adaptor", mode: relayconstant.RelayModeChatCompletions,
			format: types.RelayFormatOpenAI, path: "/chat/completions", apiKey: "pplx-key", model: "sonar", responseBody: chatResponse,
			checkUsage: checkChatUsage,
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				topP := 1.0
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, TopP: &topP, Messages: []dto.Message{{Role: "user", Content: "hello"}}})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer pplx-key", r.Header.Get("Authorization"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, 0.99, got["top_p"])
				assert.Equal(t, "sonar", got["model"])
			},
		},
		{
			name: "replicate", apiType: constant.APITypeReplicate, concreteType: "*replicate.Adaptor", mode: relayconstant.RelayModeImagesGenerations,
			format: types.RelayFormatOpenAIImage, path: "/v1/models/black-forest-labs/flux-1.1-pro/predictions", apiKey: "replicate-key", model: "black-forest-labs/flux-1.1-pro",
			responseBody: `{"status":"succeeded","output":["https://images.example/one.png"]}`,
			checkOutput:  func(t *testing.T, output string) { assert.Contains(t, output, "https://images.example/one.png") },
			checkUsage:   func(t *testing.T, usage any) { assert.Equal(t, &dto.Usage{}, usage) },
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				n := uint(2)
				return a.ConvertImageRequest(c, info, dto.ImageRequest{Model: info.UpstreamModelName, Prompt: "a blue square", Size: "1024x1024", N: &n, Quality: "hd"})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer replicate-key", r.Header.Get("Authorization"))
				assert.Equal(t, "wait", r.Header.Get("Prefer"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				input := got["input"].(map[string]any)
				assert.Equal(t, "a blue square", input["prompt"])
				assert.Equal(t, float64(2), input["num_outputs"])
				assert.Equal(t, "1:1", input["aspect_ratio"])
				assert.Equal(t, true, input["prompt_upsampling"])
			},
		},
		{
			name: "submodel", apiType: constant.APITypeSubmodel, concreteType: "*submodel.Adaptor", mode: relayconstant.RelayModeChatCompletions,
			format: types.RelayFormatOpenAI, path: "/v1/chat/completions", apiKey: "submodel-key", model: "provider/model", responseBody: chatResponse,
			checkUsage: checkChatUsage,
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, Messages: []dto.Message{{Role: "user", Content: "hello"}}})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer submodel-key", r.Header.Get("Authorization"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, "provider/model", got["model"])
			},
		},
		{
			name: "volcengine", apiType: constant.APITypeVolcEngine, concreteType: "*volcengine.Adaptor", mode: relayconstant.RelayModeChatCompletions,
			format: types.RelayFormatOpenAI, path: "/api/v3/chat/completions", apiKey: "volc-key", model: "deepseek-v3-thinking", responseBody: chatResponse,
			checkUsage: checkChatUsage,
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, Messages: []dto.Message{{Role: "user", Content: "hello"}}})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer volc-key", r.Header.Get("Authorization"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, "deepseek-v3", got["model"])
				assert.Equal(t, map[string]any{"type": "enabled"}, got["thinking"])
			},
		},
		{
			name: "xai", apiType: constant.APITypeXai, concreteType: "*xai.Adaptor", mode: relayconstant.RelayModeChatCompletions,
			format: types.RelayFormatOpenAI, path: "/v1/chat/completions", apiKey: "xai-key", model: "grok-3-mini-high", responseBody: chatResponse,
			checkUsage: checkChatUsage,
			convert: func(c *gin.Context, info *relaycommon.RelayInfo, a interfaceAdaptor) (any, error) {
				maxTokens := uint(64)
				return a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, MaxTokens: &maxTokens, Messages: []dto.Message{{Role: "user", Content: "hello"}}})
			},
			checkRequest: func(t *testing.T, r *http.Request, body []byte) {
				assert.Equal(t, "Bearer xai-key", r.Header.Get("Authorization"))
				var got map[string]any
				require.NoError(t, common.Unmarshal(body, &got))
				assert.Equal(t, "grok-3-mini", got["model"])
				assert.Equal(t, "high", got["reasoning_effort"])
				assert.Equal(t, float64(64), got["max_completion_tokens"])
				assert.NotContains(t, got, "max_tokens")
			},
		},
	}

	for _, tc := range contracts {
		t.Run(tc.name, func(t *testing.T) {
			type capturedRequest struct {
				method string
				url    *url.URL
				header http.Header
				body   []byte
				err    error
			}
			captured := make(chan capturedRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				captured <- capturedRequest{method: r.Method, url: r.URL, header: r.Header.Clone(), body: body, err: err}
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
				} else {
					w.Header().Set("Content-Type", "application/json")
				}
				_, _ = io.WriteString(w, tc.responseBody)
			}))
			defer server.Close()

			info := &relaycommon.RelayInfo{
				RelayMode: tc.mode, RelayFormat: tc.format, IsStream: tc.stream, RequestURLPath: tc.path,
				OriginModelName: tc.model, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: tc.apiKey, UpstreamModelName: tc.model},
			}
			if tc.name == "palm" {
				info.SetEstimatePromptTokens(2)
			}
			adaptor := GetAdaptor(tc.apiType)
			require.NotNil(t, adaptor)
			assert.Equal(t, tc.concreteType, fmt.Sprintf("%T", adaptor))
			adaptor.Init(info)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/relay", nil)
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Request.Header.Set("Accept", "application/json")
			requestValue, err := tc.convert(ctx, info, adaptor.(interfaceAdaptor))
			require.NoError(t, err)
			requestBody, err := common.Marshal(requestValue)
			require.NoError(t, err)
			if tc.name == "replicate" {
				info.Request = &dto.ImageRequest{ResponseFormat: "url"}
			}

			response, err := adaptor.DoRequest(ctx, info, bytes.NewReader(requestBody))
			require.NoError(t, err)
			httpResponse, ok := response.(*http.Response)
			require.True(t, ok, "adaptor should return its upstream HTTP response")
			defer httpResponse.Body.Close()

			gotRequest := <-captured
			require.NoError(t, gotRequest.err)
			assert.Equal(t, http.MethodPost, gotRequest.method)
			assert.Equal(t, tc.path, gotRequest.url.Path)
			tc.checkRequest(t, &http.Request{Header: gotRequest.header, URL: gotRequest.url}, gotRequest.body)

			usage, apiErr := adaptor.DoResponse(ctx, httpResponse, info)
			require.Nil(t, apiErr)
			require.NotNil(t, tc.checkUsage)
			tc.checkUsage(t, usage)
			output := recorder.Body.String()
			if tc.checkOutput != nil {
				tc.checkOutput(t, output)
			} else {
				assert.Contains(t, output, "answer")
			}
		})
	}
}

func TestRemainingAdaptorDispatchedHTTPContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	chatResponse := `{"id":"resp_1","object":"chat.completion","created":1,"model":"upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`
	for _, tc := range []struct {
		name, path, model, authHeader, authValue, response string
		apiType, channelType                               int
	}{
		{name: "openai", apiType: constant.APITypeOpenAI, channelType: constant.ChannelTypeOpenAI, path: "/v1/chat/completions", model: "gpt-contract", response: chatResponse},
		{name: "openrouter", apiType: constant.APITypeOpenRouter, channelType: constant.ChannelTypeOpenRouter, path: "/v1/chat/completions", model: "gpt-contract", response: chatResponse},
		{name: "xinference", apiType: constant.APITypeXinference, path: "/v1/chat/completions", model: "local-contract", response: chatResponse},
		{name: "ali", apiType: constant.APITypeAli, path: "/compatible-mode/v1/chat/completions", model: "qwen-contract", response: chatResponse},
		{name: "deepseek", apiType: constant.APITypeDeepSeek, path: "/v1/chat/completions", model: "deepseek-chat", response: chatResponse},
		{name: "minimax", apiType: constant.APITypeMiniMax, path: "/v1/text/chatcompletion_v2", model: "MiniMax-M2", response: chatResponse},
		{name: "moonshot", apiType: constant.APITypeMoonshot, path: "/v1/chat/completions", model: "kimi-k2", response: chatResponse},
		{name: "sub2api", apiType: constant.APITypeSub2API, path: "/v1/chat/completions", model: "gpt-contract", response: chatResponse},
		{name: "advancedcustom", apiType: constant.APITypeAdvancedCustom, path: "/custom/chat", model: "custom-contract", response: chatResponse},
		{name: "claude", apiType: constant.APITypeAnthropic, path: "/v1/messages", model: "claude-contract", authHeader: "x-api-key", authValue: "contract-key", response: `{"id":"c1","type":"message","role":"assistant","model":"claude-contract","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`},
		{name: "gemini", apiType: constant.APITypeGemini, path: "/v1beta/models/gemini-2.5-flash:generateContent", model: "gemini-2.5-flash", authHeader: "x-goog-api-key", authValue: "contract-key", response: `{"candidates":[{"content":{"parts":[{"text":"answer"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}}`},
		{name: "ollama", apiType: constant.APITypeOllama, path: "/api/chat", model: "llama3", response: `{"model":"llama3","message":{"role":"assistant","content":"answer"},"done":true,"prompt_eval_count":2,"eval_count":3}`},
		{name: "codex", apiType: constant.APITypeCodex, path: "/backend-api/codex/responses/compact", model: "gpt-contract", response: `{"id":"answer","object":"response.compaction","output":[],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captured := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, tc.path, r.URL.Path)
				header, value := tc.authHeader, tc.authValue
				if header == "" {
					header, value = "Authorization", "Bearer contract-key"
				}
				assert.Equal(t, value, r.Header.Get(header))
				if tc.name == "openrouter" {
					assert.Equal(t, common.AdtyBrandName, r.Header.Get("X-OpenRouter-Title"))
				}
				if tc.name == "claude" {
					assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
				}
				if tc.name == "codex" {
					assert.Equal(t, "contract-account", r.Header.Get("chatgpt-account-id"))
				}
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				captured <- body
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()
			info := &relaycommon.RelayInfo{
				RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: types.RelayFormatOpenAI, RequestURLPath: "/v1/chat/completions", OriginModelName: tc.model,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "contract-key", UpstreamModelName: tc.model, ChannelType: tc.channelType},
			}
			if tc.name == "advancedcustom" {
				info.ChannelOtherSettings.AdvancedCustom = &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: info.RequestURLPath, UpstreamPath: tc.path}}}
			}
			if tc.name == "codex" {
				info.ApiKey = `{"access_token":"contract-key","account_id":"contract-account"}`
				info.RelayMode = relayconstant.RelayModeResponsesCompact
			}
			adaptor := GetAdaptor(tc.apiType)
			require.NotNil(t, adaptor)
			adaptor.Init(info)
			out := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(out)
			ctx.Request = httptest.NewRequest(http.MethodPost, info.RequestURLPath, nil)
			ctx.Request.Header.Set("Content-Type", "application/json")
			var request any
			var err error
			if tc.name == "codex" {
				request, err = adaptor.ConvertOpenAIResponsesRequest(ctx, info, dto.OpenAIResponsesRequest{Model: tc.model, Input: json.RawMessage(`"question"`)})
			} else {
				request, err = adaptor.ConvertOpenAIRequest(ctx, info, &dto.GeneralOpenAIRequest{Model: tc.model, Messages: []dto.Message{{Role: "user", Content: "question"}}})
			}
			require.NoError(t, err)
			body, err := common.Marshal(request)
			require.NoError(t, err)
			response, err := adaptor.DoRequest(ctx, info, bytes.NewReader(body))
			require.NoError(t, err)
			assert.Contains(t, string(<-captured), "question")
			usage, apiErr := adaptor.DoResponse(ctx, response.(*http.Response), info)
			require.Nil(t, apiErr)
			got, ok := usage.(*dto.Usage)
			require.True(t, ok)
			assert.Equal(t, 2, got.PromptTokens)
			assert.Equal(t, 3, got.CompletionTokens)
			assert.Equal(t, 5, got.TotalTokens)
			assert.Contains(t, out.Body.String(), "answer")
			_, apiErr = adaptor.DoResponse(ctx, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{"))}, info)
			require.NotNil(t, apiErr, "malformed response must not report success")
		})
	}
}

func TestExtendedAdaptorSharedResponseErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		apiType int
		format  types.RelayFormat
		mode    int
		body    string
		status  int
	}{
		{name: "baidu v2 openai branch preserves upstream status", apiType: constant.APITypeBaiduV2, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, body: `{"error":{"message":"denied","type":"auth_error","code":"invalid_api_key"}}`, status: http.StatusUnauthorized},
		{name: "coze rejects malformed detail response", apiType: constant.APITypeCoze, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
		{name: "dify rejects malformed json", apiType: constant.APITypeDify, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
		{name: "jimeng rejects malformed json", apiType: constant.APITypeJimeng, format: types.RelayFormatOpenAIImage, mode: relayconstant.RelayModeImagesGenerations, body: `{`, status: http.StatusOK},
		{name: "moka embeddings rejects malformed json", apiType: constant.APITypeMokaAI, format: types.RelayFormatEmbedding, mode: relayconstant.RelayModeEmbeddings, body: `{`, status: http.StatusOK},
		{name: "newapi claude branch rejects malformed json", apiType: constant.APITypeNewAPI, format: types.RelayFormatClaude, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
		{name: "newapi gemini branch rejects malformed json", apiType: constant.APITypeNewAPI, format: types.RelayFormatGemini, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
		{name: "newapi openai branch rejects malformed json", apiType: constant.APITypeNewAPI, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
		{name: "palm rejects malformed json", apiType: constant.APITypePaLM, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
		{name: "perplexity openai branch rejects malformed json", apiType: constant.APITypePerplexity, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
		{name: "replicate rejects malformed json", apiType: constant.APITypeReplicate, format: types.RelayFormatOpenAIImage, mode: relayconstant.RelayModeImagesGenerations, body: `{`, status: http.StatusOK},
		{name: "submodel openai branch rejects malformed json", apiType: constant.APITypeSubmodel, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
		{name: "volcengine openai branch rejects malformed json", apiType: constant.APITypeVolcEngine, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
		{name: "xai rejects malformed json", apiType: constant.APITypeXai, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, body: `{`, status: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adaptor := GetAdaptor(tc.apiType)
			require.NotNil(t, adaptor)
			info := &relaycommon.RelayInfo{RelayMode: tc.mode, RelayFormat: tc.format, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-model"}}
			adaptor.Init(info)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			resp := &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}
			_, apiErr := adaptor.DoResponse(ctx, resp, info)
			require.NotNil(t, apiErr)
			if tc.status != http.StatusOK {
				assert.Equal(t, tc.status, apiErr.StatusCode)
			}
		})
	}
}

func TestExtendedAdaptorRequestHonorsHTTPClientTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalTimeout := common.RelayTimeout
	common.RelayTimeout = 1
	t.Cleanup(func() {
		common.RelayTimeout = originalTimeout
		service.InitHttpClient()
	})
	service.InitHttpClient()

	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseHandler
	}))
	defer server.Close()
	defer close(releaseHandler)

	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/relay", strings.NewReader(`{"model":"m","messages":[]}`))
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeChatCompletions,
		RequestURLPath: "/v1/chat/completions",
		ChannelMeta:    &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "test-key", UpstreamModelName: "test-model"},
	}
	adaptor := GetAdaptor(constant.APITypeSubmodel)
	require.NotNil(t, adaptor)

	result := make(chan error, 1)
	go func() {
		_, err := adaptor.DoRequest(ginContext, info, strings.NewReader(`{"model":"m","messages":[]}`))
		result <- err
	}()
	<-requestStarted
	err := <-result
	require.Error(t, err)
}

func TestVolcengineVisionEmbeddingContract(t *testing.T) {
	service.InitHttpClient()
	adaptor := GetAdaptor(constant.APITypeVolcEngine)
	require.NotNil(t, adaptor)
	const model = "doubao-embedding-vision-251215"
	for _, tc := range []struct {
		name, input, wantInput string
	}{
		{"text", `"a cat"`, `[{"type":"text","text":"a cat"}]`},
		{"single text batch", `["a cat"]`, `[{"type":"text","text":"a cat"}]`},
		{"mixed", `[{"type":"text","text":"a cat"},{"type":"image_url","image_url":{"url":"https://example.com/cat.png"}},{"type":"video_url","video_url":{"url":"https://example.com/cat.mp4"}}]`, `[{"type":"text","text":"a cat"},{"type":"image_url","image_url":{"url":"https://example.com/cat.png"}},{"type":"video_url","video_url":{"url":"https://example.com/cat.mp4"}}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "/api/v3/embeddings/multimodal", r.URL.Path)
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"model":"`+model+`","input":`+tc.wantInput+`,"dimensions":1024,"encoding_format":"float"}`, string(body))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"emb_1","model":"`+model+`","object":"list","data":{"object":"embedding","embedding":[0.25,-0.5]},"usage":{"prompt_tokens":25,"prompt_tokens_details":{"text_tokens":5,"image_tokens":20},"total_tokens":25}}`)
			}))
			t.Cleanup(server.Close)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeEmbeddings, RelayFormat: types.RelayFormatEmbedding, OriginModelName: model, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, UpstreamModelName: model}}
			var request dto.EmbeddingRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"`+model+`","input":`+tc.input+`,"dimensions":1024}`, &request))
			meta := request.GetTokenCountMeta()
			assert.Equal(t, "a cat", meta.CombineText)
			if tc.name == "mixed" {
				require.Len(t, meta.Files, 2)
			}
			converted, err := adaptor.ConvertEmbeddingRequest(c, info, request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			response, err := adaptor.DoRequest(c, info, bytes.NewReader(body))
			require.NoError(t, err)
			usageValue, apiErr := adaptor.DoResponse(c, response.(*http.Response), info)
			require.Nil(t, apiErr)
			usage, ok := usageValue.(*dto.Usage)
			require.True(t, ok)
			assert.Equal(t, 25, usage.PromptTokens)
			assert.Equal(t, 25, usage.TotalTokens)
			assert.Zero(t, usage.CompletionTokens)
			assert.Equal(t, 5, usage.PromptTokensDetails.TextTokens)
			assert.Equal(t, 20, usage.PromptTokensDetails.ImageTokens)
			var output dto.EmbeddingResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &output))
			require.Len(t, output.Data, 1)
			assert.Equal(t, dto.EmbeddingResponseItem{Object: "embedding", Index: 0, Embedding: []float64{0.25, -0.5}}, output.Data[0])
			assert.Equal(t, model, output.Model)
			// Final parameter overrides cannot inject content absent from the
			// original EmbeddingRequest's text/file pre-consume estimate.
			_, err = adaptor.DoRequest(c, info, strings.NewReader(`{"model":"`+model+`","input":[{"type":"image_url","image_url":{"url":"https://example.com/unestimated.png"}}],"encoding_format":"float"}`))
			require.Error(t, err)
			assert.Equal(t, 1, calls)
		})
	}
	for _, fields := range []string{
		`"input":{"image":"https://example.com/cat.png"}`,
		`"input":["a cat","a dog"]`,
		`"input":[{"type":"image_url","image_url":{"url":""}}]`,
		`"input":[{"type":"video_url","video_url":{"url":"https://example.com/cat.mp4","max_video_tokens":204800}}]`,
		`"input":"a cat","encoding_format":"base64"`,
		`"input":"a cat","dimensions":0`,
	} {
		var request dto.EmbeddingRequest
		require.NoError(t, common.UnmarshalJsonStr(`{"model":"`+model+`",`+fields+`}`, &request))
		_, err := adaptor.ConvertEmbeddingRequest(nil, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model}}, request)
		require.Error(t, err, fields)
	}
	for _, body := range []string{
		`{"error":{"message":"denied","type":"invalid_request_error","code":"bad_input"}}`,
		`{"data":{"embedding":[0.1]},"usage":{"prompt_tokens":-1,"total_tokens":-1}}`,
		`{"data":{"embedding":[0.1]},"usage":{"prompt_tokens":9223372036854775807,"total_tokens":9223372036854775807}}`,
		`{"data":{"embedding":[0.1]},"usage":{"prompt_tokens":1,"total_tokens":1,"prompt_tokens_details":{"image_tokens":2}}}`,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeEmbeddings, RelayFormat: types.RelayFormatEmbedding, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model}}
		_, apiErr := adaptor.DoResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, info)
		require.NotNil(t, apiErr, body)
	}
	legacy := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeEmbeddings, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://ark.example", UpstreamModelName: "doubao-embedding-text-240715"}}
	url, err := adaptor.GetRequestURL(legacy)
	require.NoError(t, err)
	assert.Equal(t, "https://ark.example/api/v3/embeddings", url)
}
