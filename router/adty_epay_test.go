package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEpay 是一个假易支付网关：respond 按收到的 mapi 请求决定返回什么。
type fakeEpay struct {
	mu      sync.Mutex
	server  *httptest.Server
	orders  []url.Values
	respond func(form url.Values) (status int, body string, delay time.Duration)
}

func newFakeEpay(t *testing.T, respond func(form url.Values) (int, string, time.Duration)) *fakeEpay {
	t.Helper()
	gw := &fakeEpay{respond: respond}
	gw.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gw.mu.Lock()
		gw.orders = append(gw.orders, r.PostForm)
		gw.mu.Unlock()
		status, body, delay := gw.respond(r.PostForm)
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(gw.server.Close)
	return gw
}

func epayQR(form url.Values) (int, string, time.Duration) {
	return 200, `{"code":1,"msg":"ok","trade_no":"G` + form.Get("out_trade_no") + `","qrcode":"https://qr.example/` + form.Get("out_trade_no") + `"}`, 0
}

func seedRouterEpayChannel(t *testing.T, name, gatewayURL, mode string, priority int) *model.AdtyEpayChannel {
	t.Helper()
	channel := &model.AdtyEpayChannel{
		Name: name, GatewayURL: gatewayURL, MerchantId: "pid-" + name, MerchantKey: "key-" + name,
		Priority: priority, Weight: 1, OrderMode: mode, Status: model.AdtyEpayStatusEnabled,
		Methods: []model.AdtyEpayChannelMethod{{Method: "alipay", Enabled: true, ReturnType: "qrcode"}},
	}
	require.NoError(t, model.SaveAdtyEpayChannel(channel, false))
	return channel
}

func shortEpayTimeouts(t *testing.T) {
	t.Helper()
	controller.SetAdtyEpayTimeoutsForTest(t, 200*time.Millisecond, 600*time.Millisecond)
}

func epayPay(t *testing.T, f *adtyGuardFixture, token string) *httptest.ResponseRecorder {
	t.Helper()
	return adtyJSON(t, f.engine, http.MethodPost, "/api/adty/epay/pay", `{"amount":100,"method":"alipay","device":"pc"}`, token)
}

func epayOrders(t *testing.T, f *adtyGuardFixture, userID int) map[string]string {
	t.Helper()
	var topUps []model.TopUp
	require.NoError(t, f.db.Where("user_id = ?", userID).Find(&topUps).Error)
	out := map[string]string{}
	for _, topUp := range topUps {
		out[topUp.TradeNo] = topUp.Status
	}
	return out
}

