package controller

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestGetResponseBodyClosesNonOKResponse(t *testing.T) {
	var mu sync.Mutex
	var states []http.ConnState
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"balance":1}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		mu.Lock()
		states = append(states, state)
		mu.Unlock()
	}
	upstream.Start()
	t.Cleanup(upstream.Close)

	channel := &model.Channel{Key: "sk-test"}
	_, err := GetResponseBody(http.MethodGet, upstream.URL+"/balance", channel, GetAuthHeader(channel.Key))
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")

	body, err := GetResponseBody(http.MethodGet, upstream.URL+"/ok", channel, GetAuthHeader(channel.Key))
	require.NoError(t, err)
	require.Equal(t, `{"balance":1}`, string(body))

	if client := service.GetHttpClient(); client != nil {
		client.CloseIdleConnections()
	}
	http.DefaultClient.CloseIdleConnections()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, state := range states {
			if state == http.StateClosed {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
}
