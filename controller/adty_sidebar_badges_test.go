package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetAdtySidebarBadgesFailsClosedWithoutUserOrSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/adty/badges", nil)
	GetAdtySidebarBadges(ctx)
	require.Contains(t, recorder.Body.String(), `"success":false`)
	require.NotContains(t, recorder.Body.String(), `"counts"`)
	require.NotContains(t, recorder.Body.String(), `"notification_unread"`)

	_ = adtyTestDB(t)
	root := &model.User{Username: "badge-ctrl-root", Password: "hashed-password", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: "badge-ctrl-root@example.com"}
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	require.NoError(t, model.DB.Create(root).Error)

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/adty/badges", nil)
	ctx.Set("id", root.Id)
	GetAdtySidebarBadges(ctx)
	require.Contains(t, recorder.Body.String(), `"success":false`)
	require.NotContains(t, recorder.Body.String(), `"notification_unread":0`)
	require.NotContains(t, recorder.Body.String(), `"counts":{}`)
}

func TestGetReferralAdminBadgesRequiresCallerAndStaysFailedClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/referral/admin/badges", nil)
	GetReferralAdminBadges(ctx)
	require.Contains(t, recorder.Body.String(), `"success":false`)
	require.NotContains(t, recorder.Body.String(), `"total":0`)
	_ = service.AdtySidebarBadgeConfirmedKinds()
}
