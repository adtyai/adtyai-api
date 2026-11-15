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

// 守 `ISS-R4-0071`：**上游 usage 的两个字段相减不得产生负数 completion。**
//
// 修前 `completion = total - prompt` 没有下限。温和情形（total 略小于 prompt）用负数
// 冲抵已经完成的那部分输出；极端情形（total = 0）把 `prompt + completion` 抹成 0，
// `service/text_quota.go` 的 `hasBillableUsage()` 判成无可计费用量，整单免费。
//
// 修法：矛盾时以 prompt 为准（total 抬到 prompt、completion 记 0）。
func TestXAIStreamHandlerDoesNotBillNegativeCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	cases := []struct {
		name         string
		promptTokens int
		totalTokens  int
	}{
		// 温和情形：total 比 prompt 少 100。
		{name: "total slightly below prompt", promptTokens: 1000, totalTokens: 900},
		// 极端情形：上游漏报 total，整单会被冲成 0。
		{name: "total zero", promptTokens: 1000, totalTokens: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

			info := &relaycommon.RelayInfo{
				OriginModelName: "grok-3",
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "grok-3",
				},
			}

			content := "hi"
			chunkData, err := common.Marshal(dto.ChatCompletionsStreamResponse{
				Id:    "chatcmpl-xai-contradictory",
				Model: "grok-3",
				Choices: []dto.ChatCompletionsStreamResponseChoice{{
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &content},
				}},
				Usage: &dto.Usage{
					PromptTokens: tc.promptTokens,
					TotalTokens:  tc.totalTokens,
				},
			})
			require.NoError(t, err)

			resp := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n"))),
			}

			usage, newAPIError := xAIStreamHandler(c, info, resp)
			require.Nil(t, newAPIError)
			require.NotNil(t, usage)

			require.GreaterOrEqual(t, usage.CompletionTokens, 0,
				"上游自相矛盾时 completion 不得为负（ISS-R4-0071）")
			require.Equal(t, tc.promptTokens, usage.PromptTokens)
			// 以 prompt 为准：total 抬到 prompt，completion 记 0。
			require.Equal(t, tc.promptTokens, usage.TotalTokens)
			require.Zero(t, usage.CompletionTokens)
			// 计费汇总重算的 total 必须仍是正数，否则整单免费。
			require.Positive(t, usage.PromptTokens+usage.CompletionTokens,
				"矛盾回包不能让整单变成无可计费用量（ISS-R4-0071）")
		})
	}
}

// 非流式走的是同一处相减（`xAIHandler`），一并守。修前这里同样能把
// completion 算成负数，只是没有 containStreamUsage 那条「关掉估算兜底」的副作用。
func TestXAIHandlerDoesNotBillNegativeCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-3",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "grok-3",
		},
	}

	body := []byte(`{"id":"chatcmpl-xai-contradictory","object":"chat.completion","model":"grok-3","choices":[],"usage":{"prompt_tokens":1000,"total_tokens":0}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := xAIHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)

	require.GreaterOrEqual(t, usage.CompletionTokens, 0, "上游自相矛盾时 completion 不得为负（ISS-R4-0071）")
	require.Equal(t, 1000, usage.PromptTokens)
	require.Equal(t, 1000, usage.TotalTokens)
	require.Zero(t, usage.CompletionTokens)
	require.Positive(t, usage.PromptTokens+usage.CompletionTokens)
}
