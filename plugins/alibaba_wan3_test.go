package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlibabaWan3(t *testing.T) {
	source, err := builtinplugins.Source("alibaba")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "alibaba"})
	require.NoError(t, err)
	for _, model := range []string{"wan2.1-t2i-plus", "wan2.1-t2i-turbo"} {
		t.Run(model, func(t *testing.T) {
			_, found := registry.Generation().LookupEndpoint("POST", "/v1/images/generations", model)
			require.True(t, found)
			body, facts, url := submitAlibabaRequest(t, plugin, model, map[string]any{"model": model, "prompt": "a cat"})
			assert.Equal(t, model, body["model"])
			assert.Equal(t, "https://dashscope.aliyuncs.com/api/v1/services/aigc/text2image/image-synthesis", url)
			// The legacy API defaults n to four; reserve the actual outgoing count.
			assert.Equal(t, float64(4), body["parameters"].(map[string]any)["n"])
			assert.Equal(t, float64(4), facts["image_count"])
		})
	}

	roundTrip := func(t *testing.T, value any) map[string]any {
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var decoded map[string]any
		require.NoError(t, common.Unmarshal(encoded, &decoded))
		return decoded
	}
	submitCtx := func(model, upstream string, body map[string]any) map[string]any {
		return map[string]any{"model": model, "upstreamModel": upstream, "baseUrl": "https://dashscope.aliyuncs.com", "apiKey": "k", "requestBody": body}
	}
	usageCtx := func(purpose string, body map[string]any) map[string]any {
		return map[string]any{"model": "wan3.0-video", "upstreamModel": "wan3.0-video", "usagePurpose": purpose, "requestBody": body}
	}
	decodeResponses := func(model string, body map[string]any) (map[string]any, error) {
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "decodeRequest"}, map[string]any{"model": model, "body": map[string]any{"kind": "json", "value": body}, "stream": false})
		if callErr != nil {
			return nil, callErr
		}
		return roundTrip(t, value), nil
	}

	t.Run("duration -1 becomes the auto_duration marker in every decoder", func(t *testing.T) {
		resolved, callErr := decodeResponses("wan3.0-video", map[string]any{"model": "wan3.0-video", "input": "a cat", "duration": -1})
		require.NoError(t, callErr)
		requestBody := resolved["requestBody"].(map[string]any)
		assert.Equal(t, true, requestBody["auto_duration"])
		assert.NotContains(t, requestBody, "duration")

		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "wan3.0-video", "body": map[string]any{"kind": "json", "value": map[string]any{"model": "wan3.0-video", "prompt": "a cat", "seconds": -1}}})
		require.NoError(t, callErr)
		requestBody = roundTrip(t, value)["requestBody"].(map[string]any)
		assert.Equal(t, true, requestBody["auto_duration"])
		assert.NotContains(t, requestBody, "seconds")
		assert.NotContains(t, requestBody, "duration")

		value, callErr = plugin.Engine.CallPath(t.Context(), "native", []string{"createVideoTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": map[string]any{
			"model": "wan3.0-video", "input": map[string]any{"prompt": "a cat"}, "parameters": map[string]any{"duration": -1, "ratio": "16:9"},
		}}})
		require.NoError(t, callErr)
		requestBody = roundTrip(t, value)["requestBody"].(map[string]any)
		assert.Equal(t, true, requestBody["auto_duration"])
		assert.NotContains(t, requestBody, "duration")
		parameters := requestBody["metadata"].(map[string]any)["parameters"].(map[string]any)
		assert.Equal(t, "16:9", parameters["ratio"])
		assert.NotContains(t, parameters, "duration")
	})

	t.Run("auto_duration submits -1 upstream and bills 30 seconds up front", func(t *testing.T) {
		body := map[string]any{"model": "wan3.0-video", "prompt": "a cat", "auto_duration": true}
		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", body))
		require.NoError(t, callErr)
		parameters := roundTrip(t, value)["body"].(map[string]any)["parameters"].(map[string]any)
		assert.Equal(t, float64(-1), parameters["duration"])

		value, callErr = plugin.Engine.Call(t.Context(), "extractUsage", usageCtx("facts", body))
		require.NoError(t, callErr)
		assert.Equal(t, map[string]any{"seconds": float64(30), "resolution": "1080P"}, roundTrip(t, value))

		value, callErr = plugin.Engine.Call(t.Context(), "extractUsage", usageCtx("billing_ratios", body))
		require.NoError(t, callErr)
		assert.Equal(t, float64(30), roundTrip(t, value)["seconds"])

		_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan2.7-t2v", "wan2.7-t2v", map[string]any{"model": "wan2.7-t2v", "prompt": "a cat", "auto_duration": true}))
		require.ErrorContains(t, callErr, "only supported by wan3.0")
	})

	// /v1/video/generations has no plugin decoder, so -1 reaches the plugin as
	// a vendor parameter in metadata.
	t.Run("metadata parameters keep -1 on /v1/video/generations", func(t *testing.T) {
		body, facts, _ := submitAlibabaRequest(t, plugin, "wan3.0-video", map[string]any{
			"model": "wan3.0-video", "prompt": "a cat", "metadata": map[string]any{"parameters": map[string]any{"duration": -1}},
		})
		assert.Equal(t, float64(-1), body["parameters"].(map[string]any)["duration"])
		assert.Equal(t, float64(30), facts["seconds"])
	})

	t.Run("channel-mapped alias resolves defaults from the upstream model", func(t *testing.T) {
		direct, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "a cat"}))
		require.NoError(t, callErr)
		alias, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("my-wan3", "wan3.0-video", map[string]any{"model": "my-wan3", "prompt": "a cat"}))
		require.NoError(t, callErr)
		assert.Equal(t, roundTrip(t, direct)["body"], roundTrip(t, alias)["body"])
		assert.Equal(t, "1080P", roundTrip(t, alias)["body"].(map[string]any)["parameters"].(map[string]any)["resolution"])
	})

	t.Run("size maps to a resolution tier and unknown sizes are rejected", func(t *testing.T) {
		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "a cat", "size": "1280*720"}))
		require.NoError(t, callErr)
		parameters := roundTrip(t, value)["body"].(map[string]any)["parameters"].(map[string]any)
		assert.Equal(t, "720P", parameters["resolution"])
		assert.NotContains(t, parameters, "size")
		assert.Equal(t, "16:9", parameters["ratio"])

		_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "a cat", "size": "1000*1000"}))
		require.ErrorContains(t, callErr, "invalid size")
		_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "a cat", "duration": 31}))
		require.ErrorContains(t, callErr, "between 2 and 30")
	})

	t.Run("image-only input stays rejected for t2v models and accepted for wan3.0", func(t *testing.T) {
		imageOnly := []any{map[string]any{"type": "input_image", "image_url": "https://cdn.example/first.png"}}
		_, callErr := decodeResponses("wan2.7-t2v", map[string]any{"model": "wan2.7-t2v", "input": imageOnly})
		require.ErrorContains(t, callErr, "input is required")

		resolved, callErr := decodeResponses("wan3.0-video", map[string]any{"model": "wan3.0-video", "input": imageOnly})
		require.NoError(t, callErr)
		assert.Equal(t, "image_to_video", resolved["action"])

		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "", "images": []any{"https://cdn.example/first.png"}}))
		require.NoError(t, callErr)
		input := roundTrip(t, value)["body"].(map[string]any)["input"].(map[string]any)
		assert.Equal(t, []any{map[string]any{"type": "first_frame", "url": "https://cdn.example/first.png"}}, input["media"])
		assert.NotContains(t, input, "img_url")
	})

	t.Run("completion facts read the wan3.0 usage block", func(t *testing.T) {
		value, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{}, map[string]any{}, map[string]any{
			"output": map[string]any{"task_status": "SUCCEEDED", "video_url": "https://upstream.example/v.mp4"},
			"usage":  map[string]any{"video_count": 1, "duration": 7.5, "output_video_duration": 7.5, "SR": 720, "ratio": "16:9"},
		})
		require.NoError(t, callErr)
		assert.Equal(t, map[string]any{"seconds": 7.5, "resolution": "720P"}, roundTrip(t, value))

		value, callErr = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{}, map[string]any{}, map[string]any{
			"output": map[string]any{"task_status": "SUCCEEDED", "duration": 5, "resolution": "1080p"},
		})
		require.NoError(t, callErr)
		assert.Equal(t, map[string]any{"seconds": float64(5), "resolution": "1080P"}, roundTrip(t, value))
	})
}

