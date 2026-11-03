package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLogSearchRejectsOverflowingStartTime(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, db.Create(&model.Log{
		UserId: 3, Username: "old", Type: model.LogTypeConsume, Content: "ancient-log", CreatedAt: 1,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/self?start_timestamp=-9223372036854775808", nil)
	ctx.Set("id", 3)
	ctx.Set("role", common.RoleCommonUser)
	GetUserLogs(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":false`)
	require.NotContains(t, recorder.Body.String(), "ancient-log")
}
