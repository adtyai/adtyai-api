package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserLogsHideChannelAndRatios(t *testing.T) {
	other := `{"cache_tokens":9007199254740993,"model_ratio":2,"group_ratio":1.5,"completion_ratio":4,` +
		`"model_price":0,"upstream_model_name":"gpt-real","is_model_mapped":true,"expr_b64":"eA==",` +
		`"matched_tier":"t1","seconds":5,"request_path":"/v1/chat/completions",` +
		`"tool_surcharges":[{"name":"web_search","count":2,"price":0.01}],` +
		`"stream_status":{"status":"error","end_reason":"timeout","end_error":"dial https://up.example"},` +
		`"admin_info":{"use_channel":["3"]}}`
	log := &Log{ChannelId: 3, ChannelName: "secret", UpstreamRequestId: "up-1", Other: other}

	AdtyProjectUserLog(log)

	assert.Zero(t, log.ChannelId)
	assert.Empty(t, log.ChannelName)
	assert.Empty(t, log.UpstreamRequestId)
	assert.Contains(t, log.Other, `"cache_tokens":9007199254740993`, "large integers stay exact")
	parsed, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"cache_tokens", "request_path", "tool_surcharges", "stream_status", "billing_mode"}, keysOf(parsed))
	assert.Equal(t, []any{map[string]any{"name": "web_search", "count": float64(2)}}, parsed["tool_surcharges"])
	assert.Equal(t, map[string]any{"status": "error", "end_reason": "timeout"}, parsed["stream_status"])
}

func TestPerCallMarkerKept(t *testing.T) {
	cases := []struct {
		other, mode string
	}{
		{`{"model_price":0.04}`, "per_call"},
		{`{"model_price":0,"model_ratio":2}`, "per_token"},
		{`{}`, "per_token"},
		{`{"billing_mode":"tiered_expr","model_price":0.04}`, "tiered_expr"},
	}
	for _, tc := range cases {
		log := &Log{Other: tc.other}
		AdtyProjectUserLog(log)
		parsed, err := common.StrToMap(log.Other)
		require.NoError(t, err)
		assert.Equal(t, tc.mode, parsed["billing_mode"], tc.other)
		assert.NotContains(t, parsed, "model_price", tc.other)
	}
}

func TestAdminLogsStillShowChannel(t *testing.T) {
	other := `{"model_ratio":2,"admin_info":{"use_channel":["3"]}}`
	logs := []*Log{{ChannelId: 3, Other: other}}
	FormatAdminLogs(logs)
	assert.Equal(t, 3, logs[0].ChannelId)
	assert.Contains(t, logs[0].Other, "model_ratio")
	assert.Contains(t, logs[0].Other, "admin_info")
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
