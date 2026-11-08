package cohere_test

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/cloudflare"
	"github.com/QuantumNous/new-api/relay/channel/cohere"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/jina"
	"github.com/QuantumNous/new-api/relay/channel/mistral"
	"github.com/QuantumNous/new-api/relay/channel/zhipu_4v"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCohereEmbeddingContract(t *testing.T) {
	a := &cohere.Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeEmbeddings, ChannelMeta: &relaycommon.ChannelMeta{}}
	info.ChannelBaseUrl = "https://api.cohere.com"
	info.UpstreamModelName = "embed-v4.0"
	dimensions := 256
	request, err := a.ConvertEmbeddingRequest(nil, info, dto.EmbeddingRequest{Model: "embed-v4.0", Input: []any{"first", "second"}, Dimensions: &dimensions})
	require.NoError(t, err)
	body, err := common.Marshal(request)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"embed-v4.0","texts":["first","second"],"input_type":"search_document","embedding_types":["float"],"output_dimension":256}`, string(body))
	url, err := a.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.cohere.com/v2/embed", url)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"embed-1","embeddings":{"float":[[0.1,0.2],[0.3,0.4]]},"meta":{"billed_units":{"input_tokens":7}}}`))}
	usage, apiErr := a.DoResponse(c, resp, info)
	require.Nil(t, apiErr)
	assert.Equal(t, 7, usage.(*dto.Usage).PromptTokens)
	var result dto.EmbeddingResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	assert.Equal(t, "list", result.Object)
	assert.Equal(t, "embed-v4.0", result.Model)
	assert.Equal(t, []dto.EmbeddingResponseItem{{Object: "embedding", Index: 0, Embedding: []float64{0.1, 0.2}}, {Object: "embedding", Index: 1, Embedding: []float64{0.3, 0.4}}}, result.Data)
}

func TestCohereEmbeddingRejectsTokenIDInput(t *testing.T) {
	_, err := (&cohere.Adaptor{}).ConvertEmbeddingRequest(nil, nil, dto.EmbeddingRequest{Model: "embed-v4.0", Input: []any{1.0, 2.0}})
	require.Error(t, err)
}

func TestCohereEmbeddingUsagePresenceAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name, responseMeta string
		wantTokens         int
		wantError          bool
	}{
		{"billed preferred", `,"meta":{"billed_units":{"input_tokens":7},"tokens":{"input_tokens":11}}`, 7, false},
		{"integer-valued billed double", `,"meta":{"billed_units":{"input_tokens":9.0}}`, 9, false},
		{"explicit billed zero", `,"meta":{"billed_units":{"input_tokens":0},"tokens":{"input_tokens":11}}`, 0, false},
		{"tokens fallback", `,"meta":{"tokens":{"input_tokens":11}}`, 11, false},
		{"integer-valued fallback double", `,"meta":{"tokens":{"input_tokens":9.0}}`, 9, false},
		{"explicit tokens zero", `,"meta":{"tokens":{"input_tokens":0}}`, 0, false},
		{"missing meta", "", 4, false},
		{"missing billed tokens", `,"meta":{"billed_units":{}}`, 4, false},
		{"negative billed tokens", `,"meta":{"billed_units":{"input_tokens":-1}}`, 0, true},
		{"fractional billed tokens", `,"meta":{"billed_units":{"input_tokens":1.5}}`, 0, true},
		{"oversized billed tokens", `,"meta":{"billed_units":{"input_tokens":2147483648}}`, 0, true},
		{"negative fallback tokens", `,"meta":{"tokens":{"input_tokens":-1}}`, 0, true},
		{"fractional fallback tokens", `,"meta":{"tokens":{"input_tokens":1.5}}`, 0, true},
		{"oversized fallback tokens", `,"meta":{"tokens":{"input_tokens":2147483648}}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeEmbeddings, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "embed-v4.0"}}
			info.SetEstimatePromptTokens(4)
			body := `{"embeddings":{"float":[[0.1,0.2]]}` + tc.responseMeta + `}`
			usage, apiErr := (&cohere.Adaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, info)
			if tc.wantError {
				require.NotNil(t, apiErr)
				assert.Nil(t, usage)
				assert.Empty(t, recorder.Body.String())
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, tc.wantTokens, usage.(*dto.Usage).PromptTokens)
			assert.Equal(t, tc.wantTokens, usage.(*dto.Usage).TotalTokens)
		})
	}
}

func TestCohereEmbeddingValidatesVectorPayloads(t *testing.T) {
	for _, tc := range []struct {
		name, embeddings string
		wantError        bool
	}{
		{"empty float vector", `{"float":[[]]}`, true},
		{"null float vector", `{"float":[null]}`, true},
		{"null float element", `{"float":[[null]]}`, true},
		{"empty base64 vector", `{"base64":[""]}`, true},
		{"invalid base64 vector", `{"base64":["not base64!"]}`, true},
		{"valid base64 vector", `{"base64":["AACAPw=="]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeEmbeddings, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "embed-v4.0"}}
			body := `{"embeddings":` + tc.embeddings + `,"meta":{"billed_units":{"input_tokens":7}}}`
			usage, apiErr := (&cohere.Adaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, info)
			if tc.wantError {
				require.NotNil(t, apiErr)
				assert.Nil(t, usage)
				assert.Empty(t, recorder.Body.String())
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, 7, usage.(*dto.Usage).PromptTokens)
			assert.Equal(t, "AACAPw==", gjson.GetBytes(recorder.Body.Bytes(), "data.0.embedding").String())
		})
	}
}

