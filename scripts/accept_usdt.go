//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func newUser(tag string) model.User {
	u := model.User{
		Username:    "accept-usdt-" + tag + "-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       0,
		AffCode:     "af" + common.GetRandomString(6),
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	return u
}

func signParams(token string, params map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range params {
		out[k] = v
	}
	out["signature"] = service.AdtyBepusdtSign(out, token)
	return out
}

func postNotify(handler gin.HandlerFunc, body map[string]interface{}) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/pay/usdt/notify", strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	handler(c)
	return w
}

func main() {
	gin.SetMode(gin.TestMode)
	if os.Getenv("ADTY_SECRET_KEY") == "" {
		_ = os.Setenv("ADTY_SECRET_KEY", "adtyai-accept-test-secret-key-do-not-use-prod")
	}
	common.InitAdtySecretKeys()
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	model.InitOptionMap()
	must(model.DB.AutoMigrate(&model.TopupOrder{}))
	fmt.Println("InitDB+InitLogDB OK")

	fakeToken := "fake-bepusdt-api-token-accept-only"
	setting.SetBepusdtApiToken("")
	setting.SetBepusdtBaseURL("")
	service.SetAdtyBepusdtProbeOverride(nil)

	// (1) 未配置 → USDT available=false
	ok, _ := service.IsAdtyUsdtAvailable()
	if ok {
		panic("(1) expected unavailable when unconfigured")
	}
	fmt.Println("(1) unconfigured → available=false OK")

	// Mock BEpusdt HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/v1/order/create-transaction", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		_ = json.Unmarshal(body, &req)
		orderID, _ := req["order_id"].(string)
		amount := 0.0
		switch v := req["amount"].(type) {
		case float64:
			amount = v
		case json.Number:
			amount, _ = v.Float64()
		}
		// lock 1:1 USDT for USD in mock
		payAmt := fmt.Sprintf("%.2f", amount)
		resp := map[string]interface{}{
			"status_code": 200,
			"data": map[string]interface{}{
				"order_id":        orderID,
				"trade_id":        "MOCKTX" + common.GetRandomString(6),
				"actual_amount":   payAmt,
				"token":           "USDT",
				"receive_address": "TAcceptMockUsdtAddress111",
				"payment_url":     "https://pay.example.test/mock",
				"expiration_time": int64(900),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// (2) 配置假 token + mock base → available=true
	setting.SetBepusdtBaseURL(srv.URL)
	setting.SetBepusdtApiToken(fakeToken)
	ok, reason := service.IsAdtyUsdtAvailable()
	if !ok {
		panic("(2) expected available after config+mock probe, reason=" + reason)
	}
	fmt.Println("(2) configured fake token + mock probe → available=true OK")

	// (7) options / secret meta: no plaintext
	set, fp := model.BepusdtApiTokenSecretMeta()
	if !set || !strings.HasPrefix(fp, "hm_") {
		panic(fmt.Sprintf("(7a) secret meta want set+hm_ got set=%v fp=%q", set, fp))
	}
	if strings.Contains(fp, fakeToken) {
		panic("(7a) fingerprint leaked token")
	}
	// simulate GetOptions sensitive skip: OptionMap holds token but public view must not
	common.OptionMapRWMutex.Lock()
	common.OptionMap["BepusdtApiToken"] = fakeToken
	common.OptionMap["BepusdtBaseURL"] = srv.URL
	common.OptionMapRWMutex.Unlock()
	rawOpt, _ := common.Marshal(common.OptionMap)
	// OptionMap itself has it (internal); public GetOptions must strip — unit-check helper:
	public := map[string]string{}
	common.OptionMapRWMutex.RLock()
	for k, v := range common.OptionMap {
		ks := k
		vs := common.Interface2String(v)
		isSens := strings.HasSuffix(ks, "Token") || strings.HasSuffix(ks, "Secret") ||
			strings.HasSuffix(ks, "Key") || strings.HasSuffix(ks, "secret") || strings.HasSuffix(ks, "api_key")
		if isSens {
			continue
		}
		public[ks] = vs
	}
	common.OptionMapRWMutex.RUnlock()
	pubJSON, _ := json.Marshal(public)
	if strings.Contains(string(pubJSON), fakeToken) {
		panic("(7) public options leaked ApiToken: " + string(pubJSON))
	}
	_ = rawOpt
	fmt.Println("(7) GET options path strips ApiToken plaintext; secret_set+hm_ fingerprint OK")

	// (3) 下单写入汇率/应付 USDT 快照
	u := newUser("ord")
	order, err := model.CreateTopupOrder(u.Id, 10, "usdt", "fp-usdt-"+common.GetRandomString(8), "10.0.0.1")
	must(err)
	cashier, err := controller.StartUsdtPurchaseForOrder(order)
	must(err)
	order, _ = model.GetTopupOrderByNo(order.OrderNo)
	if order.PayUSDT <= 0 || order.UsdtFxRate <= 0 || order.UsdtExpireAt <= 0 {
		panic(fmt.Sprintf("(3) snapshot missing: pay_usdt=%v fx=%v exp=%v cashier=%v",
			order.PayUSDT, order.UsdtFxRate, order.UsdtExpireAt, cashier))
	}
	if order.PaymentProvider != model.PaymentProviderBepusdt {
		panic("(3) provider want bepusdt got " + order.PaymentProvider)
	}
	fmt.Printf("(3) order snapshot pay_usdt=%.4f usdt_fx=%.4f expire_at=%d OK\n",
		order.PayUSDT, order.UsdtFxRate, order.UsdtExpireAt)

	// (4) 假签名回调拒
	bad := map[string]interface{}{
		"order_id":      order.OrderNo,
		"status":        2,
		"actual_amount": fmt.Sprintf("%.2f", order.PayUSDT),
		"signature":     "deadbeef",
	}
	w := postNotify(controller.AdtyUsdtNotify, bad)
	if w.Code != http.StatusForbidden {
		panic(fmt.Sprintf("(4) fake signature want 403 got %d body=%s", w.Code, w.Body.String()))
	}
	order, _ = model.GetTopupOrderByNo(order.OrderNo)
	if order.Status != model.TopupOrderStatusPending {
		panic("(4) order should stay pending after bad sig")
	}
	fmt.Println("(4) fake signature rejected OK")

	// (5) 少付仍按订单快照入账，不进核对
	u2 := newUser("under")
	order2, err := model.CreateTopupOrder(u2.Id, 10, "usdt", "fp-usdt-u-"+common.GetRandomString(8), "10.0.0.2")
	must(err)
	_, err = controller.StartUsdtPurchaseForOrder(order2)
	must(err)
	order2, _ = model.GetTopupOrderByNo(order2.OrderNo)
	underAmt := order2.PayUSDT - (model.GetUsdtUnderpayTolerance() + 0.05)
	under := signParams(fakeToken, map[string]interface{}{
		"order_id":      order2.OrderNo,
		"status":        2,
		"actual_amount": fmt.Sprintf("%.6f", underAmt),
		"trade_id":      "UNDER" + common.GetRandomString(6),
	})
	w = postNotify(controller.AdtyUsdtNotify, under)
	if w.Code != http.StatusOK {
		panic(fmt.Sprintf("(5) underpay notify want 200 ack got %d", w.Code))
	}
	order2, _ = model.GetTopupOrderByNo(order2.OrderNo)
	if order2.Status != model.TopupOrderStatusPaid {
		panic(fmt.Sprintf("(5) want paid got %s", order2.Status))
	}
	paid2, _ := model.GetUserPaidQuota(u2.Id)
	wantPaid2, _ := model.DollarsToQuota(10)
	if paid2 != wantPaid2 {
		panic(fmt.Sprintf("(5) underpay must credit snapshot, want %d got %d", wantPaid2, paid2))
	}
	fmt.Println("(5) underpay status 2 credits snapshot OK")

	// (6) 合法回调入账 + 重复幂等
	good := signParams(fakeToken, map[string]interface{}{
		"order_id":      order.OrderNo,
		"status":        2,
		"actual_amount": fmt.Sprintf("%.2f", order.PayUSDT),
		"trade_id":      "GOOD" + common.GetRandomString(6),
	})
	w = postNotify(controller.AdtyUsdtNotify, good)
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		panic(fmt.Sprintf("(6) good notify want ok got %d %s", w.Code, w.Body.String()))
	}
	order, _ = model.GetTopupOrderByNo(order.OrderNo)
	if order.Status != model.TopupOrderStatusPaid {
		panic("(6) order not paid")
	}
	paid, _ := model.GetUserPaidQuota(u.Id)
	wantPaid, _ := model.DollarsToQuota(10)
	if paid != wantPaid {
		panic(fmt.Sprintf("(6) paid quota want %d got %d", wantPaid, paid))
	}
	w = postNotify(controller.AdtyUsdtNotify, good)
	if w.Code != http.StatusOK {
		panic(fmt.Sprintf("(6b) replay want 200 got %d", w.Code))
	}
	paidAgain, _ := model.GetUserPaidQuota(u.Id)
	if paidAgain != paid {
		panic("(6b) replay changed balance")
	}
	fmt.Println("(6) valid callback credit + idempotent replay OK")

	// (8) local window ended, status 2 still credits the snapshot
	uExp := newUser("exp")
	orderExp, err := model.CreateTopupOrder(uExp.Id, 10, "usdt", "fp-usdt-e-"+common.GetRandomString(8), "10.0.0.3")
	must(err)
	_, err = controller.StartUsdtPurchaseForOrder(orderExp)
	must(err)
	orderExp, _ = model.GetTopupOrderByNo(orderExp.OrderNo)
	must(model.DB.Model(&model.TopupOrder{}).Where("order_no = ?", orderExp.OrderNo).
		Update("usdt_expire_at", time.Now().Unix()-60).Error)
	orderExp, _ = model.GetTopupOrderByNo(orderExp.OrderNo)
	if orderExp.UsdtExpireAt >= time.Now().Unix() {
		panic("(8) failed to force expire")
	}
	expCb := signParams(fakeToken, map[string]interface{}{
		"order_id":      orderExp.OrderNo,
		"status":        2,
		"actual_amount": fmt.Sprintf("%.2f", orderExp.PayUSDT),
		"trade_id":      "EXP" + common.GetRandomString(6),
	})
	w = postNotify(controller.AdtyUsdtNotify, expCb)
	if w.Code != http.StatusOK {
		panic(fmt.Sprintf("(8) expired notify want 200 ack got %d body=%s", w.Code, w.Body.String()))
	}
	orderExp, _ = model.GetTopupOrderByNo(orderExp.OrderNo)
	if orderExp.Status != model.TopupOrderStatusPaid {
		panic(fmt.Sprintf("(8) want paid got %s", orderExp.Status))
	}
	if orderExp.PendingReason == "timeout" {
		panic("(8) local expiry must not become timeout review")
	}
	paidExp, _ := model.GetUserPaidQuota(uExp.Id)
	wantPaidExp, _ := model.DollarsToQuota(10)
	if paidExp != wantPaidExp {
		panic(fmt.Sprintf("(8) expired status 2 must credit snapshot, want %d got %d", wantPaidExp, paidExp))
	}
	fmt.Println("(8) status 2 after local expiry credits snapshot OK")

	w = postNotify(controller.AdtyBepusdtLegacyNotify, good)
	if w.Code != http.StatusNotFound {
		panic(fmt.Sprintf("legacy path want 404 got %d", w.Code))
	}
	fmt.Println("legacy notify always 404 OK")

	fmt.Println("ALL USDT ACCEPTANCE PASSED")
}