func TestAlibabaAdditionalVideoModels(t *testing.T) {
	source, err := builtinplugins.Source("alibaba")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "alibaba"})
	require.NoError(t, err)
	for _, tc := range []struct {
		model      string
		input      map[string]any
		parameters map[string]any
		service    string
		seconds    float64
	}{
		{"happyhorse-1.0-t2v", map[string]any{"prompt": "a horse"}, map[string]any{"ratio": "21:9", "duration": 3}, "video-generation", 3},
		{"happyhorse-1.1-t2v", map[string]any{"prompt": "a horse"}, map[string]any{"resolution": "480P", "duration": 15}, "video-generation", 15},
		{"happyhorse-1.0-i2v", map[string]any{"media": []any{map[string]any{"type": "first_frame", "url": "https://cdn.example/a.png"}}}, nil, "video-generation", 5},
		{"happyhorse-1.1-i2v", map[string]any{"media": []any{map[string]any{"type": "first_frame", "url": "https://cdn.example/a.png"}}}, nil, "video-generation", 5},
		{"happyhorse-1.0-r2v", map[string]any{"prompt": "[Image 1] runs", "media": []any{map[string]any{"type": "reference_image", "url": "https://cdn.example/a.png"}}}, nil, "video-generation", 5},
		{"happyhorse-1.1-r2v", map[string]any{"prompt": "[Image 1] runs", "media": []any{map[string]any{"type": "reference_image", "url": "https://cdn.example/a.png"}}}, nil, "video-generation", 5},
		{"happyhorse-1.0-video-edit", map[string]any{"prompt": "change the coat", "media": []any{map[string]any{"type": "video", "url": "https://cdn.example/a.mp4"}}}, nil, "video-generation", 120},
		{"wan2.7-videoedit", map[string]any{"media": []any{map[string]any{"type": "video", "url": "https://cdn.example/a.mp4"}}}, map[string]any{"duration": 0, "audio_setting": "origin"}, "video-generation", 20},
		{"wan2.7-r2v-2026-06-12", map[string]any{"prompt": "Video 1 runs", "media": []any{map[string]any{"type": "reference_video", "url": "https://cdn.example/a.mp4"}}}, nil, "video-generation", 35},
		{"wan2.6-r2v", map[string]any{"prompt": "character1 runs", "reference_urls": []any{"https://cdn.example/a.mp4"}}, nil, "video-generation", 35},
		{"wan2.6-r2v-flash", map[string]any{"prompt": "character1 runs", "reference_urls": []any{"https://cdn.example/a.mp4"}}, nil, "video-generation", 35},
		{"wan2.2-animate-move", map[string]any{"image_url": "https://cdn.example/a.png", "video_url": "https://cdn.example/a.mp4", "watermark": false}, map[string]any{"mode": "wan-pro", "check_image": false}, "image2video", 30},
		{"wan2.2-animate-mix", map[string]any{"image_url": "https://cdn.example/a.png", "video_url": "https://cdn.example/a.mp4"}, map[string]any{"mode": "wan-std"}, "image2video", 30},
		{"wan2.1-vace-plus", map[string]any{"prompt": "a cat", "function": "image_reference", "ref_images_url": []any{"https://cdn.example/a.png"}}, nil, "video-generation", 5},
		{"wan2.1-kf2v-plus", map[string]any{"first_frame_url": "https://cdn.example/a.png", "last_frame_url": "https://cdn.example/b.png"}, nil, "image2video", 5},
		{"wan2.1-t2v-plus", map[string]any{"prompt": "a cat"}, nil, "video-generation", 5},
		{"wan2.1-t2v-turbo", map[string]any{"prompt": "a cat"}, nil, "video-generation", 5},
		{"wan2.1-i2v-plus", map[string]any{"img_url": "https://cdn.example/a.png"}, nil, "video-generation", 5},
		{"wan2.1-i2v-turbo", map[string]any{"img_url": "https://cdn.example/a.png"}, nil, "video-generation", 5},
	} {
		t.Run(tc.model, func(t *testing.T) {
			binding, found := registry.Generation().LookupEndpoint("POST", "/v1/videos", tc.model)
			require.True(t, found)
			assert.Same(t, plugin, binding.Plugin)
			body, facts, url := submitAlibabaRequest(t, plugin, tc.model, map[string]any{"model": tc.model, "metadata": map[string]any{"input": tc.input, "parameters": tc.parameters}})
			assert.Equal(t, tc.model, body["model"])
			assert.Equal(t, "https://dashscope.aliyuncs.com/api/v1/services/aigc/"+tc.service+"/video-synthesis", url)
			for key, value := range tc.input {
				assert.Equal(t, value, body["input"].(map[string]any)[key], key)
			}
			for key, value := range tc.parameters {
				assert.EqualValues(t, value, body["parameters"].(map[string]any)[key], key)
			}
			assert.Equal(t, tc.seconds, facts["seconds"])
		})
	}
	t.Run("animation settles mode and upstream video duration", func(t *testing.T) {
		value, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"model": "wan2.2-animate-move"}, map[string]any{}, map[string]any{"usage": map[string]any{"video_duration": 5.2, "video_ratio": "pro"}})
		require.NoError(t, callErr)
		assert.Equal(t, map[string]any{"seconds": 5.2, "mode": "wan-pro"}, alibabaObject(t, value))
	})
	t.Run("specialized inputs reject invalid billable parameters", func(t *testing.T) {
		for _, tc := range []struct {
			model             string
			input, parameters map[string]any
			want              string
		}{
			{"happyhorse-1.1-t2v", map[string]any{"prompt": "a horse"}, map[string]any{"duration": 2}, "between 3 and 15"},
			{"happyhorse-1.1-r2v", map[string]any{"prompt": "a horse"}, nil, "reference_image"},
			{"wan2.7-videoedit", map[string]any{"media": []any{map[string]any{"type": "video", "url": "https://cdn.example/a.mp4"}}}, map[string]any{"duration": 11}, "between 2 and 10"},
			{"wan2.2-animate-mix", map[string]any{"image_url": "https://cdn.example/a.png", "video_url": "https://cdn.example/a.mp4"}, map[string]any{"mode": "unknown"}, "mode"},
		} {
			_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": tc.model, "upstreamModel": tc.model, "baseUrl": "https://dashscope.aliyuncs.com", "apiKey": "k", "requestBody": map[string]any{"model": tc.model, "metadata": map[string]any{"input": tc.input, "parameters": tc.parameters}}})
			require.ErrorContains(t, callErr, tc.want)
		}
	})
}
