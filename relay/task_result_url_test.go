package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicTaskResultURLHidesDataURI(t *testing.T) {
	prev := system_setting.ServerAddress
	system_setting.ServerAddress = "https://api.example.test"
	t.Cleanup(func() { system_setting.ServerAddress = prev })

	task := &model.Task{
		TaskID: "task_data",
		PrivateData: model.TaskPrivateData{
			ResultURL: "data:video/mp4;base64,AAAA",
		},
	}
	assert.Equal(t, "https://api.example.test/v1/videos/task_data/content", publicTaskResultURL(task))
	assert.Equal(t, "https://cdn.example/video.mp4", publicTaskResultURL(&model.Task{
		PrivateData: model.TaskPrivateData{ResultURL: "https://cdn.example/video.mp4"},
	}))
}

func TestPublicTaskResultURLDoesNotAdvertiseEmptyResultWithoutPlugin(t *testing.T) {
	prev := system_setting.ServerAddress
	system_setting.ServerAddress = "https://api.example.test"
	t.Cleanup(func() { system_setting.ServerAddress = prev })

	empty := &model.Task{TaskID: "task_empty", Status: model.TaskStatusSuccess}
	assert.Empty(t, publicTaskResultURL(empty))

	withPlugin := &model.Task{
		TaskID: "task_plugin",
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{Execution: &model.TaskExecutionSnapshot{
			TaskPlugin: &model.TaskPluginSnapshot{Key: "plugin.video"},
		}},
	}
	assert.Equal(t, "https://api.example.test/v1/videos/task_plugin/content", publicTaskResultURL(withPlugin))
}

func TestOverlayOpenAIVideoResultURLPatchesOnlyURL(t *testing.T) {
	body := []byte(`{"id":"vid_1","status":"completed","seconds":"8","metadata":{"url":"https://cdn.example/a.mp4","duration":5}}`)
	got := overlayOpenAIVideoResultURL(body, "https://api.example.test/v1/videos/task_1/content")

	var payload map[string]any
	require.NoError(t, common.Unmarshal(got, &payload))
	assert.Equal(t, "https://api.example.test/v1/videos/task_1/content", payload["url"])
	assert.Equal(t, "vid_1", payload["id"])
	assert.Equal(t, "8", payload["seconds"])
	meta, ok := payload["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://api.example.test/v1/videos/task_1/content", meta["url"])
	assert.EqualValues(t, 5, meta["duration"])

	unchanged := overlayOpenAIVideoResultURL(body, "")
	assert.Equal(t, string(body), string(unchanged))
}
