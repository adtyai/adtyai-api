package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdtyFail_OmitsEmptyParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != "invalid_id" || resp["success"] != false {
		t.Fatalf("unexpected %v", resp)
	}
	if _, ok := resp["params"]; ok {
		t.Fatalf("params should be omitted when empty, got %v", resp)
	}
}

func TestAdtyFail_IncludesParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	adtyFail(c, http.StatusBadRequest, "below_min", "aff withdraw below minimum", map[string]any{
		"min": 10.0, "amount_quota": int64(1),
	})
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	params, _ := resp["params"].(map[string]any)
	if params == nil || params["min"] != 10.0 {
		t.Fatalf("want params.min=10 got %v", resp)
	}
}