func TestAdtyEpayPay(t *testing.T) {
	f := newAdtyGuardFixture(t)
	confirmRouterCompliance(t)
	setRouterBonusRates(t)
	shortEpayTimeouts(t)

	t.Run("fails over on timeout", func(t *testing.T) {
		slow := newFakeEpay(t, func(url.Values) (int, string, time.Duration) { return 200, `{"code":1}`, 400 * time.Millisecond })
		good := newFakeEpay(t, epayQR)
		first := seedRouterEpayChannel(t, "slow", slow.server.URL, model.AdtyEpayOrderModeAPI, 9)
		second := seedRouterEpayChannel(t, "good", good.server.URL, model.AdtyEpayOrderModeAPI, 5)
		t.Cleanup(func() {
			_ = model.SetAdtyEpayChannelStatus(first.Id, model.AdtyEpayStatusDisabled, "manual")
			_ = model.SetAdtyEpayChannelStatus(second.Id, model.AdtyEpayStatusDisabled, "manual")
		})
		token, userID := bepusdtBuyer(t, f, "epay-timeout")
		resp := epayPay(t, f, token)
		view := decodeAdtyData[model.AdtyTopUpView](t, resp.Body)
		assert.Equal(t, second.Id, view.Ext.ChannelId)
		assert.Contains(t, view.Ext.QRCode, "https://qr.example/")
		statuses := epayOrders(t, f, userID)
		assert.Len(t, statuses, 2)
		failed := 0
		for _, status := range statuses {
			if status == common.TopUpStatusFailed {
				failed++
			}
		}
		assert.Equal(t, 1, failed, "the order on the slow channel is marked failed")
		assert.True(t, len(view.TradeNo) <= 32)
	})

	t.Run("fails over on string code and HTML", func(t *testing.T) {
		stringCode := newFakeEpay(t, func(url.Values) (int, string, time.Duration) { return 200, `{"code":"error","msg":"bad"}`, 0 })
		html := newFakeEpay(t, func(url.Values) (int, string, time.Duration) { return 200, `<html>busy</html>`, 0 })
		good := newFakeEpay(t, epayQR)
		a := seedRouterEpayChannel(t, "strcode", stringCode.server.URL, model.AdtyEpayOrderModeAPI, 9)
		b := seedRouterEpayChannel(t, "html", html.server.URL, model.AdtyEpayOrderModeAPI, 8)
		c := seedRouterEpayChannel(t, "good2", good.server.URL, model.AdtyEpayOrderModeAPI, 7)
		t.Cleanup(func() {
			for _, id := range []int{a.Id, b.Id, c.Id} {
				_ = model.SetAdtyEpayChannelStatus(id, model.AdtyEpayStatusDisabled, "manual")
			}
		})
		token, _ := bepusdtBuyer(t, f, "epay-codes")
		view := decodeAdtyData[model.AdtyTopUpView](t, epayPay(t, f, token).Body)
		assert.Equal(t, c.Id, view.Ext.ChannelId)
	})

	t.Run("fails over when the gateway returns a javascript link", func(t *testing.T) {
		evil := newFakeEpay(t, func(form url.Values) (int, string, time.Duration) {
			return 200, `{"code":1,"trade_no":"G` + form.Get("out_trade_no") + `","qrcode":"javascript:alert(1)"}`, 0
		})
		good := newFakeEpay(t, epayQR)
		bad := seedRouterEpayChannel(t, "jsqr", evil.server.URL, model.AdtyEpayOrderModeAPI, 9)
		next := seedRouterEpayChannel(t, "jsgood", good.server.URL, model.AdtyEpayOrderModeAPI, 5)
		t.Cleanup(func() {
			_ = model.SetAdtyEpayChannelStatus(bad.Id, model.AdtyEpayStatusDisabled, "manual")
			_ = model.SetAdtyEpayChannelStatus(next.Id, model.AdtyEpayStatusDisabled, "manual")
		})
		token, _ := bepusdtBuyer(t, f, "epay-js")
		view := decodeAdtyData[model.AdtyTopUpView](t, epayPay(t, f, token).Body)
		assert.Equal(t, next.Id, view.Ext.ChannelId)
		assert.NotContains(t, view.Ext.QRCode, "javascript:")
		assert.Contains(t, view.Ext.QRCode, "https://qr.example/")
	})

	t.Run("total deadline", func(t *testing.T) {
		var ids []int
		for i := range 4 {
			slow := newFakeEpay(t, func(url.Values) (int, string, time.Duration) { return 200, `{}`, 250 * time.Millisecond })
			ids = append(ids, seedRouterEpayChannel(t, fmt.Sprintf("dl%d", i), slow.server.URL, model.AdtyEpayOrderModeAPI, 9-i).Id)
		}
		t.Cleanup(func() {
			for _, id := range ids {
				_ = model.SetAdtyEpayChannelStatus(id, model.AdtyEpayStatusDisabled, "manual")
			}
		})
		token, userID := bepusdtBuyer(t, f, "epay-deadline")
		started := time.Now()
		resp := epayPay(t, f, token)
		assert.Less(t, time.Since(started), 1500*time.Millisecond)
		assert.Contains(t, resp.Body.String(), `"success":false`)
		assert.Less(t, len(epayOrders(t, f, userID)), 4, "stops trying once the total deadline passes")
	})

	t.Run("unavailable", func(t *testing.T) {
		token, _ := bepusdtBuyer(t, f, "epay-none")
		resp := epayPay(t, f, token)
		assert.Contains(t, resp.Body.String(), "This payment method is not available")
	})

	t.Run("submit mode builds a signed URL", func(t *testing.T) {
		channel := seedRouterEpayChannel(t, "submit", "https://pay.example/submit.php", model.AdtyEpayOrderModeSubmit, 9)
		t.Cleanup(func() { _ = model.SetAdtyEpayChannelStatus(channel.Id, model.AdtyEpayStatusDisabled, "manual") })
		token, _ := bepusdtBuyer(t, f, "epay-submit")
		view := decodeAdtyData[model.AdtyTopUpView](t, epayPay(t, f, token).Body)
		parsed, err := url.Parse(view.Ext.PayURL)
		require.NoError(t, err)
		assert.Equal(t, "/submit.php", parsed.Path)
		params := map[string]string{}
		for key, values := range parsed.Query() {
			params[key] = values[0]
		}
		assert.True(t, service.EpayVerify(params, "key-submit"))
		assert.Equal(t, "730.00", params["money"])
		assert.NotEmpty(t, params["return_url"])
	})
}

