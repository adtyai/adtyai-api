package openai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type realtimeTestSockets struct {
	client      *websocket.Conn
	clientRelay *websocket.Conn
	targetRelay *websocket.Conn
	upstream    *websocket.Conn
}

func newRealtimeTestSockets(t *testing.T) *realtimeTestSockets {
	t.Helper()
	gin.SetMode(gin.TestMode)
	clientRelayConnCh := make(chan *websocket.Conn, 1)
	clientHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			clientRelayConnCh <- conn
		}
	}))
	t.Cleanup(clientHTTP.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(clientHTTP.URL, "http"), nil)
	require.NoError(t, err)
	clientRelay := <-clientRelayConnCh

	targetRelayConnCh := make(chan *websocket.Conn, 1)
	targetHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			targetRelayConnCh <- conn
		}
	}))
	t.Cleanup(targetHTTP.Close)

	upstream, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(targetHTTP.URL, "http"), nil)
	require.NoError(t, err)
	targetRelay := <-targetRelayConnCh
	sockets := &realtimeTestSockets{
		client: client, clientRelay: clientRelay, targetRelay: upstream,
		upstream: targetRelay,
	}
	t.Cleanup(sockets.close)
	return sockets
}

func (s *realtimeTestSockets) close() {
	for _, conn := range []*websocket.Conn{s.client, s.clientRelay, s.targetRelay, s.upstream} {
		if conn != nil {
			_ = conn.Close()
		}
	}
}

func (s *realtimeTestSockets) relayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ClientWs:        s.clientRelay,
		TargetWs:        s.targetRelay,
		OriginModelName: "gpt-4o-realtime-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-4o-realtime-preview",
		},
		IsFirstRequest: true,
	}
}

type realtimeTestResult struct {
	usage  chan *dto.RealtimeUsage
	done   chan struct{}
	cancel context.CancelFunc
}

func startRealtimeTestHandler(t *testing.T, sockets *realtimeTestSockets, info *relaycommon.RelayInfo) realtimeTestResult {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil).WithContext(ctx)
	finished := realtimeTestResult{usage: make(chan *dto.RealtimeUsage, 1), done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(finished.done)
		_, usage := OpenaiRealtimeHandler(ginContext, info)
		finished.usage <- usage
	}()
	t.Cleanup(func() {
		cancel()
		sockets.close()
		select {
		case <-finished.done:
		case <-time.After(5 * time.Second):
			t.Error("realtime handler did not stop during cleanup")
		}
	})
	return finished
}

func TestOpenaiRealtimeStopsWhenRequestIsCancelled(t *testing.T) {
	sockets := newRealtimeTestSockets(t)
	finished := startRealtimeTestHandler(t, sockets, sockets.relayInfo())
	finished.cancel()
	select {
	case <-finished.done:
	case <-time.After(5 * time.Second):
		t.Fatal("realtime handler ignored request cancellation while both peers were idle")
	}
}

