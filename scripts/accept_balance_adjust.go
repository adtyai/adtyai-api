//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func main() {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	model.InitOptionMap()
	fmt.Println("InitDB OK (balance-adjustments admin API frozen this release)")

	adminId := 1
	idParams := gin.Params{{Key: "id", Value: "1"}}

	code, resp := call(http.MethodGet, "/api/admin/balance-adjustments", nil, adminId, controller.AdminListBalanceAdjustments, nil)
	expectFrozen("GET list", code, resp)

	code, resp = call(http.MethodPost, "/api/admin/balance-adjustments", map[string]any{
		"user_id": 1, "account": "paid", "amount": 100, "reason": "freeze-accept",
	}, adminId, controller.AdminCreateBalanceAdjustment, nil)
	expectFrozen("POST create", code, resp)

	code, resp = call(http.MethodPost, "/api/admin/balance-adjustments/1/approve", map[string]any{
		"confirm": true,
	}, adminId, controller.AdminApproveBalanceAdjustment, idParams)
	expectFrozen("POST approve", code, resp)

	code, resp = call(http.MethodPost, "/api/admin/balance-adjustments/1/reject", map[string]any{
		"confirm": true, "reason": "frozen",
	}, adminId, controller.AdminRejectBalanceAdjustment, idParams)
	expectFrozen("POST reject", code, resp)

	fmt.Println("ALL accept_balance_adjust PASSED (frozen 403)")
}