func epayCallback(t *testing.T, f *adtyGuardFixture, channelID int, key string, values map[string]string) string {
	t.Helper()
	sign, err := service.EpaySign(values, key)
	require.NoError(t, err)
	query := url.Values{}
	for k, v := range values {
		query.Set(k, v)
	}
	query.Set("sign", sign)
	query.Set("sign_type", "MD5")
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/user/epay/notify/%d?%s", channelID, query.Encode()), nil)
	resp := httptest.NewRecorder()
	f.engine.ServeHTTP(resp, req)
	return resp.Body.String()
}

func TestAdtyEpayNotify(t *testing.T) {
	f := newAdtyGuardFixture(t)
	confirmRouterCompliance(t)
	setRouterBonusRates(t)
	gw := newFakeEpay(t, epayQR)
	one := seedRouterEpayChannel(t, "one", gw.server.URL, model.AdtyEpayOrderModeAPI, 9)
	two := seedRouterEpayChannel(t, "two", gw.server.URL, model.AdtyEpayOrderModeAPI, 1)
	token, userID := bepusdtBuyer(t, f, "epay-notify")
	order := func() model.AdtyTopUpView {
		return decodeAdtyData[model.AdtyTopUpView](t, epayPay(t, f, token).Body)
	}
	params := func(tradeNo, pid, money string) map[string]string {
		return map[string]string{"pid": pid, "trade_no": "G" + tradeNo, "out_trade_no": tradeNo, "type": "alipay",
			"name": "Top-up", "money": money, "trade_status": "TRADE_SUCCESS"}
	}
	reviews := func(tradeNo string) int64 {
		var count int64
		require.NoError(t, f.db.Model(&model.PaymentOrphanEvent{}).Where("reference_id = ?", tradeNo).Count(&count).Error)
		return count
	}

	view := order()
	require.Equal(t, one.Id, view.Ext.ChannelId)
	t.Run("uses the channel's own key", func(t *testing.T) {
		body := epayCallback(t, f, two.Id, "key-one", params(view.TradeNo, "pid-one", "730.00"))
		assert.Equal(t, "fail", body)
	})
	t.Run("credits once", func(t *testing.T) {
		before := quotaOf(t, f, userID)
		for range 2 {
			assert.Equal(t, "success", epayCallback(t, f, one.Id, "key-one", params(view.TradeNo, "pid-one", "730.00")))
		}
		assert.Equal(t, before+51_250_000, quotaOf(t, f, userID))
	})
	t.Run("amount mismatch queues review and acks", func(t *testing.T) {
		other := order()
		assert.Equal(t, "success", epayCallback(t, f, one.Id, "key-one", params(other.TradeNo, "pid-one", "729.99")))
		assert.EqualValues(t, 1, reviews(other.TradeNo))
	})
	t.Run("channel mismatch queues review", func(t *testing.T) {
		other := order()
		assert.Equal(t, "success", epayCallback(t, f, two.Id, "key-two", params(other.TradeNo, "pid-two", "730.00")))
		assert.EqualValues(t, 1, reviews(other.TradeNo))
	})
	t.Run("test order acked", func(t *testing.T) {
		assert.Equal(t, "success", epayCallback(t, f, one.Id, "key-one", params("TEST11234", "pid-one", "1.00")))
	})
	t.Run("late callback after failed", func(t *testing.T) {
		before := quotaOf(t, f, userID)
		other := order()
		require.NoError(t, f.db.Model(&model.TopUp{}).Where("trade_no = ?", other.TradeNo).Update("status", common.TopUpStatusFailed).Error)
		assert.Equal(t, "success", epayCallback(t, f, one.Id, "key-one", params(other.TradeNo, "pid-one", "730.00")))
		assert.Equal(t, before+51_250_000, quotaOf(t, f, userID))
	})
}

