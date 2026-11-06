package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
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
