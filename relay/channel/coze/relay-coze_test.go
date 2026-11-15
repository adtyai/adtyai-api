package coze

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

func newCozeStreamContext(t *testing.T, sse string) (*gin.Context, *httptest.ResponseRecorder, *http.Response) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(sse)),
	}
	return c, recorder, resp
}

func cozeStreamInfo(estimatePrompt int) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "bot-model"}}
	info.SetEstimatePromptTokens(estimatePrompt)
	return info
}

// 上游把失败写成流里的一帧 `event: error`。它必须上抛：客户端不能收到 [DONE]，
// 这一单也不能按成功结算（ISS-R4-0120）。
func TestCozeStreamErrorEventIsRaised(t *testing.T) {
	sse := "event: conversation.message.delta\ndata: {\"content\":\"\\\"half\\\"\",\"type\":\"answer\"}\n\n" +
		"event: error\ndata: {\"code\":4000,\"msg\":\"bad bot\"}\n\n" +
		"event: conversation.chat.completed\ndata: {\"usage\":{\"input_count\":2,\"output_count\":3,\"token_count\":5}}\n\n"
	c, recorder, resp := newCozeStreamContext(t, sse)

	usage, apiErr := cozeChatStreamHandler(c, cozeStreamInfo(9), resp)

	require.NotNil(t, apiErr, "上游的错误帧不能降级成一行日志")
	assert.Contains(t, apiErr.Error(), "coze stream error")
	assert.Contains(t, apiErr.Error(), "4000")
	assert.Nil(t, usage, "失败不结算")
	body := recorder.Body.String()
	assert.NotContains(t, body, "[DONE]", "错误之后不能补 [DONE] 伪装成功")
	assert.NotContains(t, body, `"finish_reason":"stop"`, "错误之后不能再发 stop 帧")
	assert.Contains(t, body, "half", "已经吐出去的内容保持原样")
}

// 流式路径不经过 checkIfChatComplete，coze_input_count 从来没被设置过：
// 缺 usage 的兜底不能把 prompt 记成 0（ISS-R4-0120）。
func TestCozeStreamFallbackUsesEstimatedPromptTokens(t *testing.T) {
	sse := "event: conversation.message.delta\ndata: {\"content\":\"\\\"answer\\\"\",\"type\":\"answer\"}\n\n"
	c, recorder, resp := newCozeStreamContext(t, sse)

	usage, apiErr := cozeChatStreamHandler(c, cozeStreamInfo(9), resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 9, usage.PromptTokens, "coze_input_count 没设置时要退回通道估算的 prompt")
	assert.Positive(t, usage.TotalTokens)
	assert.Contains(t, recorder.Body.String(), "[DONE]")
}
