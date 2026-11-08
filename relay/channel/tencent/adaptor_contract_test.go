package tencent

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTencentAdaptorHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotPath, gotAction, gotVersion, gotTimestamp, gotAuthorization, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAction = r.Header.Get("X-TC-Action")
		gotVersion = r.Header.Get("X-TC-Version")
		gotTimestamp = r.Header.Get("X-TC-Timestamp")
		gotAuthorization = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Response":{"Id":"req-1","Choices":[{"Message":{"Role":"assistant","Content":"hello"},"FinishReason":"stop"}],"Usage":{"PromptTokens":3,"CompletionTokens":2,"TotalTokens":5}}}`)
	}))
	defer server.Close()

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "123|secret-id|secret-key")

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl: server.URL,
		ApiKey:         "123|secret-id|secret-key",
	}}
	info.UpstreamModelName = "hunyuan-test"
	info.OriginModelName = "hunyuan-test"
	adaptor := &Adaptor{}
	adaptor.Init(info)
	requestBody, err := adaptor.ConvertOpenAIRequest(ctx, info, &dto.GeneralOpenAIRequest{
		Model: "hunyuan-test", Messages: []dto.Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	encoded, err := common.Marshal(requestBody)
	require.NoError(t, err)
	response, err := adaptor.DoRequest(ctx, info, bytes.NewReader(encoded))
	require.NoError(t, err)
	httpResponse, ok := response.(*http.Response)
	require.True(t, ok)
	defer httpResponse.Body.Close()

	assert.Equal(t, "/", gotPath)
	assert.Equal(t, "ChatCompletions", gotAction)
	assert.Equal(t, "2023-09-01", gotVersion)
	assert.NotEmpty(t, gotTimestamp)
	assert.Regexp(t, `^TC3-HMAC-SHA256 Credential=secret-id/`, gotAuthorization)
	assert.Contains(t, gotAuthorization, "Signature=")
	assert.JSONEq(t, `{"Model":"hunyuan-test","Messages":[{"Role":"user","Content":"hi"}]}`, gotBody)

	usage, apiErr := adaptor.DoResponse(ctx, httpResponse, info)
	require.Nil(t, apiErr)
	assert.Equal(t, 3, usage.(*dto.Usage).PromptTokens)
	assert.Equal(t, 2, usage.(*dto.Usage).CompletionTokens)
	assert.Contains(t, recorder.Body.String(), `"content":"hello"`)
}

func TestTencentAdaptorRejectsMalformedAndProviderErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
	}{
		{name: "malformed", body: `{"Response":`},
		{name: "provider error", body: `{"Response":{"Error":{"Code":1001,"Message":"invalid request"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			common.SetContextKey(ctx, constant.ContextKeyChannelKey, "123|secret-id|secret-key")
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "123|secret-id|secret-key"}}
			info.UpstreamModelName = "hunyuan-test"
			adaptor := &Adaptor{}
			adaptor.Init(info)
			response, err := adaptor.DoRequest(ctx, info, strings.NewReader(`{"Messages":[]}`))
			require.NoError(t, err)
			httpResponse, ok := response.(*http.Response)
			require.True(t, ok)
			defer httpResponse.Body.Close()
			_, apiErr := adaptor.DoResponse(ctx, httpResponse, info)
			require.NotNil(t, apiErr)
		})
	}
}

func TestTencentAdaptorRequestCancellationStopsUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	entered := make(chan struct{})
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-releaseHandler:
		}
	}))
	defer server.Close()
	defer close(releaseHandler)
	request, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(request)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "123|secret-id|secret-key")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "123|secret-id|secret-key"}}
	info.UpstreamModelName = "hunyuan-test"
	adaptor := &Adaptor{}
	adaptor.Init(info)
	done := make(chan error, 1)
	go func() {
		_, err := adaptor.DoRequest(ctx, info, strings.NewReader(`{"Messages":[]}`))
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request did not arrive")
	}
	cancel()
	select {
	case err := <-done:
		assert.Error(t, err)
	case <-time.After(2 * time.Second):
		server.CloseClientConnections()
		<-done
		t.Fatal("upstream cancellation did not return")
	}
}
