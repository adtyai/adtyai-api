package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetupRejectsBlankAndEmailUsernames(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.Setup{}))
	previous := constant.Setup
	constant.Setup = false
	t.Cleanup(func() { constant.Setup = previous })

	for _, username := range []string{"   ", "root@example.com"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(`{"username":"`+username+`","password":"password-123","confirmPassword":"password-123"}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		PostSetup(ctx)
		require.Contains(t, recorder.Body.String(), `"success":false`, username)
		require.False(t, constant.Setup)
	}
	var roots int64
	require.NoError(t, db.Model(&model.User{}).Where("role = ?", 100).Count(&roots).Error)
	require.Zero(t, roots)
}
