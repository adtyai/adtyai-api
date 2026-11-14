package xai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestXAIStreamHandlerPreservesTokenDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-3",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "grok-3",
		},
	}

	content := "hi"
	chunkData, err := common.Marshal(dto.ChatCompletionsStreamResponse{
		Id:    "chatcmpl-xai-cache",
		Model: "grok-3",
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &content},
		}},
		Usage: &dto.Usage{
			PromptTokens:     100,
			CompletionTokens: 20,
			TotalTokens:      120,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 40,
			},
			CompletionTokenDetails: dto.OutputTokenDetails{
				ReasoningTokens: 5,
				TextTokens:      15,
			},
		},
	})
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader([]byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n"))),
	}

	usage, newAPIError := xAIStreamHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 120, usage.TotalTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 40, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 5, usage.CompletionTokenDetails.ReasoningTokens)
	require.Equal(t, 15, usage.CompletionTokenDetails.TextTokens)
}
