package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDoCheckinAndGetCheckinStatusStayRetired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saved := *operation_setting.GetCheckinSetting()
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{Enabled: true, MinQuota: 5000, MaxQuota: 5000}
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = saved })
	require.False(t, operation_setting.IsCheckinEnabled())

	for _, handler := range []gin.HandlerFunc{DoCheckin, GetCheckinStatus} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/checkin", nil)
		ctx.Set("id", 1)
		handler(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, false, body["success"])
		require.Equal(t, model.ErrCheckinRetired.Error(), body["message"])
		require.NotContains(t, recorder.Body.String(), "min_quota")
		require.NotContains(t, recorder.Body.String(), "quota_awarded")
	}
}