func TestCohereCurrentChatUsesCompatibilityProtocol(t *testing.T) {
	for _, model := range []string{"c4ai-aya-expanse-32b", "north-mini-code-1.0", "north-small-translate-1.0", "tiny-aya-earth", "tiny-aya-fire", "tiny-aya-global", "tiny-aya-water"} {
		t.Run(model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeChatCompletions, RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.cohere.com", UpstreamModelName: model}}
			request := &dto.GeneralOpenAIRequest{Model: model, Messages: []dto.Message{{Role: "user", Content: "first"}, {Role: "assistant", Content: "reply"}, {Role: "user", Content: "second"}}}
			a := &cohere.Adaptor{}
			converted, err := a.ConvertOpenAIRequest(nil, info, request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var sent map[string]any
			require.NoError(t, common.Unmarshal(body, &sent))
			assert.Equal(t, []any{map[string]any{"role": "user", "content": "first"}, map[string]any{"role": "assistant", "content": "reply"}, map[string]any{"role": "user", "content": "second"}}, sent["messages"])
			url, err := a.GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://api.cohere.com/compatibility/v1/chat/completions", url)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			responseBody := `{"id":"chat-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`
			usage, apiErr := a.DoResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(responseBody))}, info)
			require.Nil(t, apiErr)
			assert.Equal(t, 9, usage.(*dto.Usage).TotalTokens)
			assert.JSONEq(t, responseBody, recorder.Body.String())
		})
	}
}

func TestMistralEmbeddingContract(t *testing.T) {
	request, err := (&mistral.Adaptor{}).ConvertEmbeddingRequest(nil, nil, dto.EmbeddingRequest{Model: "codestral-embed-2505", Input: []any{"func main() {}"}, EncodingFormat: "base64"})
	require.NoError(t, err)
	body, err := common.Marshal(request)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"codestral-embed-2505","input":["func main() {}"],"encoding_format":"base64"}`, string(body))
}

func TestJinaEmbeddingMapsMediaAndEncoding(t *testing.T) {
	request, err := (&jina.Adaptor{}).ConvertEmbeddingRequest(nil, nil, dto.EmbeddingRequest{
		Model: "jina-embeddings-v5-omni-small", EncodingFormat: "base64",
		Input: []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}}, map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/a.mp4"}}},
	})
	require.NoError(t, err)
	body, err := common.Marshal(request)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"jina-embeddings-v5-omni-small","input":[{"image":"https://example.com/a.png"},{"video":"https://example.com/a.mp4"}],"embedding_type":"base64"}`, string(body))
}

