package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
)

func TestPublicTaskModelHidesUpstreamSnapshot(t *testing.T) {
	task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess, FailReason: "Incorrect API key provided: sk-channel-secret"}
	task.Properties.UpstreamModelName = "hidden-upstream-model"
	task.Data = []byte(`{"name":"operations/secret-op"}`)
	task.PrivateData.ResultURL = "https://upstream.example/video.mp4"
	view := PublicTaskModel2Dto(task)
	props, ok := view.Properties.(model.Properties)
	assert.True(t, ok)
	assert.Empty(t, props.UpstreamModelName)
	assert.Nil(t, view.Data)
	assert.NotContains(t, view.FailReason, "sk-channel-secret")
	assert.NotContains(t, view.ResultURL, "upstream.example")
	assert.Contains(t, view.ResultURL, "task_public")
}

func TestPublicTaskModelDoesNotExposeLegacySuccessResultURLInFailReason(t *testing.T) {
	resultURL := "//private-upstream.example/video/path-secret?signature=secret"
	task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess, FailReason: resultURL}

	view := PublicTaskModel2Dto(task)

	assert.Empty(t, view.FailReason)
	assert.NotContains(t, view.ResultURL, "private-upstream.example")
	assert.Equal(t, resultURL, task.FailReason)
}

func TestPublicTaskModelKeepsFailureReasonForFailedTask(t *testing.T) {
	task := &model.Task{TaskID: "task_failed", Status: model.TaskStatusFailure, FailReason: "provider rejected the request"}

	view := PublicTaskModel2Dto(task)

	assert.Equal(t, task.FailReason, view.FailReason)
}

func TestPublicTaskModelDoesNotExposeForeignVideoURLs(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	for _, resultURL := range []string{
		"https://attacker.example/v1/videos/task_public/content",
		"https://attacker.example/v1/videos/task_public/content?signature=secret",
		"//attacker.example/v1/videos/task_public/content?signature=secret",
	} {
		task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess}
		task.PrivateData.ResultURL = resultURL

		view := PublicTaskModel2Dto(task)

		assert.Equal(t, "https://gateway.example/v1/videos/task_public/content", view.ResultURL)
	}

	task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess}
	task.PrivateData.ResultURL = "https://gateway.example/v1/videos/task_public/content"
	assert.Equal(t, task.PrivateData.ResultURL, PublicTaskModel2Dto(task).ResultURL)
}
