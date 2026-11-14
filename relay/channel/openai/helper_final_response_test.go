package openai

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleFinalResponseFinalizesClaudeStreamWithoutUsageChunk(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
		RelayFormat: types.RelayFormatClaude,
	}

	lastStreamData := `{"id":"c1","model":"gpt-test","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}`
	usage := &dto.Usage{PromptTokens: 4, CompletionTokens: 1, TotalTokens: 5}

	HandleFinalResponse(c, info, lastStreamData, "c1", 1700000000, "gpt-test", "", usage, false)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: message_delta")
	assert.Contains(t, body, "event: message_stop")
	require.NotNil(t, info.ClaudeConvertInfo)
	assert.True(t, info.ClaudeConvertInfo.Done)
	require.NotNil(t, info.ClaudeConvertInfo.Usage)
	assert.Equal(t, 4, info.ClaudeConvertInfo.Usage.PromptTokens)
	assert.Equal(t, 1, info.ClaudeConvertInfo.Usage.CompletionTokens)
}

func TestHandleFinalResponseClaudeFinalizeIsIdempotentAfterUsageChunk(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
		RelayFormat: types.RelayFormatClaude,
	}

	// Prior content chunk leaves an open text block, matching a real stream.
	require.NoError(t, handleClaudeFormat(c, `{"id":"c1","model":"gpt-test","choices":[{"index":0,"delta":{"content":"hi"}}]}`, info))
	assert.False(t, info.ClaudeConvertInfo.Done)

	lastStreamData := `{"id":"c1","model":"gpt-test","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`
	usage := &dto.Usage{PromptTokens: 4, CompletionTokens: 1, TotalTokens: 5}
	HandleFinalResponse(c, info, lastStreamData, "c1", 1700000000, "gpt-test", "", usage, true)

	body := recorder.Body.String()
	assert.Equal(t, 1, countSubstring(body, "event: message_stop"))
	assert.True(t, info.ClaudeConvertInfo.Done)
}

func countSubstring(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}
