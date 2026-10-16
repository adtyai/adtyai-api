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
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func newUser(prefix string, role int) *model.User {
	u := &model.User{
		Username: prefix + common.GetRandomString(6), Password: "unused", Role: role,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "ac" + common.GetRandomString(6),
	}
	must(model.DB.Create(u).Error)
	return u
}

var engine *gin.Engine

func post(token string, body any) (int, string) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/admins", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func tok(u *model.User) string {
	b, err := service.CreateLoginSession(u.Id, "password", "127.0.0.1", "accept-admin-create")
	must(err)
	return b.AccessToken
}

func js(v any) string { b, _ := json.Marshal(v); return strings.Trim(string(b), "\"") }

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
	fmt.Println("InitDB+admin-create OK")

	gin.SetMode(gin.TestMode)
	engine = gin.New()
	g := engine.Group("/api/admin/admins")
	g.Use(middleware.AdminAuth())
	g.POST("", controller.AdminCreateAdmin)

	pw := "Acc3pt-" + common.GetRandomString(10)
	newName := "a36" + common.GetRandomString(8)
	body := map[string]any{"username": newName, "password": pw, "confirm": true}

	// (1) anon / common / admin(non-root) → 401/403
	if code, b := post("", body); code != 401 && code != 403 {
		panic(fmt.Sprintf("(1a) anon got %d %s", code, b))
	}
	cu := newUser("a36c", common.RoleCommonUser)
	if code, b := post(tok(cu), body); code != 401 && code != 403 {
		panic(fmt.Sprintf("(1b) common got %d %s", code, b))
	}
	ad := newUser("a36a", common.RoleAdminUser)
	if code, b := post(tok(ad), body); code != 403 {
		panic(fmt.Sprintf("(1c) admin want 403 got %d %s", code, b))
	}
	fmt.Println("(1) anon/common/admin rejected OK")

	root := newUser("a36r", common.RoleRootUser)
	rt := tok(root)
	// (2) no confirm → 400
	if code, b := post(rt, map[string]any{"username": newName, "password": pw}); code != 400 || !strings.Contains(b, "confirm_required") {
		panic(fmt.Sprintf("(2) want 400 confirm_required got %d %s", code, b))
	}
	fmt.Println("(2) root without confirm → 400 OK")

	// (5) role=Root → 400 role_not_allowed
	if code, b := post(rt, map[string]any{"username": newName, "password": pw, "confirm": true, "role": common.RoleRootUser}); code != 400 || !strings.Contains(b, "role_not_allowed") {
		panic(fmt.Sprintf("(5) want 400 role_not_allowed got %d %s", code, b))
	}
	fmt.Println("(5) role=Root → 400 role_not_allowed OK")

	// (3) success
	code, b := post(rt, body)
	if code != 200 {
		panic(fmt.Sprintf("(3) want 200 got %d %s", code, b))
	}
	var created model.User
	must(model.DB.Where("username = ?", newName).First(&created).Error)
	if created.Role != common.RoleAdminUser {
		panic(fmt.Sprintf("(3) role want %d got %d", common.RoleAdminUser, created.Role))
	}
	login := model.User{Username: newName, Password: pw}
	must(login.ValidateAndFill())
	fmt.Printf("(3) root+confirm → 200 id=%d role=Admin login OK\n", created.Id)

	// (4) duplicate
	if code, b := post(rt, body); code != 400 || !strings.Contains(b, "username_taken") {
		panic(fmt.Sprintf("(4) want 400 username_taken got %d %s", code, b))
	}
	fmt.Println("(4) duplicate username → 400 OK")
	fmt.Println("ALL A36 admin-create checks passed")
}
