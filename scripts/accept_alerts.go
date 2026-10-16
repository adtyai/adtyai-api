//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

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

func adminCtx(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "accept-alerts-admin")
	return c, w
}

type mockTG struct {
	lastURL  string
	lastBody string
	status   int
	respBody string
	calls    int
}

func (m *mockTG) Do(req *http.Request) (*http.Response, error) {
	m.calls++
	m.lastURL = req.URL.String()
	b, _ := io.ReadAll(req.Body)
	m.lastBody = string(b)
	code := m.status
	if code == 0 {
		code = 200
	}
	body := m.respBody
	if body == "" {
		body = `{"ok":true}`
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func assertNoPlainToken(raw, token string, label string) {
	if token != "" && strings.Contains(raw, token) {
		panic(fmt.Sprintf("%s: plaintext token leaked in response", label))
	}
	// A17(27): also must not find common sensitive names as plaintext values
	for _, needle := range []string{`"123456:ABC-DEF"`, token} {
		if needle != "" && strings.Contains(raw, needle) {
			panic(fmt.Sprintf("%s: found sensitive needle %q", label, needle))
		}
	}
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
	must(model.EnsureOpsAlertTable(model.DB))
	fmt.Println("InitDB+alerts OK")

	fakeToken := "123456:ABC-DEF_accept_alerts_fake_token_xyz"

	// reset runtime
	setting.SetAdtyTelegramBotToken("")
	setting.SetTelegramUrgentChatId("")
	setting.SetTelegramPendingChatId("")
	setting.SetAdtyAlertUrgentEnabled(true)
	setting.SetAdtyAlertPendingEnabled(true)
	_ = model.UpdateOption(model.OptAdtyTelegramBotToken, "")
	_ = model.UpdateOption(model.OptTelegramUrgentChatId, "")
	_ = model.UpdateOption(model.OptTelegramPendingChatId, "")

	// (1) GET — no plaintext token; secret_set=false
	c, w := adminCtx(http.MethodGet, "/api/admin/alerts", "")
	controller.AdminGetAlerts(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(1) GET status want 200 got %d body=%s", w.Code, w.Body.String()))
	}
	getBody := w.Body.String()
	assertNoPlainToken(getBody, fakeToken, "(1)")
	var getResp struct {
		Success bool                   `json:"success"`
		Data    model.AlertsConfigView `json:"data"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if !getResp.Success {
		panic("(1) success=false")
	}
	if getResp.Data.BotToken.SecretSet {
		panic("(1) secret_set should be false initially")
	}
	if getResp.Data.BotToken.OptionKey != model.OptAdtyTelegramBotToken {
		panic("(1) option_key mismatch")
	}
	if strings.Contains(getBody, `"AdtyTelegramBotToken":"`) && strings.Contains(getBody, fakeToken) {
		panic("(1) token key with value in GET")
	}
	fmt.Println("(1) GET no plaintext; secret_set=false OK")

	officialBefore := common.TelegramBotToken
	_ = officialBefore

	// (2) PUT set fake token → GET secret_set + hm_
	put1 := fmt.Sprintf(`{"bot_token":%q,"urgent_chat_id":"-100111","pending_chat_id":"-100222","urgent_route_enabled":true,"pending_route_enabled":true}`, fakeToken)
	c, w = adminCtx(http.MethodPut, "/api/admin/alerts", put1)
	controller.AdminPutAlerts(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(2) PUT status want 200 got %d body=%s", w.Code, w.Body.String()))
	}
	assertNoPlainToken(w.Body.String(), fakeToken, "(2) PUT resp")
	c, w = adminCtx(http.MethodGet, "/api/admin/alerts", "")
	controller.AdminGetAlerts(c)
	getBody = w.Body.String()
	assertNoPlainToken(getBody, fakeToken, "(2) GET")
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if !getResp.Data.BotToken.SecretSet {
		panic("(2) secret_set want true")
	}
	if !strings.HasPrefix(getResp.Data.BotToken.Fingerprint, "hm_") {
		panic(fmt.Sprintf("(2) fingerprint want hm_ got %q", getResp.Data.BotToken.Fingerprint))
	}
	if getResp.Data.UrgentChatId != "-100111" || getResp.Data.PendingChatId != "-100222" {
		panic("(2) chat ids not saved")
	}
	fpAfterSet := getResp.Data.BotToken.Fingerprint
	fmt.Printf("(2) PUT token → secret_set + %s OK\n", fpAfterSet)

	// official OAuth token untouched
	if common.TelegramBotToken != officialBefore {
		panic("(2) official TelegramBotToken was mutated")
	}

	// (3) PUT empty bot_token → unchanged
	c, w = adminCtx(http.MethodPut, "/api/admin/alerts", `{"bot_token":""}`)
	controller.AdminPutAlerts(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(3) PUT empty status %d %s", w.Code, w.Body.String()))
	}
	c, w = adminCtx(http.MethodGet, "/api/admin/alerts", "")
	controller.AdminGetAlerts(c)
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if !getResp.Data.BotToken.SecretSet || getResp.Data.BotToken.Fingerprint != fpAfterSet {
		panic(fmt.Sprintf("(3) empty PUT changed token: set=%v fp=%q want %q",
			getResp.Data.BotToken.SecretSet, getResp.Data.BotToken.Fingerprint, fpAfterSet))
	}
	if setting.GetAdtyTelegramBotToken() != fakeToken {
		panic("(3) runtime token cleared by empty PUT")
	}
	fmt.Println("(3) PUT empty bot_token leaves token OK")

	// omit bot_token entirely, change chat
	c, w = adminCtx(http.MethodPut, "/api/admin/alerts", `{"urgent_chat_id":"-100333"}`)
	controller.AdminPutAlerts(c)
	if w.Code != 200 {
		panic("(3b) omit PUT failed")
	}
	if setting.GetAdtyTelegramBotToken() != fakeToken {
		panic("(3b) omit cleared token")
	}
	if setting.GetTelegramUrgentChatId() != "-100333" {
		panic("(3b) urgent chat not updated")
	}
	fmt.Println("(3b) omit bot_token + update chat OK")

	// (4) PUT switches + merge_minutes
	c, w = adminCtx(http.MethodPut, "/api/admin/alerts", `{
		"urgent_route_enabled":false,
		"pending_route_enabled":true,
		"kind_enabled":{"epay_demote":false,"disk":true},
		"merge_minutes":15
	}`)
	controller.AdminPutAlerts(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(4) PUT switches %d %s", w.Code, w.Body.String()))
	}
	assertNoPlainToken(w.Body.String(), fakeToken, "(4)")
	if setting.IsAdtyAlertUrgentEnabled() {
		panic("(4) urgent_route should be false")
	}
	if model.GetAlertMergeMinutes() != 15 {
		panic(fmt.Sprintf("(4) merge_minutes want 15 got %d", model.GetAlertMergeMinutes()))
	}
	kinds := setting.GetAdtyAlertKindEnabled()
	if kinds["epay_demote"] != false {
		panic("(4) kind epay_demote not false")
	}
	// restore urgent route for test send
	_ = model.UpdateOption(model.OptAdtyAlertUrgentEnabled, "true")
	fmt.Println("(4) switches + merge_minutes OK")

	// (5) POST test missing config → 400
	setting.SetAdtyTelegramBotToken("")
	_ = model.UpdateOption(model.OptAdtyTelegramBotToken, "")
	c, w = adminCtx(http.MethodPost, "/api/admin/alerts/test", `{"level":"urgent"}`)
	controller.AdminTestAlerts(c)
	if w.Code != 400 {
		panic(fmt.Sprintf("(5) missing config want 400 got %d %s", w.Code, w.Body.String()))
	}
	if !strings.Contains(w.Body.String(), "not configured") && !strings.Contains(w.Body.String(), "not_configured") {
		panic(fmt.Sprintf("(5) want not_configured body=%s", w.Body.String()))
	}
	fmt.Println("(5) POST test missing config → 400 OK")

	// (6) POST test mock success
	must(model.UpdateOption(model.OptAdtyTelegramBotToken, fakeToken))
	must(model.UpdateOption(model.OptTelegramUrgentChatId, "-100111"))
	must(model.UpdateOption(model.OptAdtyAlertUrgentEnabled, "true"))
	mock := &mockTG{status: 200, respBody: `{"ok":true,"result":{"message_id":1}}`}
	prev := service.AdtyTelegramHTTPClient
	service.AdtyTelegramHTTPClient = mock
	defer func() { service.AdtyTelegramHTTPClient = prev }()

	c, w = adminCtx(http.MethodPost, "/api/admin/alerts/test", `{"level":"urgent","text":"accept test"}`)
	controller.AdminTestAlerts(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(6) test send want 200 got %d %s", w.Code, w.Body.String()))
	}
	assertNoPlainToken(w.Body.String(), fakeToken, "(6)")
	if mock.calls < 1 {
		panic("(6) mock HTTP not called")
	}
	if !strings.Contains(mock.lastURL, "api.telegram.org/bot") {
		panic(fmt.Sprintf("(6) unexpected URL %s", mock.lastURL))
	}
	// URL contains token — that's Telegram API path; response body must not
	alerts, err := model.ListOpsAlertsByKind("alert_test", 5)
	must(err)
	if len(alerts) < 1 {
		panic("(6) ops_alert_events alert_test missing")
	}
	if alerts[0].Level != "urgent" {
		panic("(6) alert level mismatch")
	}
	fmt.Println("(6) POST test mock success + ops_alert_events OK")

	// (7) A17(27): final GET body has no plaintext of fake token / TelegramBotToken plaintext
	c, w = adminCtx(http.MethodGet, "/api/admin/alerts", "")
	controller.AdminGetAlerts(c)
	final := w.Body.String()
	assertNoPlainToken(final, fakeToken, "(7)")
	for _, bad := range []string{fakeToken, "ABC-DEF_accept_alerts"} {
		if strings.Contains(final, bad) {
			panic(fmt.Sprintf("(7) leaked %q", bad))
		}
	}
	if !strings.Contains(final, "hm_") {
		panic("(7) missing hm_ fingerprint")
	}
	if !strings.Contains(final, "secret_set") {
		panic("(7) missing secret_set")
	}
	// thresholds aggregated
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if getResp.Data.Thresholds["AlertMergeMinutes"] == "" {
		panic("(7) thresholds missing AlertMergeMinutes")
	}
	fmt.Println("(7) A17(27) GET no plaintext Token; hm_+secret_set+thresholds OK")

	fmt.Println("ALL accept_alerts checks passed")
}
