package ali

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rerank response without a usage block must still be billed: results are
// already delivered to the client, so a zero token count would settle the
// request for free. A reported count is never replaced by the estimate.
func TestRerankResponseWithoutUsageBillsTheEstimate(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		estimate int
		want     int
	}{
		{
			name:     "no usage bills the estimate",
			body:     `{"output":{"results":[{"index":1,"relevance_score":0.9}]},"request_id":"r1"}`,
			estimate: 42,
			want:     42,
		},
		{
			name:     "reported usage is kept",
			body:     `{"output":{"results":[{"index":1,"relevance_score":0.9}]},"usage":{"total_tokens":100},"request_id":"r2"}`,
			estimate: 42,
			want:     100,
		},
		{
			name:     "negative usage falls back to the estimate",
			body:     `{"output":{"results":[{"index":1,"relevance_score":0.9}]},"usage":{"total_tokens":-7},"request_id":"r3"}`,
			estimate: 42,
			want:     42,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			info := &relaycommon.RelayInfo{
				RelayMode:   constant.RelayModeRerank,
				RelayFormat: types.RelayFormatOpenAI,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gte-rerank"},
			}
			info.SetEstimatePromptTokens(tt.estimate)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tt.body))}

			rawUsage, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			usage, ok := rawUsage.(*dto.Usage)
			require.True(t, ok)
			wantUsage := dto.Usage{PromptTokens: tt.want, CompletionTokens: 0, TotalTokens: tt.want}
			require.NotNil(t, usage)
			assert.Equal(t, wantUsage, *usage)

			var got dto.RerankResponse
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, wantUsage, got.Usage)
			assert.Equal(t, []dto.RerankResponseResult{{Index: 1, RelevanceScore: 0.9}}, got.Results)
		})
	}
}
