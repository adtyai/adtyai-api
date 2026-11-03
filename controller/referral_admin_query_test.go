package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReferralAdminQueriesReportDatabaseFailure(t *testing.T) {
	_ = adtyTestDB(t)

	cases := []struct {
		name    string
		path    string
		handler gin.HandlerFunc
		zero    string
	}{
		{"overview", "/api/referral/admin/overview", GetReferralOverview, `"affiliates":0`},
		{"badges", "/api/referral/admin/badges", GetReferralAdminBadges, `"total":0`},
		{"withdrawals", "/api/referral/admin/withdrawals", GetAdminReferralWithdrawals, `"total":0`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
			tc.handler(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), `"success":false`)
			require.NotContains(t, recorder.Body.String(), tc.zero)
		})
	}
}