func TestJinaEmbeddingRejectsInputsWithoutBillingEstimates(t *testing.T) {
	for _, input := range []any{
		map[string]any{"image": "https://example.com/a.png"},
		[]any{map[string]any{"audio": "https://example.com/a.wav"}},
		[]any{map[string]any{"text": "unestimated native text"}},
		[]any{map[string]any{"content": []any{map[string]any{"text": "fused"}}}},
		[]any{map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "YWJj", "format": "wav"}}},
		[]any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": ""}}},
		[]any{map[string]any{"type": "image_url", "image_url": "https://example.com/a.png"}},
		[]any{map[string]any{"type": "video_url", "video_url": "https://example.com/a.mp4"}},
		[]any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}, "text": 1}},
		[]any{1.0, 2.0},
	} {
		_, err := (&jina.Adaptor{}).ConvertEmbeddingRequest(nil, nil, dto.EmbeddingRequest{Model: "jina-embeddings-v5-omni-small", Input: input})
		require.Error(t, err, "input: %#v", input)
	}
}

func TestJinaEmbeddingFinalOverridesPreserveCountedInput(t *testing.T) {
	for _, tc := range []struct {
		name           string
		input          any
		override       map[string]any
		skipConversion bool
		wantError      bool
	}{
		{name: "unchanged text", input: "counted text"},
		{name: "unchanged mapped media and output options", input: []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/counted.png"}}}, override: map[string]any{"dimensions": 256, "embedding_type": "base64"}},
		{name: "changed text", input: "counted text", override: map[string]any{"input": "a different uncounted text"}, wantError: true},
		{name: "changed model", input: "counted text", override: map[string]any{"model": "jina-other-model"}, wantError: true},
		{name: "empty model", input: "counted text", override: map[string]any{"model": ""}, wantError: true},
		{name: "null model", input: "counted text", override: map[string]any{"model": nil}, wantError: true},
		{name: "native image override", input: "counted text", override: map[string]any{"input": []any{map[string]any{"image": "https://example.com/uncounted.png"}}}, wantError: true},
		{name: "native audio override", input: "counted text", override: map[string]any{"input": []any{map[string]any{"audio": "https://example.com/uncounted.wav"}}}, wantError: true},
		{name: "native PDF override", input: "counted text", override: map[string]any{"input": []any{map[string]any{"pdf": "https://example.com/uncounted.pdf"}}}, wantError: true},
		{name: "native merged content override", input: "counted text", override: map[string]any{"input": []any{map[string]any{"content": []any{map[string]any{"text": "uncounted"}}}}}, wantError: true},
		{name: "missing counted request", input: "counted text", skipConversion: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan []byte, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- body
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":[{"embedding":[0.1]}]}`)
			}))
			defer upstream.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeEmbeddings, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: upstream.URL, UpstreamModelName: "jina-embeddings-v5-omni-small", ParamOverride: tc.override}}
			a := &jina.Adaptor{}
			var converted any = map[string]any{"model": info.UpstreamModelName, "input": tc.input}
			var err error
			if !tc.skipConversion {
				converted, err = a.ConvertEmbeddingRequest(c, info, dto.EmbeddingRequest{Model: info.UpstreamModelName, Input: tc.input})
				require.NoError(t, err)
			}
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			body, err = relaycommon.ApplyParamOverrideWithRelayInfo(body, info)
			require.NoError(t, err)
			response, err := a.DoRequest(c, info, bytes.NewReader(body))
			if upstreamResponse, ok := response.(*http.Response); ok && upstreamResponse != nil {
				defer upstreamResponse.Body.Close()
			}
			if tc.wantError {
				require.Error(t, err)
				assert.Empty(t, requests, "invalid overrides must be rejected before any upstream request")
				return
			}
			require.NoError(t, err)
			require.Len(t, requests, 1)
			assert.JSONEq(t, string(body), string(<-requests))
		})
	}
}

