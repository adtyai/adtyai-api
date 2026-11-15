package openai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 这条守的是 `ISS-R4-0103` 同批的 `ISS-R4-0110`：兜底估算不能裸 cast。
//
// 同一函数里 `:107`（能拿到时长那条）用的是 `common.QuotaRound`，4 行之外的兜底
// 分支却写 `int(math.Ceil(sizeInKB))`——billing.md:14 明令禁止的写法。对正常响应体
// 两者数值相同，差别只在越界：裸 cast 会给出一个超出 int32 的 quota，QuotaRound
// 饱和到 `common.MaxQuota` 并留下 QuotaClamp 痕迹。
func TestEstimatedAudioTokensFromSizeKBSaturates(t *testing.T) {
	require.Equal(t, 1, estimatedAudioTokensFromSizeKB(1), "估算口径不变：每 KB 1 token")
	require.Equal(t, 3, estimatedAudioTokensFromSizeKB(2.1), "仍然向上取整")
	// 3e9 KB 已超过单请求 quota 上限：裸 int cast 会给 3000000000。
	require.Equal(t, common.MaxQuota, estimatedAudioTokensFromSizeKB(3e9),
		"兜底估算必须走 common.QuotaRound 的饱和口径（ISS-R4-0110）")
}

// 走一遍真实的兜底链路：非流式 TTS 响应拿不到可解析的音频头时，
// `GetAudioDuration` 报错 → usage 用响应体大小兜底估算。
func TestOpenaiTTSFallbackEstimatesFromBodySize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)

	body := bytes.Repeat([]byte("not-a-wav-header"), 256) // 4096 字节，解析不出时长
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	info := &relaycommon.RelayInfo{
		IsStream: false,
		Request:  &dto.AudioRequest{ResponseFormat: "wav"},
	}

	usage := OpenaiTTSHandler(c, resp, info)
	require.NotNil(t, usage)
	require.Equal(t, estimatedAudioTokensFromSizeKB(4096.0/1000.0), usage.CompletionTokens)
	require.Equal(t, usage.CompletionTokens, usage.CompletionTokenDetails.AudioTokens)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
}
