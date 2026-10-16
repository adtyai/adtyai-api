//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"

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
		Username:    "accept-ep-" + tag + "-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       0,
		AffCode:     "ep" + common.GetRandomString(6),
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
	fmt.Println("InitDB OK (enterprise-pricing A8 unfrozen: admin HTTP + runtime match)")

	admin := newUser("admin", common.RoleRootUser)
	user := newUser("u", common.RoleCommonUser)
	must(model.DB.Where("user_id = ?", user.Id).Delete(&model.EnterprisePriceRule{}).Error)

	code, resp := call(http.MethodGet, "/api/admin/enterprise-pricing", nil, admin.Id, controller.AdminListEnterprisePricing, nil)
	if code != http.StatusOK || resp["success"] != true {
		panic(fmt.Sprintf("GET list: want 200 success got %d %v", code, resp))
	}
	fmt.Println("GET list → 200 OK")

	code, resp = call(http.MethodPost, "/api/admin/enterprise-pricing", map[string]any{
		"user_id": user.Id, "discount": 0.5, "confirm": true,
	}, admin.Id, controller.AdminCreateEnterprisePricing, nil)
	if code != http.StatusOK || resp["success"] != true {
		panic(fmt.Sprintf("POST create: want 200 success got %d %v", code, resp))
	}
	data, _ := resp["data"].(map[string]any)
	ruleID := int64(0)
	switch v := data["id"].(type) {
	case float64:
		ruleID = int64(v)
	}
	if ruleID <= 0 {
		panic(fmt.Sprintf("POST create: missing rule id in %v", resp))
	}
	fmt.Printf("POST create → rule %d OK\n", ruleID)

	idParams := gin.Params{{Key: "id", Value: strconv.FormatInt(ruleID, 10)}}
	code, resp = call(http.MethodPut, "/api/admin/enterprise-pricing/"+strconv.FormatInt(ruleID, 10), map[string]any{
		"discount": 0.8, "confirm": true,
	}, admin.Id, controller.AdminUpdateEnterprisePricing, idParams)
	if code != http.StatusOK || resp["success"] != true {
		panic(fmt.Sprintf("PUT update: want 200 success got %d %v", code, resp))
	}
	fmt.Println("PUT update → 200 OK")

	rid, d := model.MatchEnterprisePriceRule(user.Id, "default", "gpt-4")
	if rid != ruleID || math.Abs(d-0.8) > 1e-12 {
		panic(fmt.Sprintf("MatchEnterprisePriceRule: want %d/0.8 got %d/%f", ruleID, rid, d))
	}
	if got := model.EffectiveGroupRatio(2.0, d); math.Abs(got-1.6) > 1e-12 {
		panic(fmt.Sprintf("EffectiveGroupRatio: want 1.6 got %f", got))
	}
	fmt.Println("runtime MatchEnterprisePriceRule → discount applied OK")

	code, resp = call(http.MethodDelete, "/api/admin/enterprise-pricing/"+strconv.FormatInt(ruleID, 10), map[string]any{
		"confirm": true,
	}, admin.Id, controller.AdminDeleteEnterprisePricing, idParams)
	if code != http.StatusOK || resp["success"] != true {
		panic(fmt.Sprintf("DELETE: want 200 success got %d %v", code, resp))
	}
	rid, d = model.MatchEnterprisePriceRule(user.Id, "default", "gpt-4")
	if rid != 0 || math.Abs(d-1.0) > 1e-12 {
		panic(fmt.Sprintf("after delete Match: want 0/1.0 got %d/%f", rid, d))
	}
	fmt.Println("DELETE → match cleared OK")

	fmt.Println("ALL accept_enterprise_pricing PASSED (A8 unfrozen)")
}