func TestJinaEmbeddingRejectsInvalidUpstreamResults(t *testing.T) {
	for _, tc := range []struct {
		name, body string
	}{
		{"upstream error", `{"error":{"message":"upstream failed","type":"upstream_error"},"usage":{"total_tokens":9}}`},
		{"missing vectors", `{"object":"list","usage":{"total_tokens":9}}`},
		{"null vectors", `{"data":null,"usage":{"total_tokens":9}}`},
		{"empty vectors", `{"data":[],"usage":{"total_tokens":9}}`},
		{"nonarray vectors", `{"data":{"embedding":[0.2]},"usage":{"total_tokens":9}}`},
		{"missing embedding", `{"data":[{}],"usage":{"total_tokens":9}}`},
		{"empty embedding", `{"data":[{"embedding":[]}],"usage":{"total_tokens":9}}`},
		{"string vector element", `{"data":[{"embedding":["bad"]}],"usage":{"total_tokens":9}}`},
		{"null vector element", `{"data":[{"embedding":[null]}],"usage":{"total_tokens":9}}`},
		{"object vector element", `{"data":[{"embedding":[{}]}],"usage":{"total_tokens":9}}`},
		{"invalid base64 embedding", `{"data":[{"embedding":"not base64!"}],"usage":{"total_tokens":9}}`},
		{"negative total", `{"data":[{"embedding":[0.2]}],"usage":{"total_tokens":-1}}`},
		{"negative input", `{"data":[{"embedding":[0.2]}],"usage":{"prompt_tokens":-1,"total_tokens":9}}`},
		{"oversized total", `{"data":[{"embedding":[0.2]}],"usage":{"total_tokens":2147483648}}`},
		{"oversized input", `{"data":[{"embedding":[0.2]}],"usage":{"prompt_tokens":2147483648,"total_tokens":9}}`},
		{"nonzero completion", `{"data":[{"embedding":[0.2]}],"usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":9}}`},
		{"inconsistent usage", `{"data":[{"embedding":[0.2]}],"usage":{"prompt_tokens":7,"total_tokens":9}}`},
		{"explicit zero input with nonzero total", `{"data":[{"embedding":[0.2]}],"usage":{"prompt_tokens":0,"total_tokens":9}}`},
		{"nonzero input with explicit zero total", `{"data":[{"embedding":[0.2]}],"usage":{"prompt_tokens":9,"total_tokens":0}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeEmbeddings, ChannelMeta: &relaycommon.ChannelMeta{}}
			info.SetEstimatePromptTokens(7)
			usage, apiErr := (&jina.Adaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, info)
			require.NotNil(t, apiErr)
			assert.Nil(t, usage)
			assert.Empty(t, recorder.Body.String())
		})
	}
}

func TestJinaEmbeddingIgnoresUntrustedAccountingFields(t *testing.T) {
	for _, extraUsage := range []string{
		`"billing_usage":{"source":"oai_chat","semantic":"openai","openai_usage":{"prompt_tokens":-5,"total_tokens":-5}}`,
		`"prompt_tokens_details":{"cached_tokens":2147483648,"image_tokens":-5},"input_tokens":-5,"input_tokens_details":{"audio_tokens":2147483648},"prompt_cache_hit_tokens":-5`,
		`"usage_source":"claude_messages","usage_semantic":"anthropic","claude_cache_creation_1_h_tokens":2147483648,"completion_tokens_details":{"audio_tokens":-5},"output_tokens":-5`,
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
		info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeEmbeddings, ChannelMeta: &relaycommon.ChannelMeta{}}
		body := `{"data":[{"embedding":[0.2]}],"usage":{"prompt_tokens":9,"total_tokens":9,` + extraUsage + `}}`
		usage, apiErr := (&jina.Adaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, info)
		require.Nil(t, apiErr)
		expected := dto.Usage{PromptTokens: 9, TotalTokens: 9}
		assert.Equal(t, &expected, usage)
		var response struct {
			Usage dto.Usage `json:"usage"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Equal(t, expected, response.Usage)
	}
}

func TestJinaEmbeddingPreservesBase64AndUsageFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name, reportedUsage string
		wantTokens          int
	}{
		{"total only", `,"usage":{"total_tokens":9}`, 9},
		{"input only", `,"usage":{"prompt_tokens":9}`, 9},
		{"explicit zero", `,"usage":{"prompt_tokens":0,"total_tokens":0}`, 0},
		{"no usage", "", 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeEmbeddings, ChannelMeta: &relaycommon.ChannelMeta{}}
			info.SetEstimatePromptTokens(7)
			body := `{"object":"list","data":[{"object":"embedding","index":0,"embedding":"AACAPw=="}]` + tc.reportedUsage + `}`
			usage, apiErr := (&jina.Adaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, info)
			require.Nil(t, apiErr)
			assert.Equal(t, tc.wantTokens, usage.(*dto.Usage).PromptTokens)
			assert.Equal(t, tc.wantTokens, usage.(*dto.Usage).TotalTokens)
			assert.Equal(t, "AACAPw==", gjson.GetBytes(recorder.Body.Bytes(), "data.0.embedding").String())
		})
	}
}

