package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestAdminPutBusinessParams_NoLongerRequiresConfirm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleRootUser)
	// 无 confirm：应直接进入业务校验（用越界值止于 out_of_range，避免无 DB 路径），不得再是 confirm_required
	body, _ := json.Marshal(map[string]any{
		"key":   "FirstTopupBonusRate",
		"value": "2",
	})
	c.Request = httptest.NewRequest(http.MethodPut, "/api/admin/business-params", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AdminPutBusinessParams(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] == "confirm_required" {
		t.Fatalf("confirm gate must be removed, got %v", resp)
	}
	if resp["code"] != "out_of_range" {
		t.Fatalf("want out_of_range (proves past confirm), got %v body=%s", resp["code"], w.Body.String())
	}
}

func TestAdminPutBusinessParams_OutOfRangeParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleRootUser)
	body, _ := json.Marshal(map[string]any{
		"confirm": true,
		"key":     "FirstTopupBonusRate",
		"value":   "2",
	})
	c.Request = httptest.NewRequest(http.MethodPut, "/api/admin/business-params", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AdminPutBusinessParams(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "out_of_range" {
		t.Fatalf("want out_of_range got %v body=%s", resp["code"], w.Body.String())
	}
	params, _ := resp["params"].(map[string]any)
	if params == nil {
		t.Fatalf("want params, body=%s", w.Body.String())
	}
	if params["key"] != "FirstTopupBonusRate" {
		t.Fatalf("want key in params got %v", params)
	}
	if _, ok := params["min"]; !ok {
		t.Fatalf("want min in params got %v", params)
	}
	if _, ok := params["max"]; !ok {
		t.Fatalf("want max in params got %v", params)
	}
	msg, _ := resp["message"].(string)
	if msg != "value out of range" {
		t.Fatalf("message should not embed numbers, got %q", msg)
	}
}

func TestAdminPutBusinessParams_InvalidIdStyleCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/admin/risk-reviews/abc", nil)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	AdminGetRiskReview(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "invalid_id" {
		t.Fatalf("want invalid_id got %v", resp)
	}
}

// 批量提交时事务校验阶段失败（跨字段 crit<=warn）：params 报真正出错的 key，而不是 map 里随便一个。
func TestAdminPutBusinessParams_BatchOutOfRangeReportsOffendingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for i := 0; i < 20; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", common.RoleRootUser)
		body, _ := json.Marshal(map[string]any{
			"confirm": true,
			"items": []map[string]any{
				{"key": "AffFreezeDays", "value": 7},
				{"key": "DiskAlertWarnPct", "value": 90},
				{"key": "DiskAlertCritPct", "value": 90},
				{"key": "TopupOrderTimeoutMinutes", "value": 30},
			},
		})
		c.Request = httptest.NewRequest(http.MethodPut, "/api/admin/business-params", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		AdminPutBusinessParams(c)
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		// B-08 起跨字段失败改报 param_rule_violation；这里仍校验报的是真正出错的 key
		if w.Code != http.StatusBadRequest || resp["code"] != "param_rule_violation" {
			t.Fatalf("want 400 param_rule_violation got %d %s", w.Code, w.Body.String())
		}
		params, _ := resp["params"].(map[string]any)
		if params["key"] != "DiskAlertCritPct" || len(params) != 3 {
			t.Fatalf("want DiskAlertCritPct {key,rule,other_key} got %v", params)
		}
	}
}

func putBP(t *testing.T, payload map[string]any) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleRootUser)
	body, _ := json.Marshal(payload)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/admin/business-params", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AdminPutBusinessParams(c)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func TestAdminPutBusinessParams_TopupTiersCodes(t *testing.T) {
	cases := []struct {
		value  any
		code   string
		params map[string]any
	}{
		{[]float64{1, 2, 3}, "topup_tiers_count_mismatch", map[string]any{"key": "TopupBonusTiers", "expected_count": 13.0, "actual_count": 3.0}},
		{[]float64{1, 1.5, 2, 2.5, 3, 3.5, 4, 4.5, 5, 5.5, 6, 6.5, 101}, "out_of_range", map[string]any{"key": "TopupBonusTiers", "index": 12.0, "value": 101.0, "min": 0.0, "max": 100.0}},
		{[]float64{1, 1.5, 2, 2.5, 3, 3.5, 4, 4.5, 5, 5.5, 6, 6.5, 6}, "topup_tiers_not_ascending", map[string]any{"key": "TopupBonusTiers", "index": 12.0, "value": 6.0, "prev_value": 6.5}},
		{map[string]any{"a": 1}, "invalid_value", map[string]any{"key": "TopupBonusTiers"}},
	}
	for _, c := range cases {
		code, resp := putBP(t, map[string]any{"confirm": true, "key": "TopupBonusTiers", "value": c.value})
		if code != http.StatusBadRequest || resp["code"] != c.code {
			t.Fatalf("%v: want 400 %s got %d %v", c.value, c.code, code, resp)
		}
		params, _ := resp["params"].(map[string]any)
		for k, v := range c.params {
			if params[k] != v {
				t.Fatalf("%v: params[%s]=%v want %v (all %v)", c.value, k, params[k], v, params)
			}
		}
		if msg, _ := resp["message"].(string); msg == "" {
			t.Fatalf("%v: empty message %v", c.value, resp)
		}
	}
}

// 审查 N-P1-4：管理员改资金参数 403 root_only；非资金参数不受影响（止于校验）。
func TestAdminPutBusinessParams_FundKeysRootOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		key, value, want string
	}{
		{"AffTopupCommissionRate", "0.5", "root_only"},
		{"TopupBonusTiers", "[50]", "root_only"},
		{"DiskAlertWarnPct", "10", "out_of_range"},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", common.RoleAdminUser)
		body, _ := json.Marshal(map[string]any{"key": tc.key, "value": tc.value})
		c.Request = httptest.NewRequest(http.MethodPut, "/api/admin/business-params", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		AdminPutBusinessParams(c)
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["code"] != tc.want {
			t.Fatalf("%s: want %s got %v", tc.key, tc.want, w.Body.String())
		}
	}
}
