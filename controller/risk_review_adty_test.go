package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminRiskReviewNoLongerRequiresConfirm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name string
		fn   func(*gin.Context)
	}{
		{"approve", AdminApproveRiskReview},
		{"reject", AdminRejectRiskReview},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			// 无 confirm + 非法 id：应在 id 校验处 400，不得再是 confirm_required
			body, _ := json.Marshal(map[string]any{})
			c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/risk-reviews/bad/"+tc.name, bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: "bad"}}
			c.Set("id", 1)
			tc.fn(c)
			require.Equal(t, http.StatusBadRequest, w.Code)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.NotEqual(t, "confirm_required", resp["code"], "confirm gate must be removed: %v", resp)
			require.Equal(t, "invalid_id", resp["code"])
		})
	}
}