func TestGeminiEmbedding2PreservesDimensions(t *testing.T) {
	dimensions := 256
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-embedding-2"}}
	request, err := (&gemini.Adaptor{}).ConvertEmbeddingRequest(nil, info, dto.EmbeddingRequest{Model: "gemini-embedding-2", Input: "hello", Dimensions: &dimensions})
	require.NoError(t, err)
	body, err := common.Marshal(request)
	require.NoError(t, err)
	assert.JSONEq(t, `{"requests":[{"model":"models/gemini-embedding-2","content":{"parts":[{"text":"hello"}]},"outputDimensionality":256}]}`, string(body))
}

func TestGeminiNativeSpeechPreservesAudioContract(t *testing.T) {
	for _, model := range []string{"gemini-3.8-flash-tts", "gemini-3.8-flash-lite-tts"} {
		t.Run(model, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/"+model+":generateContent", nil)
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeGemini, RelayFormat: types.RelayFormatGemini, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://generativelanguage.googleapis.com", UpstreamModelName: model}}
			var request dto.GeminiChatRequest
			require.NoError(t, common.Unmarshal([]byte(`{"contents":[{"role":"user","parts":[{"text":"Say hello","speech_metadata":{"speaker":"narrator","style":"cheerful"}}]}],"generationConfig":{"responseModalities":["AUDIO"],"speechConfig":{"voiceConfig":{"prebuiltVoiceConfig":{"voiceName":"Kore"}}}}}`), &request))
			a := &gemini.Adaptor{}
			converted, err := a.ConvertGeminiRequest(c, info, &request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			assert.JSONEq(t, `{"speaker":"narrator","style":"cheerful"}`, gjson.GetBytes(body, "contents.0.parts.0.speech_metadata").Raw)
			var sent dto.GeminiChatRequest
			require.NoError(t, common.Unmarshal(body, &sent))
			assert.Equal(t, []string{"AUDIO"}, sent.GenerationConfig.ResponseModalities)
			assert.JSONEq(t, `{"voiceConfig":{"prebuiltVoiceConfig":{"voiceName":"Kore"}}}`, string(sent.GenerationConfig.SpeechConfig))
			url, err := a.GetRequestURL(info)
			require.NoError(t, err)
			assert.Contains(t, url, "/models/"+model+":generateContent")
			responseBody := `{"candidates":[{"content":{"role":"model","parts":[{"inlineData":{"mimeType":"audio/L16;codec=pcm;rate=24000","data":"YWJj"}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":5,"totalTokenCount":8,"candidatesTokensDetails":[{"modality":"AUDIO","tokenCount":5}]}}`
			usage, apiErr := a.DoResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(responseBody))}, info)
			require.Nil(t, apiErr)
			assert.JSONEq(t, responseBody, recorder.Body.String())
			assert.Equal(t, 8, usage.(*dto.Usage).TotalTokens)
			assert.Equal(t, 5, usage.(*dto.Usage).CompletionTokenDetails.AudioTokens)
		})
	}
}