func TestOpenaiRealtimeStopsWhenClientWriteFails(t *testing.T) {
	sockets := newRealtimeTestSockets(t)
	info := sockets.relayInfo()
	info.UsePrice = true
	finished := startRealtimeTestHandler(t, sockets, info)
	clientTransport, ok := sockets.clientRelay.UnderlyingConn().(*net.TCPConn)
	require.True(t, ok)
	require.NoError(t, clientTransport.CloseWrite())
	require.NoError(t, sockets.upstream.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.audio_transcript.delta","delta":"hello"}`)))
	select {
	case <-finished.done:
	case <-time.After(5 * time.Second):
		t.Fatal("realtime handler kept reading upstream after writing to the client failed")
	}
}

func TestOpenaiRealtimeCountsFailedPreConsumptionOnce(t *testing.T) {
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN is required for the realtime billing regression")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	db := opened.DB
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	user := &model.User{Username: "realtime-empty", Quota: 0}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: "realtime-empty", UnlimitedQuota: true}
	require.NoError(t, db.Create(token).Error)

	for _, tc := range []struct {
		name  string
		input string
		done  string
		want  int
	}{
		{"provider usage", "", `{"type":"response.done","response":{"usage":{"total_tokens":300,"input_tokens":200,"output_tokens":100,"input_token_details":{"text_tokens":200},"output_token_details":{"text_tokens":100}}}}`, 300},
		{"local fallback", `{"type":"conversation.item.created","item":{"type":"message","content":[{"type":"input_text","text":"hello"}]}}`, `{"type":"response.done"}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sockets := newRealtimeTestSockets(t)
			info := sockets.relayInfo()
			info.UserId, info.TokenId, info.TokenKey = user.Id, token.Id, "sk-"+token.Key
			info.UsingGroup, info.UserGroup = "default", "default"
			finished := startRealtimeTestHandler(t, sockets, info)
			if tc.input != "" {
				require.NoError(t, sockets.client.WriteMessage(websocket.TextMessage, []byte(tc.input)))
				require.NoError(t, sockets.upstream.SetReadDeadline(time.Now().Add(2*time.Second)))
				_, _, err := sockets.upstream.ReadMessage()
				require.NoError(t, err)
			}
			require.NoError(t, sockets.upstream.WriteMessage(websocket.TextMessage, []byte(tc.done)))
			select {
			case <-finished.done:
			case <-time.After(5 * time.Second):
				t.Fatal("realtime handler did not stop when the wallet had insufficient quota")
			}
			usage := <-finished.usage
			require.NotNil(t, usage)
			require.Equal(t, tc.want, usage.TotalTokens, "failed pre-consumption must not duplicate the usage sent to final settlement")
			quota, err := model.GetUserQuota(user.Id, false)
			require.NoError(t, err)
			require.Zero(t, quota)
		})
	}
}

func TestOpenaiRealtimeForwardsResponseDoneWithoutResponse(t *testing.T) {
	sockets := newRealtimeTestSockets(t)
	info := sockets.relayInfo()
	info.UsePrice = true
	finished := startRealtimeTestHandler(t, sockets, info)
	go func() {
		for {
			if _, _, err := sockets.upstream.ReadMessage(); err != nil {
				return
			}
		}
	}()

	message := []byte(`{"type":"response.done"}`)
	require.NoError(t, sockets.upstream.WriteMessage(websocket.TextMessage, message))
	sockets.client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, got, err := sockets.client.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, string(message), string(got))

	_ = sockets.client.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	select {
	case <-finished.done:
	case <-time.After(5 * time.Second):
		t.Fatal("realtime handler did not stop after the client closed")
	}
}

func TestOpenaiRealtimeRejectsOversizedMessages(t *testing.T) {
	previousLimit := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previousLimit })

	for _, source := range []string{"client", "upstream"} {
		t.Run(source, func(t *testing.T) {
			sockets := newRealtimeTestSockets(t)
			finished := startRealtimeTestHandler(t, sockets, sockets.relayInfo())
			peer, other := sockets.client, sockets.upstream
			if source == "upstream" {
				peer, other = other, peer
			}
			go func() {
				for {
					if _, _, err := other.ReadMessage(); err != nil {
						return
					}
				}
			}()
			message := []byte(fmt.Sprintf(`{"type":"unknown","padding":"%s"}`, strings.Repeat("x", 1<<20)))
			// The receiver can reject the declared size before the sender finishes writing.
			_ = peer.WriteMessage(websocket.TextMessage, message)
			require.NoError(t, peer.SetReadDeadline(time.Now().Add(2*time.Second)))
			_, _, err := peer.ReadMessage()
			var closeErr *websocket.CloseError
			require.ErrorAs(t, err, &closeErr)
			require.Equal(t, websocket.CloseMessageTooBig, closeErr.Code)
			select {
			case <-finished.done:
			case <-time.After(5 * time.Second):
				t.Fatal("realtime handler did not stop after an oversized message")
			}
		})
	}
}

func TestOpenaiRealtimeHandlesConcurrentUsageEvents(t *testing.T) {
	sockets := newRealtimeTestSockets(t)
	info := sockets.relayInfo()
	info.UsePrice = true
	finished := startRealtimeTestHandler(t, sockets, info)

	readMessages := func(conn *websocket.Conn, expected int, result chan<- int) {
		count := 0
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				result <- count
				return
			}
			count++
			if count == expected {
				result <- count
				return
			}
		}
	}

	const events = 200
	clientReceived := make(chan int, 1)
	upstreamReceived := make(chan int, 1)
	go readMessages(sockets.client, events, clientReceived)
	go readMessages(sockets.upstream, events, upstreamReceived)
	clientEvent := []byte(`{"type":"conversation.item.created","item":{"type":"message","content":[{"type":"input_text","text":"hello"}]}}`)
	targetEvent := []byte(`{"type":"response.audio_transcript.delta","delta":"hello"}`)
	writeEvents := func(conn *websocket.Conn, event []byte) {
		for range events {
			if err := conn.WriteMessage(websocket.TextMessage, event); err != nil {
				return
			}
		}
	}
	writeDone := make(chan struct{}, 2)
	go func() { writeEvents(sockets.client, clientEvent); writeDone <- struct{}{} }()
	go func() { writeEvents(sockets.upstream, targetEvent); writeDone <- struct{}{} }()
	<-writeDone
	<-writeDone
	select {
	case count := <-clientReceived:
		require.Equal(t, events, count)
	case <-time.After(5 * time.Second):
		t.Fatal("client did not receive all upstream events")
	}
	select {
	case count := <-upstreamReceived:
		require.Equal(t, events, count)
	case <-time.After(5 * time.Second):
		t.Fatal("upstream did not receive all client events")
	}

	_ = sockets.client.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = sockets.upstream.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	select {
	case <-finished.done:
	case <-time.After(5 * time.Second):
		t.Fatal("realtime handler did not stop after both peers closed")
	}
	usage := <-finished.usage
	require.NotNil(t, usage)
	require.Equal(t, events*2, usage.TotalTokens)
	require.Equal(t, events, usage.InputTokens)
	require.Equal(t, events, usage.OutputTokens)
}
