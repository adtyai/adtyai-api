package perplexity

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentRoutingAndRequest(t *testing.T) {
	adaptor := &Adaptor{}
	for _, tc := range []struct {
		model   string
		mode    int
		want    string
		wantErr bool
	}{
		{"openai/gpt-6.1-sol", relayconstant.RelayModeResponses, "https://api.perplexity.ai/v1/agent", false},
		{"perplexity/sonar", relayconstant.RelayModeResponses, "https://api.perplexity.ai/v1/agent", false},
		{"openai/gpt-6.1-sol", relayconstant.RelayModeChatCompletions, "", true},
		{"sonar", relayconstant.RelayModeChatCompletions, "https://api.perplexity.ai/chat/completions", false},
		{"sonar", relayconstant.RelayModeResponses, "https://api.perplexity.ai/v1/responses", false},
	} {
		info := &relaycommon.RelayInfo{RelayMode: tc.mode, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.perplexity.ai", UpstreamModelName: tc.model}}
		url, err := adaptor.GetRequestURL(info)
		if tc.wantErr {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, tc.want, url)
	}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-6.1-sol"}}
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"openai/gpt-6.1-sol","input":[{"role":"user","content":"hi"},{"type":"function_call_output","call_id":"call_1","output":"ok","thought_signature":"opaque"}],"temperature":0,"top_p":0,"store":false,"stream":false,"stream_options":{"include_usage":false},"max_output_tokens":100,"tools":[{"type":"function","name":"lookup","strict":false,"parameters":{"type":"object"}}],"tool_choice":{"type":"function","name":"lookup"},"text":{"format":{"type":"json_schema","name":"answer","strict":false,"schema":{"type":"object"}}}}`), &request))
	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, request)
	require.NoError(t, err)
	data, err := common.Marshal(converted)
	require.NoError(t, err)
	var actual map[string]any
	require.NoError(t, common.Unmarshal(data, &actual))
	assert.Equal(t, float64(0), actual["temperature"])
	assert.Equal(t, float64(0), actual["top_p"])
	assert.Equal(t, false, actual["store"])
	assert.Equal(t, false, actual["stream"])
	assert.NotContains(t, actual, "stream_options")
	assert.NotContains(t, actual, "text")
	assert.JSONEq(t, `{"type":"json_schema","json_schema":{"name":"answer","strict":false,"schema":{"type":"object"}}}`, string(mustAgentJSON(t, actual["response_format"])))
	assert.JSONEq(t, `[{"type":"function","name":"lookup","strict":false,"parameters":{"type":"object"}}]`, string(mustAgentJSON(t, actual["tools"])))
	assert.JSONEq(t, `{"type":"function","name":"lookup"}`, string(mustAgentJSON(t, actual["tool_choice"])))
	assert.JSONEq(t, `[{"type":"message","role":"user","content":"hi"},{"type":"function_call_output","call_id":"call_1","output":"ok","thought_signature":"opaque"}]`, string(mustAgentJSON(t, actual["input"])))
	_, err = adaptor.ConvertOpenAIRequest(nil, info, &dto.GeneralOpenAIRequest{Model: "openai/gpt-6.1-sol"})
	require.Error(t, err)
	_, err = adaptor.ConvertClaudeRequest(nil, info, &dto.ClaudeRequest{})
	require.Error(t, err)
}

func TestAgentRequestRejectsUnsupportedAndUnboundedFields(t *testing.T) {
	for _, raw := range []string{
		`{"model":"openai/gpt-6.1-sol","input":"hi","max_output_tokens":0}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","max_output_tokens":1073741824}`,
		`{"model":"anthropic/claude-sonnet-5-5","input":"hi"}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","temperature":-1}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","top_p":1.1}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","parallel_tool_calls":false}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","text":{"format":{"type":"json_object"}}}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","tools":[{"type":"file_search","vector_store_ids":["vs_1"]}]}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var request dto.OpenAIResponsesRequest
			require.NoError(t, common.Unmarshal([]byte(raw), &request))
			_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: request.Model}}, request)
			require.Error(t, err)
		})
	}
	maxTokens := uint(math.MaxInt32 / 2)
	_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-6.1-sol"}}, dto.OpenAIResponsesRequest{Model: "openai/gpt-6.1-sol", Input: []byte(`"hi"`), MaxOutputTokens: &maxTokens})
	require.NoError(t, err)
}

func TestAgentResponsesUsageAndOutput(t *testing.T) {
	body := `{"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"openai/gpt-6.1-sol","output":[{"type":"function_call","id":"fc_1","status":"completed","name":"lookup","call_id":"call_1","arguments":"{}","thought_signature":"opaque"},{"type":"search_results","results":[{"id":1,"url":"https://example.com"}]}],"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,"input_tokens_details":{"cache_read_input_tokens":30,"cache_creation_input_tokens":10},"output_tokens_details":{"reasoning_tokens":5}}}`
	c, recorder, info := agentResponseContext(false)
	value, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": {"application/json"}}}, info)
	require.Nil(t, apiErr)
	usage, ok := value.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 20, usage.CompletionTokens)
	assert.Equal(t, 120, usage.TotalTokens)
	assert.Equal(t, 30, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 10, usage.PromptTokensDetails.CacheCreationTokensTotal())
	assert.Equal(t, 5, usage.CompletionTokenDetails.ReasoningTokens)
	var forwarded map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &forwarded))
	assert.Contains(t, recorder.Body.String(), `"thought_signature":"opaque"`)
	assert.Contains(t, recorder.Body.String(), `"type":"search_results"`)
	assert.Contains(t, recorder.Body.String(), `"cached_tokens":30`)
}

