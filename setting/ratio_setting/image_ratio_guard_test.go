package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRatioBadJSONKeepsPreviousMap(t *testing.T) {
	require.NoError(t, UpdateImageRatioByJSONString(`{"gpt-image-1":2}`))
	t.Cleanup(func() { _ = UpdateImageRatioByJSONString(`{}`) })

	require.Error(t, UpdateImageRatioByJSONString(`{`))
	ratio, ok := GetImageRatio("gpt-image-1")
	require.True(t, ok)
	assert.Equal(t, 2.0, ratio)

	require.Error(t, UpdateAudioRatioByJSONString(`{`))
	require.NoError(t, UpdateAudioRatioByJSONString(`{"whisper-1":1.5}`))
	audio, ok := audioRatioMap.Get("whisper-1")
	require.True(t, ok)
	assert.Equal(t, 1.5, audio)
	require.Error(t, UpdateAudioRatioByJSONString(`[`))
	audio, ok = audioRatioMap.Get("whisper-1")
	require.True(t, ok)
	assert.Equal(t, 1.5, audio)
}
