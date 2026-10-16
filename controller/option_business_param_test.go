package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func putOption(t *testing.T, key string, value any) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, _ := json.Marshal(map[string]any{"key": key, "value": value})
	c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateOption(c)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func assertParams(t *testing.T, name string, resp map[string]any, code string, want map[string]any) {
	t.Helper()
	if resp["success"] != false || resp["code"] != code {
		t.Fatalf("%s: want code %s got %v", name, code, resp)
	}
	if msg, _ := resp["message"].(string); msg == "" {
		t.Fatalf("%s: empty message", name)
	}
	params, _ := resp["params"].(map[string]any)
	if len(params) != len(want) {
		t.Fatalf("%s: params=%v want %v", name, params, want)
	}
	for k, v := range want {
		if params[k] != v {
			t.Fatalf("%s: params[%s]=%v want %v (all %v)", name, k, params[k], v, params)
		}
	}
}

// /api/option 通用配置保存：TopupBonusTiers 与 /api/admin/business-params 回同一套 code + params（含档位 index/value）。
func TestUpdateOption_TopupBonusTiersCodes(t *testing.T) {
	cases := []struct {
		name, value, code string
		params            map[string]any
	}{
		{"count", `[1,2,3]`, "topup_tiers_count_mismatch",
			map[string]any{"key": "TopupBonusTiers", "expected_count": 13.0, "actual_count": 3.0}},
		{"tier0 out of range", `[-1,1.5,2,2.5,3,3.5,4,4.5,5,5.5,6,6.5,8]`, "out_of_range",
			map[string]any{"key": "TopupBonusTiers", "index": 0.0, "value": -1.0, "min": 0.0, "max": 100.0}},
		{"last tier out of range", `[1,1.5,2,2.5,3,3.5,4,4.5,5,5.5,6,6.5,101]`, "out_of_range",
			map[string]any{"key": "TopupBonusTiers", "index": 12.0, "value": 101.0, "min": 0.0, "max": 100.0}},
		{"middle tier goes down", `[1,1.5,2,2.5,3,3.5,2,4.5,5,5.5,6,6.5,8]`, "topup_tiers_not_ascending",
			map[string]any{"key": "TopupBonusTiers", "index": 6.0, "value": 2.0, "prev_value": 3.5}},
		{"not an array", `{"a":1}`, "invalid_value",
			map[string]any{"key": "TopupBonusTiers", "value": `{"a":1}`}},
	}
	for _, c := range cases {
		status, resp := putOption(t, "TopupBonusTiers", c.value)
		if status != http.StatusOK {
			t.Fatalf("%s: option API keeps HTTP 200, got %d", c.name, status)
		}
		assertParams(t, c.name, resp, c.code, c.params)
	}
}

// 跨字段规则：param_rule_violation 恰好 {key, rule, other_key}，不带 value/min/max；两条路径一致。
func TestParamRuleViolation_DiskAlert(t *testing.T) {
	// /api/option：未设置时 crit 取默认 90，warn=95 违反 less_than
	_, resp := putOption(t, "DiskAlertWarnPct", "95")
	assertParams(t, "option warn", resp, "param_rule_violation",
		map[string]any{"key": "DiskAlertWarnPct", "rule": "less_than", "other_key": "DiskAlertCritPct"})

	// /api/admin/business-params 批量：warn=90、crit=90，按 key 排序先报 crit 违反 greater_than
	code, resp2 := putBP(t, map[string]any{"confirm": true, "items": []map[string]any{
		{"key": "DiskAlertWarnPct", "value": 90}, {"key": "DiskAlertCritPct", "value": 90},
	}})
	if code != http.StatusBadRequest {
		t.Fatalf("bp want 400 got %d", code)
	}
	assertParams(t, "bp crit", resp2, "param_rule_violation",
		map[string]any{"key": "DiskAlertCritPct", "rule": "greater_than", "other_key": "DiskAlertWarnPct"})
}

// invalid_value 携带的 value 是提交的原值（规范化后的字符串）。
func TestBusinessParams_InvalidValueCarriesValue(t *testing.T) {
	_, resp := putOption(t, "DailyReportTime", "25:99")
	assertParams(t, "option daily", resp, "invalid_value", map[string]any{"key": "DailyReportTime", "value": "25:99"})
	_, resp = putOption(t, "BackupRetentionDays", "500")
	if resp["code"] != "out_of_range" {
		t.Fatalf("want out_of_range got %v", resp)
	}
}
