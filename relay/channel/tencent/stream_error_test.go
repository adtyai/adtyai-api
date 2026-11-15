package tencent

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTencentStreamTestContext() (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		UpstreamModelName: "hunyuan-test",
	}}
	info.IsStream = true
	info.SetEstimatePromptTokens(7)
	return c, recorder, info
}

func newTencentStreamTestResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// TestTencentStreamHandlerSurfacesUpstreamErrorFrames covers the SSE frame that carries
// an upstream error: it must fail the request instead of being pushed to the client as a
// normal chunk followed by [DONE] and a successful settlement.
func TestTencentStreamHandlerSurfacesUpstreamErrorFrames(t *testing.T) {
	c, recorder, info := newTencentStreamTestContext()
	body := "data: {\"Response\":{\"Error\":{\"Code\":4000,\"Message\":\"bad request\"}}}\n\n" +
		"data: [DONE]\n\n"

	usage, apiErr := tencentStreamHandler(c, info, newTencentStreamTestResponse(body))

	assert.Nil(t, usage)
	require.NotNil(t, apiErr, "an upstream error frame must surface as an error")
	assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "bad request")
	assert.NotContains(t, recorder.Body.String(), "[DONE]")
	assert.NotContains(t, recorder.Body.String(), `"object":"chat.completion.chunk"`)
}

func TestTencentStreamHandlerStreamsContentFrames(t *testing.T) {
	c, recorder, info := newTencentStreamTestContext()
	body := "data: {\"Choices\":[{\"Delta\":{\"Role\":\"assistant\",\"Content\":\"hello\"}}]}\n\n" +
		"data: {\"Choices\":[{\"Delta\":{\"Content\":\" world\"},\"FinishReason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	usage, apiErr := tencentStreamHandler(c, info, newTencentStreamTestResponse(body))

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 7, usage.PromptTokens)
	assert.Greater(t, usage.CompletionTokens, 0)
	assert.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)

	written := recorder.Body.String()
	assert.Contains(t, written, `"content":"hello"`)
	assert.Contains(t, written, `"content":" world"`)
	assert.Equal(t, 1, strings.Count(written, "[DONE]"))
}

// TestTencentStreamHandlerAbortsOnUnparsableStream covers the "bad frame treated as empty
// frame" behaviour: a stream that never parses must not spin to EOF and settle as success.
func TestTencentStreamHandlerAbortsOnUnparsableStream(t *testing.T) {
	c, _, info := newTencentStreamTestContext()
	frame := "data: {\"Response\":\n\n"
	// 远超「偶尔一个坏帧」的量级：这样的流不可能是一段可用的 SSE。
	body := strings.Repeat(frame, 6)

	usage, apiErr := tencentStreamHandler(c, info, newTencentStreamTestResponse(body))

	assert.Nil(t, usage)
	require.NotNil(t, apiErr, "a stream that never parses must not settle as success")
}

func TestTencentStreamHandlerToleratesOccasionalBadFrame(t *testing.T) {
	c, recorder, info := newTencentStreamTestContext()
	body := "data: {\"Response\":\n\n" +
		"data: {\"Choices\":[{\"Delta\":{\"Content\":\"hello\"}}]}\n\n" +
		"data: [DONE]\n\n"

	usage, apiErr := tencentStreamHandler(c, info, newTencentStreamTestResponse(body))

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Contains(t, recorder.Body.String(), `"content":"hello"`)
	assert.Contains(t, recorder.Body.String(), "[DONE]")
}
