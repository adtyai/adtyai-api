package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSelfLogStatDoesNotIncludeUsersMatchedByPercent(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.Log{
		UserId: 7, Username: "ab%", Type: model.LogTypeConsume, Quota: 100,
		PromptTokens: 3, CompletionTokens: 4, CreatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: 8, Username: "abcd", Type: model.LogTypeConsume, Quota: 900,
		PromptTokens: 10, CompletionTokens: 20, CreatedAt: now,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/stat", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "ab%")
	ctx.Set("role", common.RoleAdminUser)
	GetLogsSelfStat(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Quota int `json:"quota"`
			Rpm   int `json:"rpm"`
			Tpm   int `json:"tpm"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success, recorder.Body.String())
	require.Equal(t, 100, body.Data.Quota)
	require.Equal(t, 1, body.Data.Rpm)
	require.Equal(t, 7, body.Data.Tpm)
}
