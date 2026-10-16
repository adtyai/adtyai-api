package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminRiskReviewConfirmRequired(t *testing.T) {
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
			body, _ := json.Marshal(map[string]any{"confirm": false})
			c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/risk-reviews/1/"+tc.name, bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: "1"}}
			c.Set("id", 1)
			tc.fn(c)
			require.Equal(t, http.StatusBadRequest, w.Code)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "confirm_required", resp["code"])
			require.True(t, strings.Contains(resp["message"].(string), "confirm"))
		})
	}
}
