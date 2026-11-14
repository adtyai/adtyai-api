package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestEnsureResponsesUsageReasoningDetailsCopiesTopLevelReasoning(t *testing.T) {
	payload := []byte(`{"id":"resp_1","created_at":1786588600.0,"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"reasoning_tokens":8}}`)
	patched := ensureResponsesUsageReasoningDetails(payload, "usage")
	assert.Equal(t, int64(8), gjson.GetBytes(patched, "usage.output_tokens_details.reasoning_tokens").Int())
	assert.Equal(t, int64(8), gjson.GetBytes(patched, "usage.reasoning_tokens").Int())
	assert.Equal(t, int64(20), gjson.GetBytes(patched, "usage.output_tokens").Int())
	assert.Equal(t, int64(30), gjson.GetBytes(patched, "usage.total_tokens").Int())
	// Narrow patch must leave a non-integer created_at untouched.
	assert.Equal(t, 1786588600.0, gjson.GetBytes(patched, "created_at").Float())
}

func TestEnsureResponsesUsageReasoningDetailsLeavesExistingDetails(t *testing.T) {
	payload := []byte(`{"usage":{"reasoning_tokens":8,"output_tokens_details":{"reasoning_tokens":3,"text_tokens":5}}}`)
	patched := ensureResponsesUsageReasoningDetails(payload, "usage")
	assert.Equal(t, int64(3), gjson.GetBytes(patched, "usage.output_tokens_details.reasoning_tokens").Int())
	assert.Equal(t, int64(5), gjson.GetBytes(patched, "usage.output_tokens_details.text_tokens").Int())
}
