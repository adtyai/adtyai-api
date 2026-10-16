//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func ctxWithRole(method, path, body string, role, id int, username string) (*gin.Context, *httptest.ResponseRecorder) {
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
	c.Set("id", id)
	c.Set("role", role)
	c.Set("username", username)
	return c, w
}

func rootCtx(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	return ctxWithRole(method, path, body, common.RoleRootUser, 1, "accept-chatwoot-root")
}

func adminCtx(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	return ctxWithRole(method, path, body, common.RoleAdminUser, 2, "accept-chatwoot-admin")
}

func assertNoHmacPlain(raw, secret, label string) {
	if secret != "" && strings.Contains(raw, secret) {
		panic(fmt.Sprintf("%s: hmac plaintext leaked", label))
	}
	if strings.Contains(raw, `"ChatwootHmacSecret":"`) {
		panic(fmt.Sprintf("%s: ChatwootHmacSecret key present as value field", label))
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
	fmt.Println("InitDB+chatwoot OK")

	fakeHMAC := "cw-hmac-accept-secret-do-not-use-" + common.GetRandomString(8)
	scriptA := "<script>/* accept-a-" + common.GetRandomString(4) + " */</script>"
	scriptB := "<script>/* accept-b-" + common.GetRandomString(4) + " */</script>"

	_ = model.UpdateOption(model.OptChatwootScript, "")
	_ = model.UpdateOption(model.OptChatwootWebsiteToken, "")
	_ = model.UpdateOption(model.OptChatwootHmacSecret, "")
	_ = model.UpdateOption(model.OptChatwootBaseUrl, "")

	c, w := adminCtx(http.MethodGet, "/api/admin/chatwoot", "")
	controller.AdminGetChatwoot(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(1) GET status want 200 got %d body=%s", w.Code, w.Body.String()))
	}
	assertNoHmacPlain(w.Body.String(), fakeHMAC, "(1)")
	var getResp struct {
		Success bool                     `json:"success"`
		Data    model.ChatwootConfigView `json:"data"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if !getResp.Success || getResp.Data.BackupContact == "" {
		panic("(1) backup_contact missing")
	}
	if getResp.Data.HmacSecretSet {
		panic("(1) hmac should be unset initially")
	}
	fmt.Printf("(1) Admin GET 200 backup=%s/%s OK\n", getResp.Data.BackupContactType, getResp.Data.BackupContact)

	c, w = adminCtx(http.MethodPut, "/api/admin/chatwoot", fmt.Sprintf(`{"script":%q,"confirm":true}`, scriptA))
	controller.AdminPutChatwoot(c)
	if w.Code != 403 {
		panic(fmt.Sprintf("(2) admin PUT want 403 got %d %s", w.Code, w.Body.String()))
	}
	fmt.Println("(2) ordinary admin PUT → 403 OK")

	c, w = rootCtx(http.MethodPut, "/api/admin/chatwoot", fmt.Sprintf(`{"script":%q}`, scriptA))
	controller.AdminPutChatwoot(c)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm_required") {
		panic(fmt.Sprintf("(3) want 400 confirm_required got %d %s", w.Code, w.Body.String()))
	}
	fmt.Println("(3) Root PUT no confirm → 400 OK")

	c, w = rootCtx(http.MethodPut, "/api/admin/chatwoot", fmt.Sprintf(`{"script":%q,"website_token":"wt-accept-public","confirm":true}`, scriptA))
	controller.AdminPutChatwoot(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(4) want 200 got %d %s", w.Code, w.Body.String()))
	}
	assertNoHmacPlain(w.Body.String(), fakeHMAC, "(4)")
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if getResp.Data.Script != scriptA || getResp.Data.WebsiteToken != "wt-accept-public" {
		panic("(4) script/token mismatch")
	}
	c, w = adminCtx(http.MethodGet, "/api/admin/chatwoot", "")
	controller.AdminGetChatwoot(c)
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if getResp.Data.Script != scriptA {
		panic("(4b) GET script mismatch")
	}
	fmt.Println("(4) Root confirm script update + GET OK")

	c, w = rootCtx(http.MethodPut, "/api/admin/chatwoot", fmt.Sprintf(`{"hmac_secret":%q,"confirm":true}`, fakeHMAC))
	controller.AdminPutChatwoot(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(5) want 200 got %d %s", w.Code, w.Body.String()))
	}
	assertNoHmacPlain(w.Body.String(), fakeHMAC, "(5)")
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if !getResp.Data.HmacSecretSet || !strings.HasPrefix(getResp.Data.HmacSecretFingerprint, "hm_") {
		panic(fmt.Sprintf("(5) bad meta set=%v fp=%q", getResp.Data.HmacSecretSet, getResp.Data.HmacSecretFingerprint))
	}
	fp := getResp.Data.HmacSecretFingerprint
	c, w = adminCtx(http.MethodGet, "/api/admin/chatwoot", "")
	controller.AdminGetChatwoot(c)
	assertNoHmacPlain(w.Body.String(), fakeHMAC, "(5b)")
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if getResp.Data.HmacSecretFingerprint != fp {
		panic("(5b) fingerprint mismatch")
	}
	c, w = rootCtx(http.MethodPut, "/api/admin/chatwoot", fmt.Sprintf(`{"script":%q,"hmac_secret":"","confirm":true}`, scriptB))
	controller.AdminPutChatwoot(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(5c) want 200 got %d %s", w.Code, w.Body.String()))
	}
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if getResp.Data.Script != scriptB || getResp.Data.HmacSecretFingerprint != fp {
		panic("(5c) empty hmac should keep secret")
	}
	assertNoHmacPlain(w.Body.String(), fakeHMAC, "(5c)")
	fmt.Println("(5) hmac_secret set → fingerprint only; empty omit OK")
	fmt.Println("ALL accept_chatwoot checks passed")
}
