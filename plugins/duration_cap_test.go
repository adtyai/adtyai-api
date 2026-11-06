package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/require"
)

func TestResponsesRejectsIllegalVideoDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key   string
		model string
		body  map[string]any
	}{
		{"google", "veo", map[string]any{"model": "veo", "prompt": "a cat", "seconds": 3600}},
		{"hailuo", "MiniMax-Hailuo-02", map[string]any{"model": "MiniMax-Hailuo-02", "prompt": "a cat", "seconds": 3600}},
		{"sora", "sora", map[string]any{"model": "sora", "prompt": "a cat", "seconds": 99999}},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			source, err := builtinplugins.Source(tc.key)
			require.NoError(t, err)
			plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: tc.key})
			require.NoError(t, err)
			_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "decodeRequest"}, map[string]any{
				"model": tc.model,
				"body":  map[string]any{"kind": "json", "value": tc.body},
			})
			require.Error(t, err)
		})
	}
}
