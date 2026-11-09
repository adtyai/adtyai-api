package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBEpusdtProviderQueryTrustAndOrderBinding(t *testing.T) {
	query, ok := AdtyPaymentProviderByName(model.PaymentProviderAdtyBEpusdt).(AdtyPaymentOrderQuerier)
	require.True(t, ok, "BEpusdt must expose the verified provider query capability")
	var requests, redirected atomic.Int32
	var response atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		_, _ = w.Write([]byte(`{"status_code":200}`))
	}))
	t.Cleanup(target.Close)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/pay/info", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		var values map[string]any
		assert.NoError(t, common.Unmarshal(body, &values))
		assert.Equal(t, map[string]any{"trade_id": "gateway-query"}, values, "the public query must not send the merchant secret")
		text := response.Load().(string)
		if text == "redirect" {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		if text == "http-failure" {
			w.WriteHeader(http.StatusBadGateway)
			text = `{"status_code":200,"data":{}}`
		}
		_, _ = w.Write([]byte(text))
	}))
	t.Cleanup(server.Close)
	cfg := setting.GetBEpusdtSetting()
	saved, oldClient := *cfg, bepusdtHTTPClient
	cfg.Enabled, cfg.BaseURL, cfg.Secret = false, server.URL, "merchant-secret-must-not-leak"
	bepusdtHTTPClient = server.Client
	t.Cleanup(func() { *cfg, bepusdtHTTPClient = saved, oldClient })
	ext := model.AdtyTopupOrder{Provider: model.PaymentProviderAdtyBEpusdt, TradeNo: "BE-query", Method: "usdt.trc20",
		GatewayTradeId: "gateway-query", PayAddress: "TCaseSensitiveAddress", PayURL: server.URL + "/pay/checkout/gateway-query",
		FaceUSD: decimal.NewFromInt(2), TopupGroupRatio: decimal.NewFromInt(1), PayAmount: decimal.RequireFromString("2.01"), PayCurrency: "USDT"}
	paid := `{"status_code":200,"data":{"trade_id":"gateway-query","order_id":"BE-query","trade_type":"usdt.trc20","status":2,"money":"2","actual_amount":"2.01","token":"TCaseSensitiveAddress","fiat":"USD"}}`
	response.Store(paid)
	facts, err := query.Query(t.Context(), &ext)
	require.NoError(t, err, "disabling new checkouts must not prevent recovery of an existing order")
	require.NotNil(t, facts)
	assert.Equal(t, ext.Provider, facts.Provider)
	assert.Equal(t, ext.Method, facts.Method)
	assert.Equal(t, "USDT", facts.PaidCurrency)
	assert.True(t, decimal.RequireFromString("2.01").Equal(facts.PaidAmount))
	assert.True(t, decimal.NewFromInt(2).Equal(facts.FiatAmount))
	assert.Equal(t, ext.PayAddress, facts.PayAddress)
	assert.Equal(t, ext.GatewayTradeId, facts.GatewayTradeId)
	assert.Empty(t, facts.TxHash, "Info does not return a chain transaction hash")
	assert.Equal(t, model.AdtyPaymentSourceQuery, facts.Source)
	for _, status := range []int{1, 3, 4, 5, 6} {
		response.Store(strings.Replace(paid, `"status":2`, fmt.Sprintf(`"status":%d`, status), 1))
		facts, err = query.Query(t.Context(), &ext)
		require.NoError(t, err)
		assert.Nil(t, facts, "unconfirmed and failed payments must not become credits")
	}
	for _, invalid := range []struct{ name, from, to string }{
		{"gateway", `"gateway-query"`, `"other-gateway"`},
		{"order", `"BE-query"`, `"another-order"`},
		{"network", `"usdt.trc20"`, `"usdt.bep20"`},
		{"address-case", `"TCaseSensitiveAddress"`, `"tcasesensitiveaddress"`},
		{"fiat", `"USD"`, `"CNY"`},
		{"amount", `"actual_amount":"2.01"`, `"actual_amount":"1.99"`},
		{"fiat-amount", `"money":"2"`, `"money":"2.01"`},
		{"unknown-status", `"status":2`, `"status":99`},
		{"missing-status", `"status":2,`, ""},
		{"wrong-number", `"actual_amount":"2.01"`, `"actual_amount":"NaN"`},
		{"gateway-error", `"status_code":200`, `"status_code":400`},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			response.Store(strings.Replace(paid, invalid.from, invalid.to, 1))
			facts, err := query.Query(t.Context(), &ext)
			require.Error(t, err)
			assert.Nil(t, facts)
		})
	}
	for _, invalid := range []string{"redirect", "http-failure", "not-json", strings.Repeat("x", (1<<20)+1)} {
		response.Store(invalid)
		facts, err = query.Query(t.Context(), &ext)
		require.Error(t, err)
		assert.Nil(t, facts)
		assert.NotContains(t, err.Error(), cfg.Secret)
	}
	assert.Zero(t, redirected.Load(), "provider redirects must never be followed")
	response.Store(paid)
	for _, endpoint := range []string{strings.Replace(server.URL, "https://", "http://", 1), server.URL + "?gateway=other", "https://another.example"} {
		cfg.BaseURL = endpoint
		before := requests.Load()
		_, err = query.Query(t.Context(), &ext)
		require.Error(t, err)
		assert.Equal(t, before, requests.Load(), "untrusted or changed gateway must not be queried")
	}
	cfg.BaseURL = server.URL
	for _, field := range []string{"gateway", "address", "url", "provider"} {
		invalid := ext
		switch field {
		case "gateway":
			invalid.GatewayTradeId = ""
		case "address":
			invalid.PayAddress = ""
		case "url":
			invalid.PayURL = ""
		case "provider":
			invalid.Provider = model.PaymentProviderStripe
		}
		before := requests.Load()
		_, err = query.Query(t.Context(), &invalid)
		require.Error(t, err)
		assert.Equal(t, before, requests.Load(), field)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = query.Query(ctx, &ext)
	require.Error(t, err)
	// Relay deployments may opt out of certificate checks. An unsigned payment
	// response must still authenticate the gateway independently of that option.
	insecureTransport := http.DefaultTransport.(*http.Transport).Clone()
	insecureTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // regression fixture
	t.Cleanup(insecureTransport.CloseIdleConnections)
	bepusdtHTTPClient = func() *http.Client { return &http.Client{Transport: insecureTransport} }
	before := requests.Load()
	_, err = query.Query(t.Context(), &ext)
	require.Error(t, err, "untrusted certificates must remain rejected when the relay client disables verification")
	assert.Equal(t, before, requests.Load())
}

func TestBEpusdtLostCallbackRecoveryDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := prepareReferralDialectDB(t, dialect)
			_, ok := AdtyPaymentProviderByName(model.PaymentProviderAdtyBEpusdt).(AdtyPaymentOrderQuerier)
			require.True(t, ok)
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var body struct {
					TradeID string `json:"trade_id"`
				}
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				trade := strings.TrimPrefix(body.TradeID, "gw-")
				_, _ = fmt.Fprintf(w, `{"status_code":200,"data":{"trade_id":%q,"order_id":%q,"trade_type":"usdt.trc20","status":2,"money":"2","actual_amount":"2.01","token":"TRecoveryAddress","fiat":"USD"}}`, body.TradeID, trade)
			}))
			t.Cleanup(server.Close)
			cfg := setting.GetBEpusdtSetting()
			saved, oldClient := *cfg, bepusdtHTTPClient
			cfg.BaseURL, cfg.Enabled = server.URL, false
			bepusdtHTTPClient = server.Client
			t.Cleanup(func() { *cfg, bepusdtHTTPClient = saved, oldClient })
			buyer := referralUser(t, db, "BE-recovery-buyer")
			now := common.GetTimestamp()
			orders := make(map[string]*model.AdtyTopupOrder)
			for _, fixture := range []struct {
				name           string
				age, lastQuery int64
			}{
				{"lost", 300, 0}, {"raced", 300, now}, {"young", 30, 0}, {"old", 8 * 86400, 0}, {"recent-query", 300, now},
			} {
				trade := "BE-" + fixture.name
				ext := &model.AdtyTopupOrder{TradeNo: trade, UserId: buyer.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
					FaceUSD: decimal.NewFromInt(2), CreditQuota: 1000000, PayAmount: decimal.RequireFromString("2.01"), PayCurrency: "USDT",
					GatewayTradeId: "gw-" + trade, PayAddress: "TRecoveryAddress", PayURL: server.URL + "/pay/checkout/gw-" + trade,
					CreatedAt: now - fixture.age, LastQueryAt: fixture.lastQuery}
				require.NoError(t, model.CreateAdtyTopUp(&model.TopUp{UserId: buyer.Id, TradeNo: trade, PaymentProvider: ext.Provider,
					PaymentMethod: ext.Method, Status: common.TopUpStatusPending, CreateTime: now - fixture.age}, ext))
				orders[fixture.name] = ext
			}
			recoverAdtyHostedPayments(now)
			assert.EqualValues(t, 1, requests.Load(), "bounded maintenance must skip young, old and recently queried orders")
			assert.Equal(t, common.TopUpStatusSuccess, topUpStatusOf(t, db, orders["lost"].TradeNo))
			assert.Equal(t, common.TopUpStatusPending, topUpStatusOf(t, db, orders["raced"].TradeNo))
			require.NoError(t, RecoverAdtyHostedPaymentAs(t.Context(), orders["lost"], 88))
			raced := orders["raced"]
			start, results := make(chan struct{}), make(chan error, 2)
			go func() {
				<-start
				results <- RecoverAdtyHostedPaymentAs(t.Context(), raced, 88)
			}()
			go func() {
				<-start
				_, err := model.CompleteAdtyTopUp(raced.TradeNo, model.AdtyPaymentFacts{Provider: raced.Provider, Method: raced.Method,
					PaidAmount: decimal.RequireFromString("2.01"), FiatAmount: decimal.NewFromInt(2), PaidCurrency: "USDT",
					PayAddress: raced.PayAddress, GatewayTradeId: raced.GatewayTradeId, TxHash: "actual-chain-hash", Source: model.AdtyPaymentSourceCallback}, "")
				results <- err
			}()
			close(start)
			for range 2 {
				require.NoError(t, <-results)
			}
			require.NoError(t, RecoverAdtyHostedPaymentAs(t.Context(), raced, 88))
			var current model.User
			require.NoError(t, db.First(&current, buyer.Id).Error)
			assert.Equal(t, 2000000, current.Quota, "lost callback and concurrent delivery each credit exactly once")
			for _, name := range []string{"lost", "raced"} {
				var credits int64
				require.NoError(t, db.Model(&model.AdtyBalanceLedger{}).Where("ref_id = ?", orders[name].TradeNo).Count(&credits).Error)
				assert.EqualValues(t, 1, credits)
			}
			var events []model.AdtyOrderPaymentEvent
			require.NoError(t, db.Where("trade_no = ?", orders["lost"].TradeNo).Order("id").Find(&events).Error)
			require.Len(t, events, 2)
			assert.Equal(t, "credited", events[0].Outcome)
			assert.Equal(t, "replayed", events[1].Outcome)
			assert.Equal(t, 88, events[1].OperatorId)
			assert.Equal(t, model.AdtyPaymentSourceQuery, events[0].Source)
		})
	}
}

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