func TestEmbeddingResponsesPreserveVectorsAndUpstreamUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    interface {
			DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError)
		}
		body string
	}{
		{name: "cloudflare", a: &cloudflare.Adaptor{}, body: `{"object":"list","model":"@cf/baai/bge-m3","data":[{"object":"embedding","index":0,"embedding":[0.2,0.3]}],"usage":{"prompt_tokens":9,"total_tokens":9}}`},
		{name: "jina", a: &jina.Adaptor{}, body: `{"object":"list","model":"jina-embeddings-v5-omni-small","data":[{"object":"embedding","index":0,"embedding":[0.2,0.3]}],"usage":{"total_tokens":9}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeEmbeddings, RelayFormat: types.RelayFormatOpenAI}
			info.ChannelMeta = &relaycommon.ChannelMeta{}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.body))}
			usage, apiErr := tc.a.DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			assert.Equal(t, 9, usage.(*dto.Usage).PromptTokens)
			var result map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
			assert.Equal(t, []any{map[string]any{"object": "embedding", "index": float64(0), "embedding": []any{0.2, 0.3}}}, result["data"])
		})
	}
}

func TestNativeOfflineTranscriptionContracts(t *testing.T) {
	for _, tc := range []struct {
		name, model, baseURL, wantURL string
		a                             interface {
			ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error)
			GetRequestURL(*relaycommon.RelayInfo) (string, error)
			DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError)
		}
	}{
		{name: "cohere", model: "cohere-transcribe-03-2026", baseURL: "https://api.cohere.com", wantURL: "https://api.cohere.com/compatibility/v1/audio/transcriptions", a: &cohere.Adaptor{}},
		{name: "mistral", model: "voxtral-mini-transcribe-3-0", baseURL: "https://api.mistral.ai", wantURL: "https://api.mistral.ai/v1/audio/transcriptions", a: &mistral.Adaptor{}},
		{name: "zhipu", model: "glm-asr-2512", baseURL: "https://open.bigmodel.cn", wantURL: "https://open.bigmodel.cn/api/paas/v4/audio/transcriptions", a: &zhipu_4v.Adaptor{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var original bytes.Buffer
			writer := multipart.NewWriter(&original)
			require.NoError(t, writer.WriteField("model", "client-alias"))
			require.NoError(t, writer.WriteField("language", "en"))
			file, err := writer.CreateFormFile("file", "clip.wav")
			require.NoError(t, err)
			_, err = file.Write([]byte("audio fixture"))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &original)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeAudioTranscription, RequestURLPath: "/v1/audio/transcriptions", RelayFormat: types.RelayFormatOpenAI}
			info.ChannelMeta = &relaycommon.ChannelMeta{}
			info.ChannelBaseUrl = tc.baseURL
			info.UpstreamModelName = tc.model
			converted, err := tc.a.ConvertAudioRequest(c, info, dto.AudioRequest{Model: tc.model})
			require.NoError(t, err)
			_, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
			require.NoError(t, err)
			form, err := multipart.NewReader(converted, params["boundary"]).ReadForm(1024)
			require.NoError(t, err)
			t.Cleanup(func() { _ = form.RemoveAll() })
			assert.Equal(t, []string{tc.model}, form.Value["model"])
			assert.Equal(t, []string{"en"}, form.Value["language"])
			require.Len(t, form.File["file"], 1)
			assert.Equal(t, "clip.wav", form.File["file"][0].Filename)
			url, err := tc.a.GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, tc.wantURL, url)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"text":"hello","usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`))}
			usage, apiErr := tc.a.DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			assert.Equal(t, 8, usage.(*dto.Usage).PromptTokens)
			assert.JSONEq(t, `{"text":"hello","usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`, recorder.Body.String())
		})
	}
}
