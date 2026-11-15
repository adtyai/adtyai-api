package aws

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newNovaTestInfo(estimatePromptTokens int) *relaycommon.RelayInfo {
	info := newAwsTestRelayInfo()
	info.IsStream = false
	info.UpstreamModelName = "nova-pro-v1:0"
	info.SetEstimatePromptTokens(estimatePromptTokens)
	return info
}

func novaTestHTTPResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

// novaTestResult is what handleNovaRequest did with one canned upstream body.
type novaTestResult struct {
	recorder *httptest.ResponseRecorder
	usage    *dto.Usage
	response *dto.OpenAITextResponse
	failed   bool
	panicked bool
}

func runHandleNovaRequest(t *testing.T, upstreamBody string, estimatePromptTokens int) novaTestResult {
	t.Helper()
	gin.SetMode(gin.TestMode)

	client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		return novaTestHTTPResponse(request, upstreamBody), nil
	}))
	adaptor := &Adaptor{AwsClient: client, AwsReq: newAwsInvokeModelInput()}
	recorder := httptest.NewRecorder()
	c := newAwsTestContext(recorder, context.Background())
	info := newNovaTestInfo(estimatePromptTokens)

	result := novaTestResult{recorder: recorder}
	func() {
		defer func() {
			if recover() != nil {
				result.panicked = true
			}
		}()
		apiErr, usage := handleNovaRequest(c, info, adaptor)
		result.failed = apiErr != nil
		result.usage = usage
	}()

	if !result.panicked && recorder.Body.Len() > 0 {
		var decoded dto.OpenAITextResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &decoded))
		result.response = &decoded
	}
	return result
}

// TestHandleNovaRequestEmptyContentIsARefusal covers the legal Nova responses where
// output.message.content is empty or absent: they must not index into the slice.
func TestHandleNovaRequestEmptyContentIsARefusal(t *testing.T) {
	const estimate = 31
	tests := []struct {
		name string
		body string
	}{
		{
			name: "empty content array",
			body: `{"output":{"message":{"role":"assistant","content":[]}},"stopReason":"end_turn"}`,
		},
		{
			name: "missing content field",
			body: `{"output":{"message":{"role":"assistant"}},"stopReason":"guardrail_intervened"}`,
		},
		{
			name: "missing message and output",
			body: `{"stopReason":"guardrail_intervened"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runHandleNovaRequest(t, test.body, estimate)

			require.False(t, result.panicked, "empty upstream content must not panic")
			require.False(t, result.failed)
			assert.Equal(t, http.StatusOK, result.recorder.Code)
			require.NotNil(t, result.response)
			assert.Empty(t, result.response.Choices)
			assert.Contains(t, result.recorder.Body.String(), `"choices":[]`)

			// 空 content 时上游常常连 usage 一起省略；全零 usage 就是免费调用。
			require.NotNil(t, result.usage)
			assert.Equal(t, estimate, result.usage.PromptTokens)
			assert.Equal(t, estimate, result.usage.TotalTokens)
		})
	}
}

func TestHandleNovaRequestKeepsUpstreamUsageWhenPresent(t *testing.T) {
	const estimate = 31
	body := `{"output":{"message":{"role":"assistant","content":[{"text":"hello"}]}},` +
		`"usage":{"inputTokens":10,"outputTokens":4,"totalTokens":14}}`

	result := runHandleNovaRequest(t, body, estimate)

	require.False(t, result.panicked)
	require.False(t, result.failed)
	assert.Equal(t, http.StatusOK, result.recorder.Code)
	require.NotNil(t, result.response)
	require.Len(t, result.response.Choices, 1)
	assert.Equal(t, "assistant", result.response.Choices[0].Message.Role)
	assert.Equal(t, "hello", result.response.Choices[0].Message.Content)
	require.NotNil(t, result.usage)
	assert.Equal(t, 10, result.usage.PromptTokens)
	assert.Equal(t, 4, result.usage.CompletionTokens)
	assert.Equal(t, 14, result.usage.TotalTokens)
}

func TestHandleNovaRequestEstimatesUsageWhenUpstreamOmitsIt(t *testing.T) {
	const estimate = 31
	body := `{"output":{"message":{"role":"assistant","content":[{"text":"hello"}]}}}`

	result := runHandleNovaRequest(t, body, estimate)

	require.False(t, result.panicked)
	require.False(t, result.failed)
	require.NotNil(t, result.response)
	require.Len(t, result.response.Choices, 1)
	assert.Equal(t, "hello", result.response.Choices[0].Message.Content)
	require.NotNil(t, result.usage)
	assert.Equal(t, estimate, result.usage.PromptTokens)
	assert.Greater(t, result.usage.CompletionTokens, 0, "content must be counted even without upstream usage")
	assert.Equal(t, result.usage.PromptTokens+result.usage.CompletionTokens, result.usage.TotalTokens)
}
