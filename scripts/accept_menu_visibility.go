//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
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

// A39/A08 Root-only writes: per-run operator ids scoped to this run.
var (
	runAdminID = 800000 + int(common.GetTimestamp()%100000)
	runRootID  = runAdminID + 100000
)

func roleCtxP(method, path, body string, role, id int, username string) (*gin.Context, *httptest.ResponseRecorder) {
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

// rootCtx: Root operator (the only role allowed to mutate; no confirm needed).
func rootCtx(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	return roleCtxP(method, path, body, common.RoleRootUser, runRootID, "accept-root")
}

func respCode(w *httptest.ResponseRecorder) string {
	var r struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	return r.Code
}

func adminCtx(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	return ctxWithRole(method, path, body, common.RoleAdminUser, 2, "accept-menuvis-admin")
}

func userCtx(method, path, body string, id int) (*gin.Context, *httptest.ResponseRecorder) {
	return ctxWithRole(method, path, body, common.RoleCommonUser, id, "accept-menuvis-user")
}

func parseView(body []byte) model.MenuVisibilityView {
	var resp struct {
		Success bool                     `json:"success"`
		Data    model.MenuVisibilityView `json:"data"`
	}
	must(json.Unmarshal(body, &resp))
	if !resp.Success {
		panic("success=false: " + string(body))
	}
	if resp.Data.User == nil {
		resp.Data.User = map[string]bool{}
	}
	if resp.Data.Admin == nil {
		resp.Data.Admin = map[string]bool{}
	}
	return resp.Data
}

func statusSidebarMaps() (user, admin map[string]bool) {
	c, w := ctxWithRole(http.MethodGet, "/api/status", "", common.RoleGuestUser, 0, "")
	controller.GetStatus(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("status want 200 got %d %s", w.Code, w.Body.String()))
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			SidebarUserVisible  map[string]bool `json:"SidebarUserVisible"`
			SidebarAdminVisible map[string]bool `json:"SidebarAdminVisible"`
		} `json:"data"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &resp))
	if !resp.Success {
		panic("status success=false")
	}
	if resp.Data.SidebarUserVisible == nil {
		resp.Data.SidebarUserVisible = map[string]bool{}
	}
	if resp.Data.SidebarAdminVisible == nil {
		resp.Data.SidebarAdminVisible = map[string]bool{}
	}
	return resp.Data.SidebarUserVisible, resp.Data.SidebarAdminVisible
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
	fmt.Println("InitDB+menu-visibility OK")

	// Reset options so accept is idempotent across runs.
	must(model.UpdateOption(model.OptSidebarUserVisible, "{}"))
	must(model.UpdateOption(model.OptSidebarAdminVisible, "{}"))

	// (1) GET returns user+admin maps
	c, w := adminCtx(http.MethodGet, "/api/admin/menu-visibility", "")
	controller.AdminGetMenuVisibility(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(1) GET want 200 got %d %s", w.Code, w.Body.String()))
	}
	view := parseView(w.Body.Bytes())
	fmt.Printf("(1) GET user=%v admin=%v OK\n", view.User, view.Admin)

	su, sa := statusSidebarMaps()
	fmt.Printf("(1b) status SidebarUserVisible=%v SidebarAdminVisible=%v OK\n", su, sa)

	// (0) A39 Root-only gating
	c, w = roleCtxP(http.MethodPut, "/api/admin/menu-visibility", `{"user":{"wallet":false},"confirm":true}`, common.RoleAdminUser, runAdminID, "accept-menuvis-admin")
	controller.AdminPutMenuVisibility(c)
	if w.Code != 403 || respCode(w) != "root_required" {
		panic(fmt.Sprintf("(0a) admin PUT want 403 root_required got %d %s", w.Code, w.Body.String()))
	}
	if !model.MenuKeyVisible("user", "wallet") {
		panic("(0c) rejected PUT changed visibility")
	}
	fmt.Println("(0) admin PUT → 403; no effect OK")

	// Invalid JSON → 400
	c, w = roleCtxP(http.MethodPut, "/api/admin/menu-visibility", `{not-json`, common.RoleRootUser, runRootID, "accept-root")
	controller.AdminPutMenuVisibility(c)
	if w.Code != 400 {
		panic(fmt.Sprintf("(1c) invalid json want 400 got %d %s", w.Code, w.Body.String()))
	}
	fmt.Println("(1c) invalid JSON → 400 OK")

	// Empty body sides → 400
	c, w = rootCtx(http.MethodPut, "/api/admin/menu-visibility", `{}`)
	controller.AdminPutMenuVisibility(c)
	if w.Code != 400 {
		panic(fmt.Sprintf("(1d) empty put want 400 got %d %s", w.Code, w.Body.String()))
	}
	fmt.Println("(1d) empty PUT → 400 OK")

	// (2) PUT hide wallet → GET/status false
	c, w = rootCtx(http.MethodPut, "/api/admin/menu-visibility", `{"user":{"wallet":false},"admin":{"overview":true}}`)
	controller.AdminPutMenuVisibility(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(2) PUT want 200 got %d %s", w.Code, w.Body.String()))
	}
	view = parseView(w.Body.Bytes())
	if view.User["wallet"] != false {
		panic(fmt.Sprintf("(2) user.wallet want false got %v", view.User))
	}
	if view.Admin["overview"] != true {
		panic(fmt.Sprintf("(2) admin.overview want true got %v", view.Admin))
	}
	su, sa = statusSidebarMaps()
	if v, ok := su["wallet"]; !ok || v != false {
		panic(fmt.Sprintf("(2b) status user.wallet want false present got ok=%v v=%v map=%v", ok, v, su))
	}
	if v, ok := sa["overview"]; !ok || v != true {
		panic(fmt.Sprintf("(2b) status admin.overview want true present got ok=%v v=%v map=%v", ok, v, sa))
	}
	if model.MenuKeyVisible("user", "wallet") {
		panic("(2c) MenuKeyVisible(user,wallet) should be false")
	}
	fmt.Println("(2) PUT hide wallet + status OK")

	// (3) Permission unchanged: hide wallet does not block /api/user/topup/info
	u := model.User{
		Username:    "accept-mv-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       0,
		AffCode:     "mv" + common.GetRandomString(6),
	}
	must(model.DB.Create(&u).Error)

	c, w = userCtx(http.MethodGet, "/api/user/topup/info", "", u.Id)
	controller.GetTopUpInfo(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(3) GetTopUpInfo want 200 got %d %s (menu hide must not 403)", w.Code, w.Body.String()))
	}
	fmt.Println("(3) logged-in GetTopUpInfo still 200 after hide wallet OK")

	// Unauthenticated → 401 via UserAuth middleware
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/user/topup/info", middleware.UserAuth(), controller.GetTopUpInfo)
	req := httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != 401 {
		panic(fmt.Sprintf("(3b) unauth want 401 got %d %s", w2.Code, w2.Body.String()))
	}
	fmt.Println("(3b) unauthenticated /api/user/topup/info → 401 OK")

	// (4) new key not written → default visible
	if !model.MenuKeyVisible("user", "brand_new_menu_xyz") {
		panic("(4) missing key should default visible")
	}
	if !model.MenuKeyVisible("admin", "brand_new_admin_menu_xyz") {
		panic("(4) missing admin key should default visible")
	}
	fmt.Println("(4) missing key defaults visible OK")

	// Restore empty maps so we do not leave accept residue that hides wallet in shared cloud PG.
	must(model.UpdateOption(model.OptSidebarUserVisible, "{}"))
	must(model.UpdateOption(model.OptSidebarAdminVisible, "{}"))
	fmt.Println("restored empty maps")

	fmt.Println("ALL accept_menu_visibility checks passed")
}
