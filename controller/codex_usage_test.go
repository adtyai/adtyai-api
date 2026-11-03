package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func callResetCodexUsage(channelID int) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/"+strconv.Itoa(channelID)+"/codex/usage/reset", nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channelID)}}
	ResetCodexChannelUsage(ctx)
	return recorder
}

type redeemBody struct {
	RedeemRequestID string `json:"redeem_request_id"`
}

func readRedeemID(t *testing.T, r *http.Request) string {
	t.Helper()
	payload, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	var body redeemBody
	require.NoError(t, json.Unmarshal(payload, &body))
	require.NotEmpty(t, body.RedeemRequestID)
	return body.RedeemRequestID
}

func insertCodexResetChannel(t *testing.T, db *gorm.DB, baseURL, key string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Name:    "codex-reset",
		Type:    constant.ChannelTypeCodex,
		Key:     key,
		Status:  1,
		BaseURL: &baseURL,
		Group:   "default",
	}
	require.NoError(t, db.Create(channel).Error)
	return channel
}

func loadCodexRedeemID(t *testing.T, db *gorm.DB, channelID int) string {
	t.Helper()
	var stored model.Channel
	require.NoError(t, db.Select("codex_reset_redeem_id").First(&stored, channelID).Error)
	return stored.CodexResetRedeemID
}

func decodeResetResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

// 响应丢失和上游 5xx 后再点，沿用已落库的同一枚 redeem_request_id。上游 2xx 之后才换新键。
func TestResetCodexChannelUsageReusesRedeemRequestID(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	var mu sync.Mutex
	var ids []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/backend-api/wham/rate-limit-reset-credits/consume", r.URL.Path)
		id := readRedeemID(t, r)
		mu.Lock()
		ids = append(ids, id)
		n := len(ids)
		mu.Unlock()
		switch n {
		case 1:
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			require.NoError(t, conn.Close())
		case 2:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"unavailable"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"consumed":true}`))
		}
	}))
	t.Cleanup(upstream.Close)

	channel := insertCodexResetChannel(t, db, upstream.URL, `{"access_token":"atk","account_id":"acc-1","type":"codex"}`)

	lost := callResetCodexUsage(channel.Id)
	require.Equal(t, false, decodeResetResponse(t, lost)["success"])
	pending := loadCodexRedeemID(t, db, channel.Id)
	require.NotEmpty(t, pending)
	mu.Lock()
	require.Equal(t, []string{pending}, ids)
	mu.Unlock()

	failed := callResetCodexUsage(channel.Id)
	failedBody := decodeResetResponse(t, failed)
	require.Equal(t, false, failedBody["success"])
	require.Equal(t, float64(http.StatusInternalServerError), failedBody["upstream_status"])
	require.Equal(t, pending, loadCodexRedeemID(t, db, channel.Id))

	confirmed := callResetCodexUsage(channel.Id)
	confirmedBody := decodeResetResponse(t, confirmed)
	require.Equal(t, true, confirmedBody["success"])
	require.Empty(t, loadCodexRedeemID(t, db, channel.Id))

	again := callResetCodexUsage(channel.Id)
	require.Equal(t, true, decodeResetResponse(t, again)["success"])
	require.Empty(t, loadCodexRedeemID(t, db, channel.Id))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, ids, 4)
	require.Equal(t, pending, ids[0])
	require.Equal(t, pending, ids[1])
	require.Equal(t, pending, ids[2])
	require.NotEqual(t, pending, ids[3])
}

// 401 后刷新令牌再重试，两次 consume 使用同一枚键；确认成功后键被清掉。
func TestResetCodexChannelUsageKeepsRedeemIDAcrossTokenRefresh(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	var mu sync.Mutex
	var ids []string
	var tokens []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-at","refresh_token":"new-rt","expires_in":3600}`))
		case "/backend-api/wham/rate-limit-reset-credits/consume":
			id := readRedeemID(t, r)
			mu.Lock()
			ids = append(ids, id)
			tokens = append(tokens, r.Header.Get("Authorization"))
			n := len(ids)
			mu.Unlock()
			if n == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"expired"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"consumed":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(service.SetCodexOAuthTokenURLForTest(upstream.URL + "/oauth/token"))

	channel := insertCodexResetChannel(t, db, upstream.URL, `{"access_token":"old-at","refresh_token":"old-rt","account_id":"acc-1","type":"codex"}`)
	recorder := callResetCodexUsage(channel.Id)
	body := decodeResetResponse(t, recorder)
	require.Equal(t, true, body["success"])
	require.Empty(t, loadCodexRedeemID(t, db, channel.Id))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{ids[0], ids[0]}, ids)
	require.Equal(t, []string{"Bearer old-at", "Bearer new-at"}, tokens)
}

// 两次同时重置只能占一枚键，上游收到的 redeem_request_id 相同。
func TestResetCodexChannelUsageConcurrentClickSharesRedeemID(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	var mu sync.Mutex
	var ids []string
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := readRedeemID(t, r)
		mu.Lock()
		ids = append(ids, id)
		mu.Unlock()
		<-started
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"late"}`))
	}))
	t.Cleanup(upstream.Close)

	channel := insertCodexResetChannel(t, db, upstream.URL, `{"access_token":"atk","account_id":"acc-1","type":"codex"}`)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		callResetCodexUsage(channel.Id)
	}()
	go func() {
		defer wg.Done()
		callResetCodexUsage(channel.Id)
	}()
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ids) == 2
	}, 5*time.Second, 10*time.Millisecond)
	close(started)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, ids, 2)
	require.Equal(t, ids[0], ids[1])
	require.Equal(t, ids[0], loadCodexRedeemID(t, db, channel.Id))
}
