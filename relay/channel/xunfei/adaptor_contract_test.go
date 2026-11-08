package xunfei

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestXunfeiConfiguredEndpointAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v4.0/chat" {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		verifyXunfeiHandshake(t, r, "contract-key", "contract-secret")
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		var request XunfeiChatRequest
		if err := conn.ReadJSON(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Header.AppId != "contract-app" {
			t.Errorf("unexpected app ID: %q", request.Header.AppId)
		}
		if err := conn.WriteJSON(map[string]any{
			"header": map[string]any{"code": 0, "status": 2},
			"payload": map[string]any{
				"choices": map[string]any{"status": 2, "text": []any{map[string]any{"content": "answer", "role": "assistant"}}},
				"usage":   map[string]any{"text": map[string]any{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7}},
			},
		}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	out := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(out)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("base_url", server.URL)
	_, authURL := getXunfeiAuthUrl(c, "contract-key", "contract-secret", "SparkDesk-v4.0")
	// Fail before any network call if the configured local endpoint is ignored.
	require.True(t, strings.HasPrefix(authURL, "ws"+strings.TrimPrefix(server.URL, "http")+"/v4.0/chat?"), "configured endpoint was ignored: %s", authURL)
	info := &common.RelayInfo{ChannelMeta: &common.ChannelMeta{ApiKey: "contract-app|contract-secret|contract-key"}}
	a := &Adaptor{}
	_, err := a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{
		Model: "SparkDesk-v4.0", Messages: []dto.Message{{Role: "user", Content: "question"}},
	})
	require.NoError(t, err)
	resp, err := a.DoRequest(c, info, nil)
	require.NoError(t, err)
	usage, apiErr := a.DoResponse(c, resp.(*http.Response), info)
	require.Nil(t, apiErr)
	require.Equal(t, &dto.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}, usage)
	require.Contains(t, out.Body.String(), "answer")
	require.Contains(t, out.Body.String(), `"total_tokens":7`)
}

func TestXunfeiAdaptorRejectsFailedResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, frame string
	}{
		{name: "provider error", frame: `{"header":{"code":10013,"message":"contract upstream denied"}}`},
		{name: "malformed json", frame: `{`},
		{name: "connection closes before final frame"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				var request XunfeiChatRequest
				if err := conn.ReadJSON(&request); err != nil {
					t.Error(err)
					return
				}
				if tc.frame != "" {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(tc.frame)); err != nil {
						t.Error(err)
					}
				}
			}))
			defer server.Close()
			out := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(out)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set("base_url", server.URL)
			info := &common.RelayInfo{ChannelMeta: &common.ChannelMeta{ApiKey: "app|secret|key"}}
			a := &Adaptor{}
			_, err := a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: "SparkDesk-v4.0"})
			require.NoError(t, err)
			_, apiErr := a.DoResponse(c, &http.Response{StatusCode: http.StatusOK}, info)
			require.NotNil(t, apiErr, "an upstream failure must not become a successful empty answer: %s", out.Body.String())
			require.NotContains(t, out.Body.String(), `"object":"chat.completion"`)
		})
	}
}

func TestXunfeiStreamCompletionAndFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, finalFrame string
		wantError        bool
		nonStream        bool
	}{
		{name: "complete", finalFrame: `{"header":{"code":0},"payload":{"choices":{"status":2,"text":[]},"usage":{"text":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}}}`},
		{name: "buffered final usage", nonStream: true, finalFrame: `{"header":{"code":0},"payload":{"choices":{"status":2,"text":[]},"usage":{"text":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}}}`},
		{name: "provider failure", finalFrame: `{"header":{"code":10013,"message":"denied"}}`, wantError: true},
		{name: "malformed frame", finalFrame: `{`, wantError: true},
		{name: "missing final frame", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				var request XunfeiChatRequest
				if err := conn.ReadJSON(&request); err != nil {
					t.Error(err)
					return
				}
				for _, frame := range []string{`{"header":{"code":0},"payload":{"choices":{"status":1,"text":[{"content":"answer","role":"assistant"}]}}}`, tc.finalFrame} {
					if frame != "" {
						if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
							t.Error(err)
							return
						}
					}
				}
			}))
			defer server.Close()
			out := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(out)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set("base_url", server.URL)
			info := &common.RelayInfo{IsStream: !tc.nonStream, ChannelMeta: &common.ChannelMeta{ApiKey: "app|secret|key"}}
			a := &Adaptor{}
			_, err := a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: "SparkDesk-v4.0"})
			require.NoError(t, err)
			usage, apiErr := a.DoResponse(c, &http.Response{StatusCode: http.StatusOK}, info)
			require.Contains(t, out.Body.String(), "answer")
			if tc.wantError {
				require.NotNil(t, apiErr)
				require.NotContains(t, out.Body.String(), "[DONE]")
				return
			}
			require.Nil(t, apiErr)
			require.Equal(t, &dto.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}, usage)
			if tc.nonStream {
				require.Contains(t, out.Body.String(), `"total_tokens":7`)
				return
			}
			require.Contains(t, out.Body.String(), "[DONE]")
			require.Contains(t, out.Body.String(), `"finish_reason":"stop"`)
		})
	}
}

func TestXunfeiCancellationClosesUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	connected := make(chan *websocket.Conn, 1)
	upstreamClosed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		var request XunfeiChatRequest
		if err := conn.ReadJSON(&request); err != nil {
			t.Error(err)
			return
		}
		connected <- conn
		_, _, _ = conn.ReadMessage()
		close(upstreamClosed)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	c.Set("base_url", server.URL)
	info := &common.RelayInfo{ChannelMeta: &common.ChannelMeta{ApiKey: "app|secret|key"}}
	a := &Adaptor{}
	_, err := a.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: "SparkDesk-v4.0"})
	require.NoError(t, err)
	returned := make(chan bool, 1)
	go func() {
		_, apiErr := a.DoResponse(c, &http.Response{StatusCode: http.StatusOK}, info)
		returned <- apiErr != nil
	}()
	var conn *websocket.Conn
	select {
	case conn = <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("local upstream did not receive the request")
	}
	defer conn.Close()
	cancel()
	select {
	case failed := <-returned:
		require.True(t, failed, "cancellation must not return success")
	case <-time.After(time.Second):
		_ = conn.Close()
		<-returned
		t.Fatal("cancelled request continued waiting for the upstream")
	}
	select {
	case <-upstreamClosed:
	case <-time.After(time.Second):
		t.Fatal("upstream WebSocket was not closed after cancellation")
	}
}

func TestXunfeiNativeWebSocketContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var received XunfeiChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v4.0/chat" {
			t.Errorf("Xunfei WebSocket path = %q", r.URL.Path)
		}
		verifyXunfeiHandshake(t, r, "contract-api-key", "contract-api-secret")
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade local WebSocket: %v", err)
			return
		}
		defer conn.Close()
		if err := conn.ReadJSON(&received); err != nil {
			t.Errorf("read native Xunfei request: %v", err)
			return
		}
		_ = conn.WriteJSON(map[string]any{
			"header": map[string]any{"code": 0, "sid": "x1", "status": 2},
			"payload": map[string]any{
				"choices": map[string]any{"status": 2, "text": []any{map[string]any{"content": "answer", "role": "assistant"}}},
				"usage":   map[string]any{"text": map[string]any{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7}},
			},
		})
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v4.0/chat"
	authURL := buildXunfeiAuthUrl(wsURL, "contract-api-key", "contract-api-secret")
	request := dto.GeneralOpenAIRequest{
		Model: "SparkDesk-v4.0",
		Messages: []dto.Message{
			{Role: "system", Content: "rules"},
			{Role: "user", Content: "question"},
		},
	}
	dataChan, stopChan, err := xunfeiMakeRequest(t.Context(), request, "4.0Ultra", authURL, "contract-app-id")
	if err != nil {
		t.Fatalf("xunfeiMakeRequest: %v", err)
	}
	response := <-dataChan
	if response.Payload.Choices.Status != 2 || response.Payload.Choices.Text[0].Content != "answer" {
		t.Fatalf("Xunfei response frame = %#v", response)
	}
	require.NoError(t, <-stopChan)
	if received.Header.AppId != "contract-app-id" || received.Parameter.Chat.Domain != "4.0Ultra" || len(received.Payload.Message.Text) != 3 {
		t.Fatalf("Xunfei native request = %#v", received)
	}
	if received.Payload.Message.Text[0].Role != "user" || received.Payload.Message.Text[1].Content != "Okay" || received.Payload.Message.Text[2].Content != "question" {
		t.Fatalf("Xunfei system-message mapping = %#v", received.Payload.Message.Text)
	}
}

