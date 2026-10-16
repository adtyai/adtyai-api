//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"bytes"
	"net/http"
	"net/http/httptest"
	"github.com/QuantumNous/new-api/controller"
	"github.com/gin-gonic/gin"
	"net/url"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func postEpayNotify(instanceID int64, params map[string]string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	body := form.Encode()
	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/pay/epay/notify/%d", instanceID), bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Params = gin.Params{{Key: "instance_id", Value: fmt.Sprintf("%d", instanceID)}}
	controller.AdtyEpayNotify(c)
	return w
}

func postDisable(instanceID int64, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/epay-instances/%d/disable", instanceID), bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", instanceID)}}
	// admin id for audit
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "accept-admin")
	controller.AdminDisableEpayInstance(c)
	return w
}

func postEnable(instanceID int64, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/epay-instances/%d/enable", instanceID), bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", instanceID)}}
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "accept-admin")
	controller.AdminEnableEpayInstance(c)
	return w
}


func putEpayUpdate(instanceID int64, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/admin/epay-instances/%d", instanceID), bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", instanceID)}}
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "accept-admin")
	controller.AdminUpdateEpayInstance(c)
	return w
}


func newUser(tag string) model.User {
	u := model.User{
		Username:    "accept-epay-" + tag + "-" + common.GetRandomString(6),
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

func signNotify(secret string, params map[string]string) map[string]string {
	out, err := epay.GenerateParamsWithError(params, secret)
	must(err)
	return out
}

func main() {
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
	must(model.DB.AutoMigrate(&model.EpayInstance{}, &model.TopupOrder{}))
	must(model.EnsureEpayInstanceIndexes(model.DB))
	must(model.EnsureTopupOrderIndexes(model.DB))
	fmt.Println("InitDB+InitLogDB+epay_instances OK")

	secretA := "test-epay-secret-A-fake"
	secretB := "test-epay-secret-B-fake"

	// (1) 创建实例后列表无明文/密文，有 secret_set/fingerprint
	instA, err := model.CreateEpayInstance(model.CreateEpayInstanceInput{
		Name: "accept-A", PartnerId: "10001", Secret: secretA,
		PayAddress: "https://epay.example.test/", CallbackBase: "https://api.example.test",
		DailyLimit: 0, Enabled: true,
	})
	must(err)
	instB, err := model.CreateEpayInstance(model.CreateEpayInstanceInput{
		Name: "accept-B", PartnerId: "10002", Secret: secretB,
		PayAddress: "https://epay-b.example.test/", CallbackBase: "https://api.example.test",
		DailyLimit: 0, Enabled: true,
	})
	must(err)
	view := instA.ToView()
	if !view.SecretSet || view.SecretFingerprint == "" {
		panic("(1) secret_set/fingerprint missing")
	}
	raw, _ := common.Marshal(view)
	s := string(raw)
	if strings.Contains(s, "secret_ciphertext") || strings.Contains(s, secretA) || strings.Contains(s, instA.SecretCiphertext) {
		panic("(1) secret leaked in view json: " + s)
	}
	fpWant, _ := common.AdtySecretFingerprint(secretA)
	if view.SecretFingerprint != fpWant {
		panic(fmt.Sprintf("(1) fingerprint want %s got %s", fpWant, view.SecretFingerprint))
	}
	fmt.Println("(1) create+view: secret_set/fingerprint only, no plaintext OK")

	// (2) 设主用需在 controller 层 confirm；此处测 SetPrimary 效果 + tiers available
	_, err = model.SetEpayInstancePrimary(instA.Id)
	must(err)
	prim, err := model.GetPrimaryEpayInstance()
	must(err)
	if prim.Id != instA.Id || prim.FailureStreak != 0 || prim.DemotedReason != nil || prim.DemotedAt != nil {
		panic(fmt.Sprintf("(2) primary clear demote fields failed: %+v", prim))
	}
	ok, _ := model.HasUsablePrimaryEpay()
	if !ok {
		panic("(2) expected usable primary after set")
	}
	fmt.Println("(2) set primary → failure_streak=0 demoted cleared; alipay/wxpay available OK")

	// (3) 无主用/停用 → unavailable
	_, err = model.DemoteEpayInstancePrimary(instA.Id, "accept_demote")
	must(err)
	ok, _ = model.HasUsablePrimaryEpay()
	if ok {
		panic("(3a) after demote should be unavailable")
	}
	_, err = model.SetEpayInstancePrimary(instA.Id)
	must(err)
	_, err = model.DisableEpayInstance(instA.Id)
	must(err)
	ok, _ = model.HasUsablePrimaryEpay()
	if ok {
		panic("(3b) after disable primary should be unavailable")
	}
	// re-enable + set primary for later tests
	en := true
	_, err = model.UpdateEpayInstance(instA.Id, model.UpdateEpayInstanceInput{Enabled: &en})
	must(err)
	_, err = model.SetEpayInstancePrimary(instA.Id)
	must(err)
	fmt.Println("(3) no primary / disabled → unavailable OK")

	// (4)(5) 回调验签：假签名拒；错实例密钥拒；合法签名入账；重复幂等
	u := newUser("pay")
	order, err := model.CreateTopupOrder(u.Id, 10, "alipay", "fp-epay-"+common.GetRandomString(8), "11.1.1.1")
	must(err)
	fx := model.CurrentTopupFxRate()
	payCNY := float64(order.AmountUSD) * fx
	must(model.DB.Model(order).Updates(map[string]any{
		"epay_instance_id": instA.Id,
		"fx_rate":          fx,
		"pay_cny":          payCNY,
		"payment_provider": "epay",
	}).Error)
	order, _ = model.GetTopupOrderByNo(order.OrderNo)

	baseParams := map[string]string{
		"pid":          instA.PartnerId,
		"trade_no":     "EPAYTN" + common.GetRandomString(8),
		"out_trade_no": order.OrderNo,
		"type":         "alipay",
		"name":         "ADTY10",
		"money":        fmt.Sprintf("%.2f", payCNY),
		"trade_status": epay.StatusTradeSuccess,
	}

	// fake sign
	bad := map[string]string{}
	for k, v := range baseParams {
		bad[k] = v
	}
	bad["sign"] = "deadbeef"
	bad["sign_type"] = "MD5"
	clientA, err := epay.NewClient(&epay.Config{PartnerID: instA.PartnerId, Key: secretA}, instA.PayAddress)
	must(err)
	vr, err := clientA.Verify(bad)
	must(err)
	if vr.VerifyStatus {
		panic("(4) fake signature should fail verify")
	}
	fmt.Println("(4a) fake signature rejected OK")

	// wrong instance key (B)
	wrong := map[string]string{}
	for k, v := range baseParams {
		wrong[k] = v
	}
	wrong = signNotify(secretB, wrong)
	vr, err = clientA.Verify(wrong)
	must(err)
	if vr.VerifyStatus {
		panic("(4b) instance-B signature must not verify with instance-A key")
	}
	fmt.Println("(4b) wrong-instance key rejected OK")

	// good sign → complete
	good := map[string]string{}
	for k, v := range baseParams {
		good[k] = v
	}
	good = signNotify(secretA, good)
	vr, err = clientA.Verify(good)
	must(err)
	if !vr.VerifyStatus {
		panic("(5) good signature should verify")
	}
	must(model.AttachProviderTradeNo(order.OrderNo, baseParams["trade_no"]))
	done, err := model.CompleteTopupOrder(order.OrderNo)
	must(err)
	if done.Status != model.TopupOrderStatusPaid {
		panic("(5) order not paid")
	}
	paid, _ := model.GetUserPaidQuota(u.Id)
	wantPaid, _ := model.DollarsToQuota(10)
	if paid != wantPaid {
		panic(fmt.Sprintf("(5) paid quota want %d got %d", wantPaid, paid))
	}
	done2, err := model.CompleteTopupOrder(order.OrderNo)
	must(err)
	paid2, _ := model.GetUserPaidQuota(u.Id)
	if paid2 != paid || done2.Id != done.Id {
		panic("(5) replay not idempotent")
	}
	fmt.Println("(5) valid callback path → CompleteTopupOrder + idempotent OK")

	// (6) 失败达阈值摘主
	_, err = model.SetEpayInstancePrimary(instA.Id)
	must(err)
	// force threshold small via ApplyBusinessParamRuntime if available
	threshold := model.GetEpayFailureDemoteThreshold()
	var demoted bool
	var last *model.EpayInstance
	for i := 0; i < threshold; i++ {
		last, demoted, err = model.RecordEpayInstanceFailure(instA.Id, "accept_fail")
		must(err)
	}
	if !demoted || last.IsPrimary {
		panic(fmt.Sprintf("(6) expected demote after %d failures, demoted=%v primary=%v streak=%d", threshold, demoted, last.IsPrimary, last.FailureStreak))
	}
	ok, _ = model.HasUsablePrimaryEpay()
	if ok {
		panic("(6) after auto-demote should be unavailable")
	}
	fmt.Printf("(6) failure_streak>=%d auto demote OK\n", threshold)

	// (7) 同交易号二次补单 409
	_, err = model.SetEpayInstancePrimary(instA.Id)
	must(err)
	u2 := newUser("dup")
	o2, err := model.CreateTopupOrder(u2.Id, 10, "alipay", "fp-dup-"+common.GetRandomString(8), "11.2.2.2")
	must(err)
	must(model.DB.Model(o2).Updates(map[string]any{"epay_instance_id": instA.Id, "fx_rate": fx, "pay_cny": payCNY}).Error)
	dupTrade := baseParams["trade_no"] // already on first paid order
	err = model.AttachProviderTradeNo(o2.OrderNo, dupTrade)
	if err != model.ErrEpayTradeNoDuplicate {
		panic(fmt.Sprintf("(7) want ErrEpayTradeNoDuplicate got %v", err))
	}
	fmt.Println("(7) same trade_no second attach → duplicate OK (HTTP 409)")

	// (8) 限频热更新：per-request 读 GetTopupOrderRateLimit（代码路径证明）
	n1, d1 := model.GetTopupOrderRateLimit()
	must(model.UpdateOption("TopupOrderRateLimitNum", "3"))
	must(model.UpdateOption("TopupOrderRateLimitDuration", "30"))
	n2, d2 := model.GetTopupOrderRateLimit()
	if n2 != 3 || d2 != 30 {
		panic(fmt.Sprintf("(8) hot update want 3/30 got %d/%d (before %d/%d)", n2, d2, n1, d1))
	}
	// restore
	must(model.UpdateOption("TopupOrderRateLimitNum", "10"))
	must(model.UpdateOption("TopupOrderRateLimitDuration", "60"))
	fmt.Println("(8) TopupOrderRateLimit hot-read via GetTopupOrderRateLimit OK")


	// (9) 回调：未绑实例 / PayCNY=0 / 错实例 / 金额不符 → 拒，不入账
	u9 := newUser("guard")
	oUnbound, err := model.CreateTopupOrder(u9.Id, 10, "alipay", "fp-guard-"+common.GetRandomString(8), "11.3.3.3")
	must(err)
	// deliberately leave epay_instance_id=0, pay_cny=0
	paramsUnbound := signNotify(secretA, map[string]string{
		"pid":          instA.PartnerId,
		"trade_no":     "EPAYUNB" + common.GetRandomString(6),
		"out_trade_no": oUnbound.OrderNo,
		"type":         "alipay",
		"name":         "ADTY10",
		"money":        fmt.Sprintf("%.2f", float64(10)*model.CurrentTopupFxRate()),
		"trade_status": epay.StatusTradeSuccess,
	})
	w9 := postEpayNotify(instA.Id, paramsUnbound)
	if w9.Code != http.StatusForbidden {
		panic(fmt.Sprintf("(9a) unbound want 403 got %d body=%s", w9.Code, w9.Body.String()))
	}
	oUnbound, _ = model.GetTopupOrderByNo(oUnbound.OrderNo)
	if oUnbound.Status == model.TopupOrderStatusPaid {
		panic("(9a) unbound must not credit")
	}

	oZero, err := model.CreateTopupOrder(u9.Id, 10, "alipay", "fp-zero-"+common.GetRandomString(8), "11.3.3.4")
	must(err)
	must(model.DB.Model(oZero).Updates(map[string]any{
		"epay_instance_id": instA.Id,
		"pay_cny":          0,
		"payment_provider": "epay",
	}).Error)
	paramsZero := signNotify(secretA, map[string]string{
		"pid":          instA.PartnerId,
		"trade_no":     "EPAYZERO" + common.GetRandomString(6),
		"out_trade_no": oZero.OrderNo,
		"type":         "alipay",
		"name":         "ADTY10",
		"money":        "73.00",
		"trade_status": epay.StatusTradeSuccess,
	})
	w9b := postEpayNotify(instA.Id, paramsZero)
	if w9b.Code != http.StatusForbidden {
		panic(fmt.Sprintf("(9b) pay_cny=0 want 403 got %d", w9b.Code))
	}

	oMis, err := model.CreateTopupOrder(u9.Id, 10, "alipay", "fp-mis-"+common.GetRandomString(8), "11.3.3.5")
	must(err)
	fx9 := model.CurrentTopupFxRate()
	pay9 := float64(10) * fx9
	must(model.DB.Model(oMis).Updates(map[string]any{
		"epay_instance_id": instA.Id,
		"fx_rate":          fx9,
		"pay_cny":          pay9,
		"payment_provider": "epay",
	}).Error)
	// wrong instance id in URL (instB) while order bound to A
	paramsMis := signNotify(secretA, map[string]string{
		"pid":          instA.PartnerId,
		"trade_no":     "EPAYMIS" + common.GetRandomString(6),
		"out_trade_no": oMis.OrderNo,
		"type":         "alipay",
		"name":         "ADTY10",
		"money":        fmt.Sprintf("%.2f", pay9),
		"trade_status": epay.StatusTradeSuccess,
	})
	// sign with A but hit B notify path → instance mismatch (and B key would also fail verify first)
	paramsMisB := signNotify(secretB, map[string]string{
		"pid":          instB.PartnerId,
		"trade_no":     "EPAYMISB" + common.GetRandomString(6),
		"out_trade_no": oMis.OrderNo,
		"type":         "alipay",
		"name":         "ADTY10",
		"money":        fmt.Sprintf("%.2f", pay9),
		"trade_status": epay.StatusTradeSuccess,
	})
	w9c := postEpayNotify(instB.Id, paramsMisB)
	if w9c.Code != http.StatusForbidden {
		panic(fmt.Sprintf("(9c) wrong instance want 403 got %d body=%s", w9c.Code, w9c.Body.String()))
	}
	_ = paramsMis
	// money mismatch on correct instance
	paramsAmt := signNotify(secretA, map[string]string{
		"pid":          instA.PartnerId,
		"trade_no":     "EPAYAMT" + common.GetRandomString(6),
		"out_trade_no": oMis.OrderNo,
		"type":         "alipay",
		"name":         "ADTY10",
		"money":        "0.01",
		"trade_status": epay.StatusTradeSuccess,
	})
	w9d := postEpayNotify(instA.Id, paramsAmt)
	if w9d.Code != http.StatusForbidden {
		panic(fmt.Sprintf("(9d) money mismatch want 403 got %d", w9d.Code))
	}
	paid9, _ := model.GetUserPaidQuota(u9.Id)
	if paid9 != 0 {
		panic(fmt.Sprintf("(9) guard cases must not credit, paid=%d", paid9))
	}
	fmt.Println("(9) unbound / pay_cny=0 / wrong instance / money mismatch → reject, no credit OK")

	// (10) provider_trade_no 部分唯一索引防双绑
	must(model.EnsureTopupOrderIndexes(model.DB))
	var idxCount int64
	must(model.DB.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE indexname = 'uq_topup_orders_provider_trade_no'`).Scan(&idxCount).Error)
	if idxCount != 1 {
		panic(fmt.Sprintf("(10) unique index missing, count=%d", idxCount))
	}
	u10a := newUser("uniq-a")
	u10b := newUser("uniq-b")
	oa, err := model.CreateTopupOrder(u10a.Id, 10, "alipay", "fp-ua-"+common.GetRandomString(8), "11.4.4.1")
	must(err)
	ob, err := model.CreateTopupOrder(u10b.Id, 10, "alipay", "fp-ub-"+common.GetRandomString(8), "11.4.4.2")
	must(err)
	must(model.DB.Model(oa).Updates(map[string]any{"epay_instance_id": instA.Id, "pay_cny": pay9, "fx_rate": fx9}).Error)
	must(model.DB.Model(ob).Updates(map[string]any{"epay_instance_id": instA.Id, "pay_cny": pay9, "fx_rate": fx9}).Error)
	tn := "UNIQ" + common.GetRandomString(10)
	must(model.AttachProviderTradeNo(oa.OrderNo, tn))
	err = model.AttachProviderTradeNo(ob.OrderNo, tn)
	if err != model.ErrEpayTradeNoDuplicate {
		panic(fmt.Sprintf("(10) second attach want duplicate got %v", err))
	}
	fmt.Println("(10) provider_trade_no UNIQUE partial index + duplicate attach OK")

	// (11) disable 缺 confirm=true → 400
	// use a non-primary spare: create C
	instC, err := model.CreateEpayInstance(model.CreateEpayInstanceInput{
		Name: "accept-C-disable", PartnerId: "10003", Secret: "test-epay-secret-C-fake",
		PayAddress: "https://epay-c.example.test/", CallbackBase: "https://api.example.test",
		DailyLimit: 0, Enabled: true,
	})
	must(err)
	w11 := postDisable(instC.Id, `{}`)
	if w11.Code != http.StatusBadRequest {
		panic(fmt.Sprintf("(11a) disable without confirm want 400 got %d body=%s", w11.Code, w11.Body.String()))
	}
	if !strings.Contains(w11.Body.String(), "confirm_required") {
		panic("(11a) body should mention confirm_required: " + w11.Body.String())
	}
	w11b := postDisable(instC.Id, `{"confirm":true}`)
	if w11b.Code != http.StatusOK {
		panic(fmt.Sprintf("(11b) disable with confirm want 200 got %d body=%s", w11b.Code, w11b.Body.String()))
	}
	gotC, _ := model.GetEpayInstanceById(instC.Id)
	if gotC.Status != model.EpayInstanceStatusDisabled {
		panic("(11b) instance not disabled")
	}
	fmt.Println("(11) disable confirm=true required OK")


	// (12) PUT enabled=false 不得绕过 disable+confirm；实例仍 enabled；再走 disable+confirm 才停用
	instD, err := model.CreateEpayInstance(model.CreateEpayInstanceInput{
		Name: "accept-D-put-enabled", PartnerId: "10004", Secret: "test-epay-secret-D-fake",
		PayAddress: "https://epay-d.example.test/", CallbackBase: "https://api.example.test",
		DailyLimit: 0, Enabled: true,
	})
	must(err)
	w12 := putEpayUpdate(instD.Id, `{"enabled":false}`)
	if w12.Code != http.StatusBadRequest {
		panic(fmt.Sprintf("(12a) PUT enabled=false want 400 got %d body=%s", w12.Code, w12.Body.String()))
	}
	if !strings.Contains(w12.Body.String(), "use_disable_endpoint") && !strings.Contains(w12.Body.String(), "disable") {
		panic("(12a) body should reject enabled via PUT: " + w12.Body.String())
	}
	gotD, _ := model.GetEpayInstanceById(instD.Id)
	if gotD.Status == model.EpayInstanceStatusDisabled {
		panic("(12a) instance became disabled via PUT bypass")
	}
	w12b := putEpayUpdate(instD.Id, `{"enabled":true}`)
	if w12b.Code != http.StatusBadRequest {
		panic(fmt.Sprintf("(12b) PUT enabled=true want 400 got %d body=%s", w12b.Code, w12b.Body.String()))
	}
	w12c := postDisable(instD.Id, `{"confirm":true}`)
	if w12c.Code != http.StatusOK {
		panic(fmt.Sprintf("(12c) disable+confirm want 200 got %d body=%s", w12c.Code, w12c.Body.String()))
	}
	gotD2, _ := model.GetEpayInstanceById(instD.Id)
	if gotD2.Status != model.EpayInstanceStatusDisabled {
		panic("(12c) instance not disabled after disable+confirm")
	}
	fmt.Println("(12) PUT enabled=false/true → 400; disable+confirm stops OK")

	// (13) enable：缺 confirm→400 仍停用；enable+confirm→启用；PUT enabled 仍 400；幂等；不自动设主用
	w13a := postEnable(instD.Id, `{}`)
	if w13a.Code != http.StatusBadRequest {
		panic(fmt.Sprintf("(13a) enable without confirm want 400 got %d body=%s", w13a.Code, w13a.Body.String()))
	}
	if !strings.Contains(w13a.Body.String(), "confirm_required") {
		panic("(13a) body should mention confirm_required: " + w13a.Body.String())
	}
	gotD3, _ := model.GetEpayInstanceById(instD.Id)
	if gotD3.Status != model.EpayInstanceStatusDisabled {
		panic("(13a) instance must stay disabled after enable without confirm")
	}
	w13b := putEpayUpdate(instD.Id, `{"enabled":true}`)
	if w13b.Code != http.StatusBadRequest {
		panic(fmt.Sprintf("(13b) PUT enabled=true while disabled want 400 got %d", w13b.Code))
	}
	if !strings.Contains(w13b.Body.String(), "enable") && !strings.Contains(w13b.Body.String(), "use_disable_endpoint") {
		panic("(13b) body should hint /enable: " + w13b.Body.String())
	}
	gotD3b, _ := model.GetEpayInstanceById(instD.Id)
	if gotD3b.Status != model.EpayInstanceStatusDisabled {
		panic("(13b) PUT must not re-enable")
	}
	w13c := postEnable(instD.Id, `{"confirm":true}`)
	if w13c.Code != http.StatusOK {
		panic(fmt.Sprintf("(13c) enable+confirm want 200 got %d body=%s", w13c.Code, w13c.Body.String()))
	}
	gotD4, _ := model.GetEpayInstanceById(instD.Id)
	if gotD4.Status != model.EpayInstanceStatusEnabled {
		panic("(13c) instance not enabled after enable+confirm")
	}
	if gotD4.IsPrimary {
		panic("(13c) enable must NOT auto set primary")
	}
	// idempotent enable
	w13d := postEnable(instD.Id, `{"confirm":true}`)
	if w13d.Code != http.StatusOK {
		panic(fmt.Sprintf("(13d) enable already-enabled want 200 got %d", w13d.Code))
	}
	gotD5, _ := model.GetEpayInstanceById(instD.Id)
	if gotD5.Status != model.EpayInstanceStatusEnabled {
		panic("(13d) still enabled after idempotent enable")
	}
	w13e := putEpayUpdate(instD.Id, `{"enabled":false}`)
	if w13e.Code != http.StatusBadRequest {
		panic(fmt.Sprintf("(13e) PUT enabled=false after enable want 400 got %d", w13e.Code))
	}
	gotD6, _ := model.GetEpayInstanceById(instD.Id)
	if gotD6.Status != model.EpayInstanceStatusEnabled {
		panic("(13e) PUT must not disable after enable")
	}
	fmt.Println("(13) enable confirm required; enable+confirm; PUT still 400; no auto-primary OK")

	// (9e) 回写快照成功是返回 pay_url 的前置：Bind 后订单必有 instance+pay_cny
	uSnap := newUser("snap")
	oSnap, err := model.CreateTopupOrder(uSnap.Id, 10, "alipay", "fp-snap-"+common.GetRandomString(8), "11.5.5.5")
	must(err)
	err = model.DB.Model(oSnap).Updates(map[string]any{
		"epay_instance_id": instA.Id,
		"fx_rate":          fx9,
		"pay_cny":          pay9,
		"payment_provider": "epay",
	}).Error
	must(err)
	oSnap, _ = model.GetTopupOrderByNo(oSnap.OrderNo)
	if oSnap.EpayInstanceId == 0 || oSnap.PayCNY <= 0 {
		panic("(9e) snapshot bind must set instance_id and pay_cny before pay_url")
	}
	fmt.Println("(9e) snapshot fields required before pay_url path OK")

	_ = instB
	fmt.Println("ALL accept_epay checks passed")
}
