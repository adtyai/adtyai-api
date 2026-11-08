package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configurePublicUptime(t *testing.T, enabled bool, groups []map[string]any) {
	t.Helper()
	settings := console_setting.GetConsoleSetting()
	original := *settings
	t.Cleanup(func() { *settings = original })
	encoded, err := common.Marshal(groups)
	require.NoError(t, err)
	settings.UptimeKumaEnabled = enabled
	settings.UptimeKumaGroups = string(encoded)
}

func requestPublicUptime(ctx context.Context) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/uptime/status", nil)
	GetUptimeKumaStatus(c)
	return response
}

func publicUptimeData(t *testing.T, response *httptest.ResponseRecorder) []UptimeGroupResult {
	t.Helper()
	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool                `json:"success"`
		Data    []UptimeGroupResult `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.NotNil(t, payload.Data)
	return payload.Data
}

func writePublicUptimeFixture(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.HasPrefix(r.URL.Path, "/api/status-page/heartbeat/") {
		_, _ = w.Write([]byte(`{"heartbeatList":{"7":[{"status":1}]},"uptimeList":{"7_24":99.5}}`))
		return
	}
	_, _ = w.Write([]byte(`{"publicGroupList":[{"id":2,"name":"private-channel-group","monitorList":[{"id":7,"name":"Public monitor"}]}]}`))
}

func TestPublicUptimeDisabledDoesNotFetchConfiguredUpstream(t *testing.T) {
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		writePublicUptimeFixture(w, r)
	}))
	t.Cleanup(upstream.Close)
	configurePublicUptime(t, false, []map[string]any{{"url": upstream.URL, "slug": "public", "categoryName": "Service"}})

	data := publicUptimeData(t, requestPublicUptime(t.Context()))

	assert.Empty(t, data)
	assert.Zero(t, requests.Load(), "turning the public panel off must stop unauthenticated upstream work")
}

func TestPublicUptimeStatusDoesNotCopyGroupName(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(writePublicUptimeFixture))
	t.Cleanup(upstream.Close)
	configurePublicUptime(t, true, []map[string]any{{"url": upstream.URL, "slug": "public", "categoryName": "Service"}})

	response := requestPublicUptime(t.Context())
	data := publicUptimeData(t, response)

	require.Len(t, data, 1)
	assert.Equal(t, "Service", data[0].CategoryName)
	assert.Equal(t, []Monitor{{Name: "Public monitor", Uptime: 99.5, Status: 1}}, data[0].Monitors)
	assert.NotContains(t, response.Body.String(), "private-channel-group")
}

func TestPublicUptimeDisablingPanelHidesCachedResults(t *testing.T) {
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		writePublicUptimeFixture(w, r)
	}))
	t.Cleanup(upstream.Close)
	configurePublicUptime(t, true, []map[string]any{{"url": upstream.URL, "slug": "public", "categoryName": "Service"}})
	require.Len(t, publicUptimeData(t, requestPublicUptime(t.Context())), 1)
	console_setting.GetConsoleSetting().UptimeKumaEnabled = false

	assert.Empty(t, publicUptimeData(t, requestPublicUptime(t.Context())))
	assert.EqualValues(t, 2, requests.Load(), "a cached result must not bypass the disabled public panel")
}

func TestUptimeStatusCacheExpirationRejectsOldSnapshot(t *testing.T) {
	cache := uptimeKumaStatusCache{
		configuration: "old configuration",
		expiresAt:     time.Now().Add(-time.Second),
		results:       []UptimeGroupResult{{CategoryName: "Old"}},
	}

	results, ok := cache.get("old configuration")

	assert.False(t, ok)
	assert.Nil(t, results, "an expired snapshot must force a refresh rather than cache failures indefinitely")
}

func TestPublicUptimeConcurrentAndRepeatedRequestsShareRefresh(t *testing.T) {
	var requests atomic.Int64
	release := make(chan struct{})
	started := make(chan struct{}, 32)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		started <- struct{}{}
		select {
		case <-release:
			writePublicUptimeFixture(w, r)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(upstream.Close)
	configurePublicUptime(t, true, []map[string]any{{"url": upstream.URL, "slug": "public", "categoryName": "Service"}})

	const callers = 12
	responses := make([]*httptest.ResponseRecorder, callers)
	var pending sync.WaitGroup
	for i := range callers {
		pending.Go(func() { responses[i] = requestPublicUptime(t.Context()) })
	}
	for range 2 {
		select {
		case <-started:
		case <-t.Context().Done():
			t.Fatal("the shared upstream refresh did not start")
		}
	}
	close(release)
	pending.Wait()
	for _, response := range responses {
		data := publicUptimeData(t, response)
		require.Len(t, data, 1)
		assert.Equal(t, []Monitor{{Name: "Public monitor", Uptime: 99.5, Status: 1}}, data[0].Monitors)
	}
	for range 3 {
		require.Len(t, publicUptimeData(t, requestPublicUptime(t.Context())), 1)
	}
	assert.EqualValues(t, 2, requests.Load(), "a public request burst must perform one status/heartbeat pair")
}

func TestPublicUptimeUpstreamFailureIsCached(t *testing.T) {
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)
	configurePublicUptime(t, true, []map[string]any{{"url": upstream.URL, "slug": "public", "categoryName": "Service"}})

	first := publicUptimeData(t, requestPublicUptime(t.Context()))
	require.Len(t, first, 1)
	assert.Empty(t, first[0].Monitors)
	initialRequests := requests.Load()
	require.Positive(t, initialRequests)
	for range 4 {
		assert.Equal(t, first, publicUptimeData(t, requestPublicUptime(t.Context())))
	}
	assert.Equal(t, initialRequests, requests.Load(), "an unavailable upstream must not be retried for every visitor")
}

func TestPublicUptimeConfigurationChangeInvalidatesCachedResponse(t *testing.T) {
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		writePublicUptimeFixture(w, r)
	}))
	t.Cleanup(upstream.Close)
	configurePublicUptime(t, true, []map[string]any{{"url": upstream.URL, "slug": "first", "categoryName": "First"}})
	first := publicUptimeData(t, requestPublicUptime(t.Context()))
	require.Len(t, first, 1)
	require.Equal(t, "First", first[0].CategoryName)
	updated, err := common.Marshal([]map[string]any{{"url": upstream.URL, "slug": "second", "categoryName": "Second"}})
	require.NoError(t, err)
	console_setting.GetConsoleSetting().UptimeKumaGroups = string(updated)

	second := publicUptimeData(t, requestPublicUptime(t.Context()))

	require.Len(t, second, 1)
	assert.Equal(t, "Second", second[0].CategoryName)
	assert.EqualValues(t, 4, requests.Load())
}

func TestPublicUptimeCancelledCallerDoesNotCancelSharedRefresh(t *testing.T) {
	var requests atomic.Int64
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		started <- struct{}{}
		select {
		case <-release:
			writePublicUptimeFixture(w, r)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(upstream.Close)
	configurePublicUptime(t, true, []map[string]any{{"url": upstream.URL, "slug": "public", "categoryName": "Service"}})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	firstDone := make(chan struct{})
	go func() {
		requestPublicUptime(ctx)
		close(firstDone)
	}()
	for range 2 {
		select {
		case <-started:
		case <-t.Context().Done():
			t.Fatal("the upstream refresh did not start")
		}
	}
	cancel()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("a cancelled caller remained blocked on shared upstream work")
	}
	close(release)
	data := publicUptimeData(t, requestPublicUptime(t.Context()))

	require.Len(t, data, 1)
	assert.Equal(t, []Monitor{{Name: "Public monitor", Uptime: 99.5, Status: 1}}, data[0].Monitors)
	assert.EqualValues(t, 2, requests.Load(), "cancelling one waiter must not discard the shared refresh for the next visitor")
}

func TestPublicUptimeBoundsConcurrentUpstreamRequests(t *testing.T) {
	var active, peak atomic.Int64
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for previous := peak.Load(); n > previous; previous = peak.Load() {
			if peak.CompareAndSwap(previous, n) {
				break
			}
		}
		select {
		case <-release:
			writePublicUptimeFixture(w, r)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(upstream.Close)
	groups := make([]map[string]any, 20)
	for i := range groups {
		groups[i] = map[string]any{"url": upstream.URL, "slug": "public-" + strconv.Itoa(i), "categoryName": "Service " + strconv.Itoa(i)}
	}
	configurePublicUptime(t, true, groups)
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	t.Cleanup(cancel)
	requestPublicUptime(ctx)
	close(release)
	data := publicUptimeData(t, requestPublicUptime(t.Context()))

	require.Len(t, data, 20)
	assert.LessOrEqual(t, peak.Load(), int64(8), "one public refresh may run at most four status/heartbeat pairs concurrently")
}

func TestUptimeUpstreamRequestHonorsContextDeadline(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	var result map[string]any

	err := getAndDecode(ctx, &http.Client{Timeout: time.Second}, upstream.URL, &result)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
