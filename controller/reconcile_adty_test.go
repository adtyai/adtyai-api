package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func TestParseReconcileForce(t *testing.T) {
	cases := []struct {
		in       string
		want, ok bool
	}{
		{"", false, true}, {"  ", false, true}, {"false", false, true}, {"0", false, true},
		{"true", true, true}, {"1", true, true}, {"TRUE", true, true},
		{"yes", false, false}, {"abc", false, false},
	}
	for _, tc := range cases {
		got, ok := parseReconcileForce(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("parseReconcileForce(%q)=(%v,%v) want (%v,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func reconcileReq(t *testing.T, q string) (int, map[string]any) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/admin/reconcile?"+q, nil)
	AdminGetReconcile(c)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad json: %s", w.Body.String())
	}
	return w.Code, body
}

func TestAdminGetReconcileInvalidForce(t *testing.T) {
	code, body := reconcileReq(t, "date=2026-01-01&force=maybe")
	if code != http.StatusBadRequest || body["code"] != "invalid_parameters" {
		t.Fatalf("got %d %v", code, body)
	}
}

func TestAdminGetReconcileInvalidDateUsesErrorsIs(t *testing.T) {
	code, body := reconcileReq(t, "date=not-a-date")
	if code != http.StatusBadRequest || body["code"] != "invalid_parameters" {
		t.Fatalf("got %d %v", code, body)
	}
	_, _, err := model.ParseBusinessDate("2026/01/01")
	if !errors.Is(err, model.ErrInvalidBusinessDate) {
		t.Fatalf("want ErrInvalidBusinessDate, got %v", err)
	}
	if err.Error() != `invalid date "2026/01/01" (want YYYY-MM-DD)` {
		t.Fatalf("message changed: %s", err.Error())
	}
}
