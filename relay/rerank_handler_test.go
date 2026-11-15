package relay

import (
	"testing"

	"github.com/stretchr/testify/assert"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Rerank providers commonly omit usage or report zero tokens. Settling that
// usage would refund the whole pre-charge, so the fallback bills the prompt
// estimate; reported usage is never overwritten.
func TestRerankBillingUsageFallsBackToEstimate(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	info.SetEstimatePromptTokens(37)

	cases := []struct {
		name  string
		usage any
		want  dto.Usage
	}{
		{"nil interface", nil, dto.Usage{PromptTokens: 37, TotalTokens: 37}},
		{"typed nil", (*dto.Usage)(nil), dto.Usage{PromptTokens: 37, TotalTokens: 37}},
		{"zero usage", &dto.Usage{}, dto.Usage{PromptTokens: 37, TotalTokens: 37}},
		{"prompt only", &dto.Usage{PromptTokens: 12}, dto.Usage{PromptTokens: 12, TotalTokens: 12}},
		{"total only", &dto.Usage{TotalTokens: 100}, dto.Usage{PromptTokens: 100, TotalTokens: 100}},
		{"reported usage", &dto.Usage{PromptTokens: 80, TotalTokens: 100}, dto.Usage{PromptTokens: 80, TotalTokens: 100}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rerankBillingUsage(tc.usage, info)
			assert.Equal(t, tc.want, *got)
		})
	}
}

// The adaptor's usage must not be mutated: it is the same value handed to the
// response writer.
func TestRerankBillingUsageCopiesReportedUsage(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	info.SetEstimatePromptTokens(37)
	reported := &dto.Usage{PromptTokens: 12}

	got := rerankBillingUsage(reported, info)

	assert.Equal(t, 12, got.PromptTokens)
	assert.Equal(t, 12, got.TotalTokens)
	assert.Equal(t, dto.Usage{PromptTokens: 12}, *reported)
}
