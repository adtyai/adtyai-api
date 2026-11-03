package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBEpusdt 起一个假网关，记下收到的请求体，按 respond 返回。
func fakeBEpusdt(t *testing.T, respond func(path string, body map[string]any) string) (*[]map[string]any, *[]string) {
	t.Helper()
	var bodies []map[string]any
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		require.NoError(t, common.Unmarshal(raw, &body))
		bodies = append(bodies, body)
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(respond(r.URL.Path, body)))
	}))
	t.Cleanup(server.Close)
	cfg := setting.GetBEpusdtSetting()
	saved := *cfg
	cfg.Enabled, cfg.BaseURL, cfg.Secret, cfg.TradeTypes = true, server.URL, "epusdt_password_xasddawqe", []string{"usdt.trc20", "usdt.polygon"}
	oldClient, oldNow := bepusdtHTTPClient, bepusdtNow
	bepusdtHTTPClient = func() *http.Client { return server.Client() }
	bepusdtNow = func() time.Time { return time.Unix(1_800_000_000, 0) }
	t.Cleanup(func() {
		*cfg = saved
		bepusdtHTTPClient, bepusdtNow = oldClient, oldNow
	})
	return &bodies, &paths
}

const bepusdtOK = `{"status_code":200,"message":"success","data":{"trade_id":"tid-1","order_id":"EPU1","amount":"100","actual_amount":"100.01","token":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","expiration_time":900,"status":1,"payment_url":"https://pay/x"}}`

func TestBEpusdtSignOfficialVector(t *testing.T) {
	values := map[string]any{
		"order_id":     "20220201030210321",
		"amount":       float64(42),
		"notify_url":   "http://example.com/notify",
		"redirect_url": "http://example.com/redirect",
		"signature":    "ignored",
		"empty":        "",
	}
	assert.Equal(t, "1cd4b52df5587cfb1968b0c0c6e156cd", BEpusdtSign(values, "epusdt_password_xasddawqe"))
}

func TestBEpusdtSignLargeAmount(t *testing.T) {
	// The gateway decodes JSON numbers into float64 and formats with %v, so
	// 1,000,000 becomes "1e+06"; our signature must use the same text.
	values := map[string]any{"amount": float64(1_000_000), "order_id": "x"}
	want := BEpusdtSign(map[string]any{"amount": "1e+06", "order_id": "x"}, "k")
	assert.Equal(t, want, BEpusdtSign(values, "k"))
	raw, err := bepusdtSignedBody(map[string]any{"amount": float64(1_000_000), "order_id": "x"}, "k")
	require.NoError(t, err)
	body := map[string]any{}
	require.NoError(t, common.Unmarshal(raw, &body))
	assert.Equal(t, want, body["signature"])
}

func TestCreateBEpusdtOrderJSONTypes(t *testing.T) {
	bodies, paths := fakeBEpusdt(t, func(string, map[string]any) string { return bepusdtOK })
	_, err := CreateBEpusdtOrder(BEpusdtCreateRequest{
		OrderID: "EPU1", AmountUSD: decimal.NewFromInt(100), TradeType: "usdt.trc20",
		NotifyURL: "https://site/notify", RedirectURL: "https://site/wallet", Timeout: 900,
	})
	require.NoError(t, err)
	require.Len(t, *bodies, 1)
	body := (*bodies)[0]
	assert.Equal(t, "/api/v1/order/create-transaction", (*paths)[0])
	assert.IsType(t, float64(0), body["amount"], "amount is a JSON number")
	assert.Equal(t, float64(100), body["amount"])
	assert.Equal(t, "1", body["rate"], "rate is a JSON string")
	assert.Equal(t, float64(900), body["timeout"])
	assert.Equal(t, "USD", body["fiat"])
	assert.Equal(t, BEpusdtSign(body, "epusdt_password_xasddawqe"), body["signature"])
}

func TestCreateBEpusdtOrderParsesAddressAmountExpiry(t *testing.T) {
	fakeBEpusdt(t, func(string, map[string]any) string { return bepusdtOK })
	order, err := CreateBEpusdtOrder(BEpusdtCreateRequest{OrderID: "EPU1", AmountUSD: decimal.NewFromInt(100), TradeType: "usdt.trc20"})
	require.NoError(t, err)
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", order.Address, "token is the receiving address")
	assert.Equal(t, "100.01", order.ActualAmount.String())
	assert.EqualValues(t, 1_800_000_000+900, order.ExpireAt, "expiration_time is seconds left")
	assert.Equal(t, "tid-1", order.TradeId)
}

func TestCreateBEpusdtOrderStatusCode400(t *testing.T) {
	fakeBEpusdt(t, func(string, map[string]any) string {
		return `{"status_code":400,"message":"no available address","data":null}`
	})
	_, err := CreateBEpusdtOrder(BEpusdtCreateRequest{OrderID: "EPU1", AmountUSD: decimal.NewFromInt(100), TradeType: "usdt.trc20"})
	var gatewayErr BEpusdtGatewayError
	require.ErrorAs(t, err, &gatewayErr)
	assert.Equal(t, 400, gatewayErr.StatusCode)
}

func TestBEpusdtRejectsDisabledChain(t *testing.T) {
	bodies, _ := fakeBEpusdt(t, func(string, map[string]any) string { return bepusdtOK })
	_, err := CreateBEpusdtOrder(BEpusdtCreateRequest{OrderID: "EPU1", AmountUSD: decimal.NewFromInt(100), TradeType: "usdt.bep20"})
	require.ErrorIs(t, err, errBEpusdtChainDisabled)
	assert.Empty(t, *bodies, "nothing is sent for a disabled chain")
}

func TestCancelBEpusdtOrderSigned(t *testing.T) {
	bodies, paths := fakeBEpusdt(t, func(string, map[string]any) string {
		return `{"status_code":200,"message":"success","data":{"trade_id":"tid-1"}}`
	})
	require.NoError(t, CancelBEpusdtOrder("tid-1"))
	assert.Equal(t, "/api/v1/order/cancel-transaction", (*paths)[0])
	body := (*bodies)[0]
	assert.Equal(t, "tid-1", body["trade_id"])
	assert.Equal(t, BEpusdtSign(body, "epusdt_password_xasddawqe"), body["signature"])
}
