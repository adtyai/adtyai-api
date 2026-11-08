package vertex

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVertexGeminiAPIKeyHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotPath, gotKey, gotMethod, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey, gotMethod = r.URL.Path, r.URL.Query().Get("key"), r.Method
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5},"modelVersion":"gemini-2.5-flash"}`)
	}))
	defer server.Close()

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl:    server.URL,
		ApiKey:            "vertex-secret",
		ApiVersion:        `{"default":"global"}`,
		UpstreamModelName: "gemini-2.5-flash",
		ChannelOtherSettings: dto.ChannelOtherSettings{
			VertexKeyType:   dto.VertexKeyTypeAPIKey,
			VertexProjectID: "fixture-project",
		},
	}}
	info.OriginModelName = "gemini-2.5-flash"
	info.RelayMode = constant.RelayModeChatCompletions
	info.RelayFormat = types.RelayFormatOpenAI
	adaptor := &Adaptor{}
	adaptor.Init(info)
	body := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"temperature":0.2}}`
	response, err := adaptor.DoRequest(ctx, info, strings.NewReader(body))
	require.NoError(t, err)
	httpResponse, ok := response.(*http.Response)
	require.True(t, ok)
	defer httpResponse.Body.Close()

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/v1/projects/fixture-project/locations/global/publishers/google/models/gemini-2.5-flash:generateContent", gotPath)
	assert.Equal(t, "vertex-secret", gotKey)
	assert.JSONEq(t, body, gotBody)
	assert.Empty(t, httpResponse.Request.Header.Get("Authorization"))

	usage, apiErr := adaptor.DoResponse(ctx, httpResponse, info)
	require.Nil(t, apiErr)
	assert.Equal(t, 3, usage.(*dto.Usage).PromptTokens)
	assert.Equal(t, 2, usage.(*dto.Usage).CompletionTokens)
	assert.Contains(t, recorder.Body.String(), `"content":"hello"`)
}

func TestVertexGeminiAPIKeyReturnsProviderAndMalformedErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantStatus int
	}{
		{name: "provider block", body: `{"promptFeedback":{"blockReason":"SAFETY"}}`, wantStatus: http.StatusBadRequest},
		{name: "malformed", body: `{"candidates":`, wantStatus: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, tc.body) }))
			defer server.Close()
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: server.URL, ApiKey: "vertex-secret", ApiVersion: `{"default":"global"}`, UpstreamModelName: "gemini-2.5-flash",
				ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey},
			}}
			info.OriginModelName = "gemini-2.5-flash"
			info.RelayMode = constant.RelayModeChatCompletions
			info.RelayFormat = types.RelayFormatGemini
			adaptor := &Adaptor{}
			adaptor.Init(info)
			response, err := adaptor.DoRequest(ctx, info, strings.NewReader(`{"contents":[]}`))
			require.NoError(t, err)
			httpResponse, ok := response.(*http.Response)
			require.True(t, ok)
			defer httpResponse.Body.Close()
			_, apiErr := adaptor.DoResponse(ctx, httpResponse, info)
			if apiErr != nil {
				assert.Equal(t, tc.wantStatus, apiErr.StatusCode)
			} else {
				assert.Equal(t, tc.wantStatus, recorder.Code)
			}
			if tc.name == "malformed" {
				require.NotNil(t, apiErr)
			}
		})
	}
}

func TestVertexGeminiAPIKeyContextCancellationStopsUpstream(t *testing.T) {
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
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(request)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl: server.URL, ApiKey: "vertex-secret", ApiVersion: `{"default":"global"}`, UpstreamModelName: "gemini-2.5-flash",
		ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey},
	}}
	info.OriginModelName = "gemini-2.5-flash"
	info.RelayMode = constant.RelayModeChatCompletions
	info.RelayFormat = types.RelayFormatOpenAI
	adaptor := &Adaptor{}
	adaptor.Init(info)
	done := make(chan error, 1)
	go func() {
		_, err := adaptor.DoRequest(ctx, info, strings.NewReader(`{"contents":[]}`))
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request did not arrive")
	}
	cancelRequest()
	select {
	case err := <-done:
		assert.Error(t, err)
	case <-time.After(2 * time.Second):
		server.CloseClientConnections()
		<-done
		t.Fatal("upstream cancellation did not return")
	}
}
