package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bepusdtTestSecret = "bepusdt-test-secret"

type bepusdtGateway struct {
	createStatus  int
	cancelStatus  int
	cancelMessage string
	dropCancel    bool
	creates       int
	cancels       []string
}

// useFakeBEpusdt 配好 BEpusdt 并把网关指向一个假服务。
func useFakeBEpusdt(t *testing.T) *bepusdtGateway {
	t.Helper()
	gw := &bepusdtGateway{createStatus: 200, cancelStatus: 200}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		_ = common.Unmarshal(raw, &body)
		if strings.HasSuffix(r.URL.Path, "/cancel-transaction") {
			gw.cancels = append(gw.cancels, body["trade_id"].(string))
			if gw.dropCancel {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
				return
			}
			message := gw.cancelMessage
			if message == "" {
				message = "x"
			}
			_, _ = w.Write([]byte(`{"status_code":` + strconv.Itoa(gw.cancelStatus) + `,"message":"` + message + `","data":{}}`))
			return
		}
		gw.creates++
		if gw.createStatus != 200 {
			_, _ = w.Write([]byte(`{"status_code":400,"message":"no address","data":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"status_code":200,"message":"success","data":{"trade_id":"tid-` + body["order_id"].(string) +
			`","order_id":"` + body["order_id"].(string) + `","amount":"100","actual_amount":"100.01","token":"TAddr","expiration_time":900,"status":1,"payment_url":""}}`))
	}))
	t.Cleanup(server.Close)
	cfg := setting.GetBEpusdtSetting()
	saved := *cfg
	cfg.Enabled, cfg.BaseURL, cfg.Secret, cfg.TradeTypes = true, server.URL, bepusdtTestSecret, []string{"usdt.trc20"}
	t.Cleanup(func() { *cfg = saved })
	return gw
}

// signedNotify 按网关的规则签一个回调。
func signedNotify(values map[string]any) string {
	raw, _ := common.Marshal(values)
	decoded := map[string]any{}
	_ = common.Unmarshal(raw, &decoded)
	values["signature"] = service.BEpusdtSign(decoded, bepusdtTestSecret)
	out, _ := common.Marshal(values)
	return string(out)
}

func notifyFor(tradeNo string, status int, paid float64, address string) string {
	return signedNotify(map[string]any{
		"trade_id": "tid-" + tradeNo, "order_id": tradeNo, "amount": 100, "actual_amount": paid,
		"token": address, "block_transaction_id": "0xhash", "status": status,
	})
}

func payBEpusdt(t *testing.T, f *adtyGuardFixture, token string) model.AdtyTopUpView {
	t.Helper()
	resp := adtyJSON(t, f.engine, http.MethodPost, "/api/user/bepusdt/pay", `{"amount":100,"chain":"usdt.trc20"}`, token)
	require.Equal(t, http.StatusOK, resp.Code)
	return decodeAdtyData[model.AdtyTopUpView](t, resp.Body)
}

func bepusdtBuyer(t *testing.T, f *adtyGuardFixture, name string) (string, int) {
	t.Helper()
	token := adtySessionToken(t, f.db, name, common.RoleCommonUser)
	var user model.User
	require.NoError(t, f.db.Where("username = ?", name).First(&user).Error)
	return token, user.Id
}

func quotaOf(t *testing.T, f *adtyGuardFixture, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, f.db.First(&user, id).Error)
	return user.Quota
}

func TestBEpusdtPayCreatesOrderWithBonus(t *testing.T) {
	f := newAdtyGuardFixture(t)
	confirmRouterCompliance(t)
	setRouterBonusRates(t)
	useFakeBEpusdt(t)
	token, _ := bepusdtBuyer(t, f, "usdt-buyer")
	view := payBEpusdt(t, f, token)
	require.NotNil(t, view.Ext)
	assert.True(t, strings.HasPrefix(view.TradeNo, "EPU"))
	assert.Equal(t, "TAddr", view.Ext.PayAddress)
	assert.Equal(t, "100.01", view.Ext.PayAmount.String(), "pay amount comes from the gateway")
	assert.Equal(t, "2.5", view.Ext.BonusUSD.String())
	assert.EqualValues(t, 51_250_000, view.Ext.CreditQuota)
	assert.Positive(t, view.Ext.ExpireAt)
}

// 网关下单失败时订单改成 failed。2026-11-01 取消了支付合规确认，原来「没确认时不下单」那一段删掉了：
// 不确认也能下单由 TestAdtyWalletTopUpAPIs 覆盖。
func TestBEpusdtPayHandlesGatewayFailure(t *testing.T) {
	f := newAdtyGuardFixture(t)
	gw := useFakeBEpusdt(t)
	token, userID := bepusdtBuyer(t, f, "usdt-fail")
	gw.createStatus = 400
	failed := adtyJSON(t, f.engine, http.MethodPost, "/api/user/bepusdt/pay", `{"amount":100,"chain":"usdt.trc20"}`, token)
	assert.Contains(t, failed.Body.String(), `"success":false`)
	var topUp model.TopUp
	require.NoError(t, f.db.Where("user_id = ?", userID).First(&topUp).Error)
	assert.Equal(t, common.TopUpStatusFailed, topUp.Status)
}

