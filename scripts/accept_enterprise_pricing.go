//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"

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

func expectFrozen(name string, code int, resp map[string]any) {
	if code != http.StatusForbidden {
		panic(fmt.Sprintf("%s: want 403 got %d %v", name, code, resp))
	}
	if resp["success"] != false {
		panic(fmt.Sprintf("%s: want success=false got %v", name, resp["success"]))
	}
	if resp["code"] != "feature_frozen" {
		panic(fmt.Sprintf("%s: want code=feature_frozen got %v", name, resp["code"]))
	}
	fmt.Printf("%s → 403 feature_frozen OK\n", name)
}

func call(method, path string, body any, adminId int, handler func(*gin.Context), params gin.Params) (int, map[string]any) {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	ww := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(ww)
	gc.Request = req
	gc.Params = params
	gc.Set("id", adminId)
	gc.Set("role", common.RoleRootUser)
	handler(gc)
	var resp map[string]any
	_ = json.Unmarshal(ww.Body.Bytes(), &resp)
	return ww.Code, resp
}

func newUser(tag string, role int) model.User {
	u := model.User{
		Username:    "accept-ep-freeze-" + tag + "-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       0,
		AffCode:     "epf" + common.GetRandomString(6),
		Email:       tag + "-" + common.GetRandomString(4) + "@example.com",
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	return u
}

func main() {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	model.InitOptionMap()
	must(model.EnsureEnterprisePriceSchema(model.DB))
	fmt.Println("InitDB OK (enterprise-pricing frozen this release: admin HTTP + runtime match)")

	adminId := 1
	idParams := gin.Params{{Key: "id", Value: "1"}}

	code, resp := call(http.MethodGet, "/api/admin/enterprise-pricing", nil, adminId, controller.AdminListEnterprisePricing, nil)
	expectFrozen("GET list", code, resp)

	code, resp = call(http.MethodPost, "/api/admin/enterprise-pricing", map[string]any{
		"user_id": 1, "discount": 0.9, "confirm": true,
	}, adminId, controller.AdminCreateEnterprisePricing, nil)
	expectFrozen("POST create", code, resp)

	code, resp = call(http.MethodPut, "/api/admin/enterprise-pricing/1", map[string]any{
		"discount": 0.8, "confirm": true,
	}, adminId, controller.AdminUpdateEnterprisePricing, idParams)
	expectFrozen("PUT update", code, resp)

	code, resp = call(http.MethodDelete, "/api/admin/enterprise-pricing/1", map[string]any{
		"confirm": true,
	}, adminId, controller.AdminDeleteEnterprisePricing, idParams)
	expectFrozen("DELETE", code, resp)

	// Runtime match freeze: seed a rule via model CRUD; Match must still return no-match.
	admin := newUser("admin", common.RoleRootUser)
	user := newUser("u", common.RoleCommonUser)
	must(model.DB.Where("user_id = ?", user.Id).Delete(&model.EnterprisePriceRule{}).Error)
	row, err := model.CreateEnterprisePriceRule(model.EnterprisePriceCreateInput{
		UserId:    user.Id,
		Discount:  0.5,
		CreatedBy: admin.Id,
		Confirm:   true,
	})
	must(err)
	if row == nil || row.Id <= 0 {
		panic("model CreateEnterprisePriceRule should still work (CRUD kept)")
	}
	rid, d := model.MatchEnterprisePriceRule(user.Id, "default", "gpt-4")
	if rid != 0 || math.Abs(d-1.0) > 1e-12 {
		panic(fmt.Sprintf("MatchEnterprisePriceRule frozen: want 0/1.0 got %d/%f (seeded rule %d)", rid, d, row.Id))
	}
	if got := model.EffectiveGroupRatio(2.0, d); math.Abs(got-2.0) > 1e-12 {
		panic(fmt.Sprintf("EffectiveGroupRatio with frozen discount: want 2.0 got %f", got))
	}
	_ = model.DeleteEnterprisePriceRule(row.Id, true)
	fmt.Println("runtime MatchEnterprisePriceRule → 0/1.0 OK (seeded rule ignored)")

	fmt.Println("ALL accept_enterprise_pricing PASSED (frozen 403 + runtime match off)")
}