func verifyXunfeiHandshake(t *testing.T, r *http.Request, apiKey, apiSecret string) {
	t.Helper()
	query := r.URL.Query()
	if query.Get("host") != r.Host {
		t.Errorf("signed host query = %q, request host = %q", query.Get("host"), r.Host)
	}
	decodedAuth, err := base64.StdEncoding.DecodeString(query.Get("authorization"))
	if err != nil {
		t.Errorf("decode Xunfei authorization: %v", err)
		return
	}
	auth := string(decodedAuth)
	if !strings.HasPrefix(auth, fmt.Sprintf("hmac username=\"%s\",", apiKey)) {
		t.Errorf("Xunfei authorization identity = %q", auth)
	}
	start, end := strings.LastIndex(auth, `signature="`), strings.LastIndex(auth, `"`)
	if start < 0 || end <= start+len(`signature="`) {
		t.Errorf("Xunfei signature field missing from %q", auth)
		return
	}
	gotSignature := auth[start+len(`signature="`) : end]
	signingText := strings.Join([]string{
		"host: " + r.Host,
		"date: " + query.Get("date"),
		"GET " + r.URL.Path + " HTTP/1.1",
	}, "\n")
	mac := hmac.New(sha256.New, []byte(apiSecret))
	_, _ = mac.Write([]byte(signingText))
	if !hmac.Equal([]byte(gotSignature), []byte(base64.StdEncoding.EncodeToString(mac.Sum(nil)))) {
		t.Errorf("Xunfei request signature failed verification for %q", signingText)
	}
}

func TestXunfeiAdaptorConversionAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request := &dto.GeneralOpenAIRequest{Model: "SparkDesk-v3.5", Messages: []dto.Message{{Role: "system", Content: "rules"}}}
	info := &common.RelayInfo{ChannelMeta: &common.ChannelMeta{ApiKey: "invalid"}}
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertOpenAIRequest(c, info, request)
	if err != nil || converted != request {
		t.Fatalf("ConvertOpenAIRequest = %#v, %v", converted, err)
	}
	responseValue, err := adaptor.DoRequest(c, info, nil)
	if err != nil {
		t.Fatalf("DoRequest should create its native-protocol placeholder: %v", err)
	}
	response, ok := responseValue.(*http.Response)
	if !ok || response.StatusCode != http.StatusOK {
		t.Fatalf("DoRequest placeholder = %#v", responseValue)
	}
	_, apiErr := adaptor.DoResponse(c, response, info)
	if apiErr == nil || apiErr.GetErrorCode() != "channel:invalid_key" {
		t.Fatalf("invalid Xunfei key error = %#v", apiErr)
	}

	missingRequest := &Adaptor{}
	info.ApiKey = "app|secret|key"
	_, apiErr = missingRequest.DoResponse(c, response, info)
	if apiErr == nil || apiErr.GetErrorCode() != "invalid_request" {
		t.Fatalf("missing Xunfei conversion error = %#v", apiErr)
	}
}