func TestBEpusdtNotify(t *testing.T) {
	f := newAdtyGuardFixture(t)
	confirmRouterCompliance(t)
	setRouterBonusRates(t)
	useFakeBEpusdt(t)
	token, userID := bepusdtBuyer(t, f, "usdt-notify")
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/user/bepusdt/notify", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		f.engine.ServeHTTP(resp, req)
		return resp
	}
	reviews := func(tradeNo string) int64 {
		var count int64
		require.NoError(t, f.db.Model(&model.PaymentOrphanEvent{}).Where("reference_id = ?", tradeNo).Count(&count).Error)
		return count
	}

	t.Run("bad signature", func(t *testing.T) {
		view := payBEpusdt(t, f, token)
		body := strings.Replace(notifyFor(view.TradeNo, 2, 100.01, "TAddr"), `"status":2`, `"status":2,"extra":"x"`, 1)
		resp := post(body)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("waiting status does not credit", func(t *testing.T) {
		before := quotaOf(t, f, userID)
		view := payBEpusdt(t, f, token)
		resp := post(notifyFor(view.TradeNo, 1, 100.01, "TAddr"))
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "ok", resp.Body.String())
		assert.Equal(t, before, quotaOf(t, f, userID))
	})

	t.Run("credits once on duplicate callbacks", func(t *testing.T) {
		before := quotaOf(t, f, userID)
		view := payBEpusdt(t, f, token)
		for range 2 {
			resp := post(notifyFor(view.TradeNo, 2, 100.01, "TAddr"))
			assert.Equal(t, "ok", resp.Body.String())
		}
		assert.Equal(t, before+51_250_000, quotaOf(t, f, userID))
	})

	t.Run("late callback after expiry and after failure", func(t *testing.T) {
		for _, status := range []string{common.TopUpStatusExpired, common.TopUpStatusFailed} {
			before := quotaOf(t, f, userID)
			view := payBEpusdt(t, f, token)
			require.NoError(t, f.db.Model(&model.TopUp{}).Where("trade_no = ?", view.TradeNo).Update("status", status).Error)
			resp := post(notifyFor(view.TradeNo, 2, 100.01, "TAddr"))
			assert.Equal(t, "ok", resp.Body.String(), status)
			assert.Equal(t, before+51_250_000, quotaOf(t, f, userID), status)
		}
	})

	t.Run("amount mismatch queues review and acks", func(t *testing.T) {
		before := quotaOf(t, f, userID)
		view := payBEpusdt(t, f, token)
		resp := post(notifyFor(view.TradeNo, 2, 99.5, "TAddr"))
		assert.Equal(t, "ok", resp.Body.String())
		assert.Equal(t, before, quotaOf(t, f, userID))
		assert.EqualValues(t, 1, reviews(view.TradeNo))
	})

	t.Run("address mismatch queues review", func(t *testing.T) {
		view := payBEpusdt(t, f, token)
		resp := post(notifyFor(view.TradeNo, 2, 100.01, "TOther"))
		assert.Equal(t, "ok", resp.Body.String())
		assert.EqualValues(t, 1, reviews(view.TradeNo))
	})

	t.Run("missing order queues review", func(t *testing.T) {
		resp := post(notifyFor("EPU-missing", 2, 100.01, "TAddr"))
		assert.Equal(t, "ok", resp.Body.String())
		assert.EqualValues(t, 1, reviews("EPU-missing"))
	})

	t.Run("quota limit queues review", func(t *testing.T) {
		view := payBEpusdt(t, f, token)
		require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", userID).Update("quota", common.MaxWalletQuota-1).Error)
		resp := post(notifyFor(view.TradeNo, 2, 100.01, "TAddr"))
		assert.Equal(t, "ok", resp.Body.String())
		assert.EqualValues(t, 1, reviews(view.TradeNo))
		require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", userID).Update("quota", 0).Error)
	})
}

func TestBEpusdtCancelCallsGateway(t *testing.T) {
	f := newAdtyGuardFixture(t)
	confirmRouterCompliance(t)
	gw := useFakeBEpusdt(t)
	token, _ := bepusdtBuyer(t, f, "usdt-cancel")
	view := payBEpusdt(t, f, token)

	gw.cancelStatus = 400
	gw.cancelMessage = "order already canceled"
	closed := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/orders/"+view.TradeNo+"/cancel", "", token)
	assert.Contains(t, closed.Body.String(), `"success":true`, closed.Body.String())
	var stored model.TopUp
	require.NoError(t, f.db.Where("trade_no = ?", view.TradeNo).First(&stored).Error)
	assert.Equal(t, model.TopUpStatusCanceled, stored.Status)

	paid := payBEpusdt(t, f, token)
	gw.cancelMessage = "transfer detected"
	failed := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/orders/"+paid.TradeNo+"/cancel", "", token)
	assert.Contains(t, failed.Body.String(), "A transfer has been detected")
	stored = model.TopUp{}
	require.NoError(t, f.db.Where("trade_no = ?", paid.TradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)

	droppedOrder := payBEpusdt(t, f, token)
	gw.dropCancel = true
	dropped := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/orders/"+droppedOrder.TradeNo+"/cancel", "", token)
	assert.Contains(t, dropped.Body.String(), `"success":true`, dropped.Body.String())
	stored = model.TopUp{}
	require.NoError(t, f.db.Where("trade_no = ?", droppedOrder.TradeNo).First(&stored).Error)
	assert.Equal(t, model.TopUpStatusCanceled, stored.Status)
	assert.Equal(t, []string{"tid-" + view.TradeNo, "tid-" + paid.TradeNo, "tid-" + droppedOrder.TradeNo}, gw.cancels)
}
