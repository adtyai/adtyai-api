package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// 审查 P0-2 / P1-1 / 重审 P2-1：收款账户写、人工补单、退款均 root；只读保留管理员。
func TestAdtyPaymentAdminRoutesRootGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	tag := func(name string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusTeapot, gin.H{"gate": name})
		}
	}
	registerAdtyPaymentAdminRoutes(engine.Group("/api"), tag("admin"), tag("root"))

	cases := map[string]string{
		"GET /api/admin/orders":                        "admin",
		"GET /api/admin/orders/X1":                     "admin",
		"POST /api/admin/orders/X1/refund":             "root",
		"POST /api/admin/orders/X1/complete":           "root",
		"GET /api/admin/epay-instances":                "admin",
		"GET /api/admin/epay-instances/1":              "admin",
		"POST /api/admin/epay-instances":               "root",
		"PUT /api/admin/epay-instances/1":              "root",
		"POST /api/admin/epay-instances/1/set-primary": "root",
		"POST /api/admin/epay-instances/1/disable":     "root",
		"POST /api/admin/epay-instances/1/enable":      "root",
		"POST /api/admin/epay/manual-complete":         "root",
	}
	for key, want := range cases {
		var method, path string
		for i := range key {
			if key[i] == ' ' {
				method, path = key[:i], key[i+1:]
				break
			}
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		assert.Equal(t, http.StatusTeapot, rec.Code, key)
		assert.JSONEq(t, `{"gate":"`+want+`"}`, rec.Body.String(), key)
	}
}
