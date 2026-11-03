package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPaymentComplianceConfirmReportsDatabaseFailure(t *testing.T) {
	require.NoError(t, i18n.Init())
	_ = adtyTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/payment/compliance", strings.NewReader(`{"confirmed":true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	ConfirmPaymentCompliance(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":false`)
	require.NotContains(t, recorder.Body.String(), `"confirmed":true`)
}