func TestEpayAdmin(t *testing.T) {
	f := newAdtyGuardFixture(t)
	gw := newFakeEpay(t, func(form url.Values) (int, string, time.Duration) {
		if form.Get("type") == "wxpay" {
			return 200, `{"code":-1,"msg":"支付方式未开通"}`, 0
		}
		return epayQR(form)
	})
	body := `{"name":"zpay","gateway_url":"` + gw.server.URL + `","merchant_id":"1001","merchant_key":"k-secret","priority":1,"weight":1,"order_mode":"api","methods":[{"method":"alipay","enabled":true},{"method":"wxpay","enabled":true}]}`

	t.Run("permissions", func(t *testing.T) {
		create := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/epay/channels", body, f.token)
		assert.Equal(t, http.StatusForbidden, create.Code, "only root creates channels")
		list := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/epay/channels", "", f.token)
		assert.Equal(t, http.StatusForbidden, list.Code)
		f.grant(t, authz.AdtyPaymentRead)
		list = adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/epay/channels", "", f.token)
		assert.Equal(t, http.StatusOK, list.Code)
	})

	create := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/epay/channels", body, f.root)
	require.Contains(t, create.Body.String(), `"success":true`, create.Body.String())
	created := decodeAdtyData[struct{ Id int }](t, strings.NewReader(create.Body.String()))

	t.Run("key is write-only", func(t *testing.T) {
		list := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/epay/channels", "", f.root)
		assert.NotContains(t, list.Body.String(), "k-secret")
		assert.Contains(t, list.Body.String(), `"has_key":true`)
		update := strings.Replace(body, `"merchant_key":"k-secret"`, `"merchant_key":""`, 1)
		resp := adtyJSON(t, f.engine, http.MethodPut, fmt.Sprintf("/api/adty/admin/epay/channels/%d", created.Id), update, f.root)
		require.Contains(t, resp.Body.String(), `"success":true`)
		stored, err := model.GetAdtyEpayChannel(created.Id)
		require.NoError(t, err)
		assert.Equal(t, "k-secret", stored.MerchantKey, "an empty key keeps the stored one")
	})

	t.Run("test order disables an unsupported method", func(t *testing.T) {
		f.grant(t, authz.AdtyPaymentRead)
		denied := adtyJSON(t, f.engine, http.MethodPost, fmt.Sprintf("/api/adty/admin/epay/channels/%d/test", created.Id), "", f.token)
		assert.Equal(t, http.StatusForbidden, denied.Code)
		f.grant(t, authz.AdtyPaymentRead, authz.AdtyPaymentOperate)
		resp := adtyJSON(t, f.engine, http.MethodPost, fmt.Sprintf("/api/adty/admin/epay/channels/%d/test", created.Id), "", f.token)
		require.Contains(t, resp.Body.String(), `"success":true`, resp.Body.String())
		stored, err := model.GetAdtyEpayChannel(created.Id)
		require.NoError(t, err)
		assert.Nil(t, stored.MethodFor("wxpay"), "wxpay was turned off")
		assert.NotNil(t, stored.MethodFor("alipay"))
		var topUps int64
		require.NoError(t, f.db.Model(&model.TopUp{}).Count(&topUps).Error)
		assert.Zero(t, topUps, "test orders do not write top_ups")
		status := adtyJSON(t, f.engine, http.MethodPost, fmt.Sprintf("/api/adty/admin/epay/channels/%d/status", created.Id), `{"status":"enabled"}`, f.token)
		assert.Contains(t, status.Body.String(), `"success":true`)
	})
}
