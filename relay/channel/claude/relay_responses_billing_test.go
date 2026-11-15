package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An upstream error that arrives after Claude already streamed text into a
// Responses stream must still bill the delivered output: the error frame stops
// the scanner before message_delta, the only frame carrying output_tokens, so
// the request would otherwise settle on the message_start placeholder count.
func TestClaudeResponsesStreamFailureBillsDeliveredOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })

	body := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","model":"claude-sonnet-5","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":1}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"the first delivered paragraph of the answer"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" and a second delivered sentence."}}`,
		`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
	}, "\n\n") + "\n\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-5",
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RelayMode:       relayconstant.RelayModeResponses,
		IsStream:        true,
		DisablePing:     true,
		StartTime:       time.Now(),
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "claude-sonnet-5"},
	}
	info.SetEstimatePromptTokens(100)

	usage, apiErr := ClaudeResponsesStreamHandler(c, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)

	// The failure was delivered as a protocol event, not as an HTTP error body.
	assert.Contains(t, recorder.Body.String(), "response.failed")
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Greater(t, usage.CompletionTokens, 1, "delivered output must be estimated, not the message_start placeholder")

	require.NotNil(t, usage.BillingUsage)
	canonical, ok := usage.BillingUsage.CanonicalUsage()
	require.True(t, ok)
	assert.Equal(t, usage.CompletionTokens, canonical.CompletionTokens, "settlement must bill the estimated output")
}
