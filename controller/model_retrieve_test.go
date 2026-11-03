package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRetrieveModelUsesEnabledGroupNotTheStaticTable(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Ability{}))
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "zz-custom-visible", ChannelId: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "zz-custom-hidden", ChannelId: 1, Enabled: true}).Error)

	visible := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(visible)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models/zz-custom-visible", nil)
	ctx.Params = gin.Params{{Key: "model", Value: "zz-custom-visible"}}
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	RetrieveModel(ctx, constant.ChannelTypeOpenAI)
	require.Equal(t, http.StatusOK, visible.Code)
	require.Contains(t, visible.Body.String(), `"zz-custom-visible"`)

	staticName := ""
	for name := range openAIModelsMap {
		staticName = name
		break
	}
	require.NotEmpty(t, staticName)
	hiddenStatic := httptest.NewRecorder()
	staticCtx, _ := gin.CreateTestContext(hiddenStatic)
	staticCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/models/"+staticName, nil)
	staticCtx.Params = gin.Params{{Key: "model", Value: staticName}}
	common.SetContextKey(staticCtx, constant.ContextKeyUserGroup, "default")
	RetrieveModel(staticCtx, constant.ChannelTypeOpenAI)
	require.Equal(t, http.StatusNotFound, hiddenStatic.Code)
	require.Contains(t, hiddenStatic.Body.String(), "model_not_found")

	limited := httptest.NewRecorder()
	limitedCtx, _ := gin.CreateTestContext(limited)
	limitedCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/models/zz-custom-hidden", nil)
	limitedCtx.Params = gin.Params{{Key: "model", Value: "zz-custom-hidden"}}
	common.SetContextKey(limitedCtx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(limitedCtx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(limitedCtx, constant.ContextKeyTokenModelLimit, map[string]bool{"zz-custom-visible": true})
	RetrieveModel(limitedCtx, constant.ChannelTypeOpenAI)
	require.Equal(t, http.StatusNotFound, limited.Code)
}
