//go:build ignore

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func refresh() map[string]model.HealthProbeView {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/health/refresh", nil)
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	controller.AdminRefreshHealth(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("refresh status %d body=%s", w.Code, w.Body.String()))
	}
	var resp struct {
		Success bool                        `json:"success"`
		Data    model.HealthSummaryResponse `json:"data"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &resp))
	out := map[string]model.HealthProbeView{}
	for _, p := range resp.Data.Probes {
		out[p.TargetType+"/"+p.TargetId] = p
	}
	return out
}

func setOpt(k, v string) {
	common.OptionMapRWMutex.Lock()
	common.OptionMap[k] = v
	common.OptionMapRWMutex.Unlock()
}

func want(m map[string]model.HealthProbeView, key string, ok bool, errSub, step string) {
	v, has := m[key]
	if !has {
		panic(fmt.Sprintf("%s %s missing in summary", step, key))
	}
	if v.Ok != ok {
		panic(fmt.Sprintf("%s %s ok want %v got %v err=%q", step, key, ok, v.Ok, v.Error))
	}
	if errSub != "" && !strings.Contains(v.Error, errSub) {
		panic(fmt.Sprintf("%s %s error want contains %q got %q", step, key, errSub, v.Error))
	}
	if strings.Contains(v.Error, "secret") || strings.Contains(v.Error, "token=") {
		panic(fmt.Sprintf("%s %s error leaks: %q", step, key, v.Error))
	}
}

func createAdmin(body string) (int, map[string]any) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/admins", bytes.NewBufferString(body))
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "accept-wire-root")
	controller.AdminCreateAdmin(c)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w.Code, m
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
	must(model.EnsureOpsJobRunsTable(model.DB))
	must(model.EnsureOpsAlertTable(model.DB))
	must(model.EnsureHealthProbesTable(model.DB))
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	fmt.Println("InitDB OK")

	origBep := setting.GetBepusdtBaseURL()
	origCwBase := common.OptionMap[model.OptChatwootBaseUrl]
	origCwScript := common.OptionMap[model.OptChatwootScript]
	defer func() {
		setting.SetBepusdtBaseURL(origBep)
		setOpt(model.OptChatwootBaseUrl, origCwBase)
		setOpt(model.OptChatwootScript, origCwScript)
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	inst, err := model.CreateEpayInstance(model.CreateEpayInstanceInput{
		Name: "accept-wire-epay", PartnerId: "1000", Secret: "accept-wire-secret",
		PayAddress: srv.URL + "/", Enabled: true,
	})
	must(err)
	defer model.DB.Delete(&model.EpayInstance{}, inst.Id)
	epayKey := "epay/" + strconv.FormatInt(inst.Id, 10)

	// (1) fake server → ok=true for all three; chatwoot via script src parse
	setting.SetBepusdtBaseURL(srv.URL)
	setOpt(model.OptChatwootBaseUrl, "")
	setOpt(model.OptChatwootScript, `<script>(function(d,t){var BASE_URL="`+srv.URL+`";var g=d.createElement(t);g.src=BASE_URL+"/packs/js/sdk.js";})(document,"script");</script>`)
	if got := model.ChatwootProbeURL(); got != srv.URL {
		panic(fmt.Sprintf("(1) chatwoot url parse got %q", got))
	}
	m := refresh()
	want(m, epayKey, true, "http 200", "(1)")
	want(m, "bepusdt/main", true, "http 200", "(1)")
	want(m, "chatwoot/main", true, "http 200", "(1)")
	want(m, "postgres/main", true, "", "(1)")
	for _, k := range []string{epayKey, "bepusdt/main", "chatwoot/main", "postgres/main"} {
		if m[k].Status != "ok" {
			panic(fmt.Sprintf("(1) %s status want ok got %q", k, m[k].Status))
		}
	}
	fmt.Println("(1) fake server → epay/bepusdt/chatwoot ok=true status=ok OK")

	// (2) empty config → rows with skipped（先清旧行，确保无先前真实失败）
	model.DB.Where("(target_type = ? AND target_id = ?) OR (target_type = ? AND target_id = ?)",
		"bepusdt", "main", "chatwoot", "main").Delete(&model.HealthProbe{})
	setting.SetBepusdtBaseURL("")
	setOpt(model.OptChatwootScript, "")
	setOpt(model.OptChatwootBaseUrl, "")
	m = refresh()
	want(m, "bepusdt/main", false, "skipped", "(2)")
	want(m, "chatwoot/main", false, "skipped", "(2)")
	for _, k := range []string{"bepusdt/main", "chatwoot/main"} {
		if m[k].Status != "skipped" {
			panic(fmt.Sprintf("(2) %s status want skipped got %q", k, m[k].Status))
		}
		if m[k].LastFailureAt != nil {
			panic(fmt.Sprintf("(2) %s skipped must not set last_failure_at", k))
		}
	}
	fmt.Println("(2) empty config → skipped ok=false status=skipped last_failure_at=null OK")

	// (3) server down → ok=false, short reason, no URL echoed
	setting.SetBepusdtBaseURL(srv.URL)
	setOpt(model.OptChatwootBaseUrl, srv.URL)
	srv.Close()
	m = refresh()
	want(m, epayKey, false, "", "(3)")
	want(m, "bepusdt/main", false, "", "(3)")
	want(m, "chatwoot/main", false, "", "(3)")
	if strings.Contains(m["bepusdt/main"].Error, "127.0.0.1") {
		panic("(3) error echoes url: " + m["bepusdt/main"].Error)
	}
	for _, k := range []string{epayKey, "bepusdt/main", "chatwoot/main"} {
		if m[k].Status != "fail" || m[k].LastFailureAt == nil {
			panic(fmt.Sprintf("(3) %s want status=fail with last_failure_at, got %q %v", k, m[k].Status, m[k].LastFailureAt))
		}
	}
	// (3c) 再次 skipped：last_failure_at 保留真实失败时间，不被 skipped 刷新
	realFail := *m["bepusdt/main"].LastFailureAt
	setting.SetBepusdtBaseURL("")
	m2 := refresh()
	if m2["bepusdt/main"].Status != "skipped" || m2["bepusdt/main"].LastFailureAt == nil || !m2["bepusdt/main"].LastFailureAt.Equal(realFail) {
		panic(fmt.Sprintf("(3c) skipped after fail: status=%q lf=%v want lf=%v", m2["bepusdt/main"].Status, m2["bepusdt/main"].LastFailureAt, realFail))
	}
	setting.SetBepusdtBaseURL(srv.URL)
	fmt.Println("(3c) skipped after real fail keeps last_failure_at OK")
	fmt.Printf("(3) server down → ok=false (%s) OK\n", m["bepusdt/main"].Error)

	// disabled instance is not probed
	model.DB.Where("target_type = ? AND target_id = ?", "epay", strconv.FormatInt(inst.Id, 10)).Delete(&model.HealthProbe{})
	_, err = model.DisableEpayInstance(inst.Id)
	must(err)
	if _, has := refresh()[epayKey]; has {
		panic("(3b) disabled epay should not be probed")
	}
	fmt.Println("(3b) disabled epay skipped OK")

	// (3d) concurrent: slow stub must not block fast siblings past HTTP timeout
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(model.HealthProbeHTTPTimeout + 3*time.Second):
			w.WriteHeader(200)
		}
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer fast.Close()
	inst2, err := model.CreateEpayInstance(model.CreateEpayInstanceInput{
		Name: "accept-wire-epay-slow", PartnerId: "1001", Secret: "accept-wire-secret-2",
		PayAddress: slow.URL + "/", Enabled: true,
	})
	must(err)
	defer model.DB.Delete(&model.EpayInstance{}, inst2.Id)
	epaySlowKey := "epay/" + strconv.FormatInt(inst2.Id, 10)
	// re-enable first epay on fast server
	_, err = model.EnableEpayInstance(inst.Id)
	must(err)
	_ = model.DB.Model(&model.EpayInstance{}).Where("id = ?", inst.Id).Update("pay_address", fast.URL+"/").Error
	setting.SetBepusdtBaseURL(fast.URL)
	setOpt(model.OptChatwootBaseUrl, fast.URL)
	if got := setting.GetBepusdtBaseURL(); got != fast.URL {
		panic(fmt.Sprintf("(3d) bepusdt url not set: %q", got))
	}
	if got := model.ChatwootProbeURL(); got != fast.URL {
		panic(fmt.Sprintf("(3d) chatwoot url not set: %q", got))
	}
	t0 := time.Now()
	// 直接跑并发探活（绕过 gin），再读汇总，避免刷新路径干扰 URL 断言
	must(model.RunConfiguredProbes(context.Background()))
	elapsed := time.Since(t0)
	probes, err := model.ListHealthSummary()
	must(err)
	m = map[string]model.HealthProbeView{}
	for _, p := range probes {
		m[p.TargetType+"/"+p.TargetId] = p
	}
	// wall clock ≈ one HTTP timeout, not sum of slow+fast
	if elapsed > model.HealthProbeHTTPTimeout+4*time.Second {
		panic(fmt.Sprintf("(3d) concurrent too slow: %v (want ~%v)", elapsed, model.HealthProbeHTTPTimeout))
	}
	if elapsed < model.HealthProbeHTTPTimeout-time.Second {
		// slow target should force ~timeout wall; if everything finishes instantly something is wrong
		panic(fmt.Sprintf("(3d) concurrent too fast: %v (expected near timeout %v)", elapsed, model.HealthProbeHTTPTimeout))
	}
	want(m, "bepusdt/main", true, "http 200", "(3d)")
	want(m, "chatwoot/main", true, "http 200", "(3d)")
	want(m, epayKey, true, "http 200", "(3d)")
	want(m, epaySlowKey, false, "timeout", "(3d)")
	want(m, "postgres/main", true, "", "(3d)")
	if m[epaySlowKey].Status != "fail" {
		panic(fmt.Sprintf("(3d) slow epay status want fail got %q", m[epaySlowKey].Status))
	}
	fmt.Printf("(3d) concurrent probes wall=%v (timeout=%v) slow=fail fast=ok OK\n", elapsed.Round(time.Millisecond), model.HealthProbeHTTPTimeout)

	// (4) A36 redact
	code, body := createAdmin(`{"username":"aw","password":"short","confirm":true}`)
	if code != 400 || body["message"] != controller.AdminCreateMsgInvalidInput {
		panic(fmt.Sprintf("(4a) invalid_params got %d %v", code, body))
	}
	orig := model.DB
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	model.DB = orig.WithContext(cctx)
	code, body = createAdmin(`{"username":"accwire01","password":"Passw0rd!x","confirm":true}`)
	model.DB = orig
	if code != 500 || body["code"] != "db_error" || body["message"] != controller.AdminCreateMsgDBError {
		panic(fmt.Sprintf("(4b) db_error got %d %v", code, body))
	}
	if strings.Contains(fmt.Sprint(body["message"]), "context") {
		panic("(4b) leaks driver text")
	}
	if controller.AdminCreateMsgCreateFailed != "create failed" {
		panic("(4c) create_failed const")
	}
	src, err := os.ReadFile("controller/admin_create_adty.go")
	must(err)
	for _, ln := range strings.Split(string(src), "\n") {
		if strings.Contains(ln, "adminCreateReject(") && strings.Contains(ln, "err.Error()") {
			panic("(4c) reject still echoes err.Error(): " + ln)
		}
	}
	fmt.Println("(4) A36 invalid_input/db_error fixed msgs + create_failed const/static OK")
	fmt.Println("ALL ACCEPT HEALTH PROBES WIRE PASSED")
}