func TestAgentStreamUsageAndErrors(t *testing.T) {
	previous := constant.StreamingTimeout
	constant.StreamingTimeout = 10
	t.Cleanup(func() { constant.StreamingTimeout = previous })
	for _, tc := range []struct {
		name                         string
		body                         string
		prompt, output, cache, write int
		wantErr                      bool
		outcome                      string
	}{
		{"usage", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"openai/gpt-6.1-sol\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":100,\"output_tokens\":20,\"total_tokens\":120,\"input_tokens_details\":{\"cache_read_input_tokens\":30,\"cache_creation_input_tokens\":10}}}}\n\n", 100, 20, 30, 10, false, "completed"},
		{"explicit zero usage", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":0,\"output_tokens\":0,\"total_tokens\":0}}}\n\n", 0, 0, 0, 0, false, "completed"},
		{"failure", "data: {\"type\":\"response.failed\",\"error\":{\"code\":\"bad_tool\",\"message\":\"tool unavailable\"}}\n\n", 0, 0, 0, 0, true, "failed"},
		{"malformed", "data: {invalid}\n\n", 0, 0, 0, 0, true, "failed"},
		{"terminal missing usage", "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[]}}\n\n", 0, 0, 0, 0, true, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, recorder, info := agentResponseContext(true)
			info.SetEstimatePromptTokens(99)
			value, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Content-Type": {"text/event-stream"}}}, info)
			if tc.wantErr {
				require.NotNil(t, apiErr)
				assert.NotContains(t, recorder.Body.String(), "response.completed")
				return
			}
			require.Nil(t, apiErr)
			usage := value.(*dto.Usage)
			assert.Equal(t, tc.prompt, usage.PromptTokens)
			assert.Equal(t, tc.output, usage.CompletionTokens)
			assert.Equal(t, tc.cache, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, tc.write, usage.PromptTokensDetails.CacheCreationTokensTotal())
			assert.Equal(t, tc.outcome, info.StreamStatus.ResponseOutcome())
			assert.Contains(t, recorder.Body.String(), "event: response.completed")
			assert.Contains(t, recorder.Body.String(), "response.output_text.delta")
		})
	}
}

func TestAgentResponseRejectsErrorsAndInvalidUsage(t *testing.T) {
	for _, body := range []string{
		`{"id":"resp_1","status":"failed","error":{"message":"upstream failed"},"output":[]}`,
		`{"id":"resp_1","status":"queued","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`,
		`{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":-1,"output_tokens":1,"total_tokens":0}}`,
		`{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":100,"output_tokens":1,"total_tokens":101,"input_tokens_details":{"cache_read_input_tokens":101}}}`,
	} {
		c, recorder, info := agentResponseContext(false)
		_, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, info)
		require.NotNil(t, apiErr)
		assert.Empty(t, recorder.Body.String())
	}
}

func TestAgentFinalRequestValidatesPassthroughAndOverrides(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Equal(t, "/v1/agent", r.URL.Path)
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"openai/gpt-6.1-sol","input":"hi","temperature":0,"top_p":0,"store":false,"max_output_tokens":100,"response_format":{"type":"json_schema","json_schema":{"name":"answer","strict":false,"schema":{"type":"object"}}}}`, string(data))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	adaptor := &Adaptor{}
	for _, body := range []string{
		`{"model":"openai/gpt-6.1-sol","input":"hi","max_output_tokens":18446744073686646784}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","max_output_tokens":null}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","max_steps":501}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","tools":[{"type":"web_search","max_tokens":18446744073686646784}]}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","tool_choice":{"type":"web_search"}}`,
		`{"model":"openai/gpt-6.1-sol","input":"hi","background":true}`,
	} {
		c, _, info := agentResponseContext(false)
		info.ChannelBaseUrl = server.URL
		_, err := adaptor.DoRequest(c, info, strings.NewReader(body))
		require.Error(t, err, body)
	}
	assert.Zero(t, requests)
	c, _, info := agentResponseContext(false)
	info.ChannelBaseUrl = server.URL
	body := `{"model":"openai/gpt-6.1-sol","input":"hi","temperature":0,"top_p":0,"store":false,"max_output_tokens":100,"stream_options":{"include_usage":false},"text":{"format":{"type":"json_schema","name":"answer","strict":false,"schema":{"type":"object"}}}}`
	result, err := adaptor.DoRequest(c, info, strings.NewReader(body))
	require.NoError(t, err)
	response, ok := result.(*http.Response)
	require.True(t, ok)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, 1, requests)
	info.RelayFormat = types.RelayFormatOpenAI
	_, err = adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{Model: "openai/gpt-6.1-sol", Input: []byte(`"hi"`)})
	require.Error(t, err)
}

func agentResponseContext(stream bool) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses, RelayFormat: types.RelayFormatOpenAIResponses, IsStream: stream, DisablePing: true, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-6.1-sol"}}
	return c, recorder, info
}

func mustAgentJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}
