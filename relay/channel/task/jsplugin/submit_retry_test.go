package jsplugin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcceptedJSONSubmitDoesNotRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plugin, err := pluginruntime.CompilePlugin(mockPlugin, pluginruntime.Options{})
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		OriginModelName: "mock-v1",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "mock-upstream", ChannelBaseUrl: "https://provider.example"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "public-task"},
	}
	adaptor := New(plugin)
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"a cat"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("task_request", map[string]any{"prompt": "a cat"})
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	oversized := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxTaskPluginPersistedJSONBytes+1))),
	}
	_, taskErr := adaptor.ParseResponse(c, oversized, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
	assert.True(t, taskErr.NoRetry, "an oversized accepted body must not be submitted again")

	thrown, err := pluginruntime.CompilePlugin(strings.Replace(mockPlugin, "return {\n    taskId: resp.body.id,", "throw new Error(\"bad payload\"); return {\n    taskId: resp.body.id,", 1), pluginruntime.Options{})
	require.NoError(t, err)
	thrownAdaptor := New(thrown)
	thrownAdaptor.Init(info)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"paid-task"}`)),
	}
	_, taskErr = thrownAdaptor.ParseResponse(c, response, info)
	require.NotNil(t, taskErr)
	assert.True(t, taskErr.NoRetry, "a received JSON body must not be submitted again")
}
