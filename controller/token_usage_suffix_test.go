package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTokenUsageIgnoresChannelSuffix(t *testing.T) {
	db := adtyTestDB(t)
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	require.NoError(t, db.Create(&model.Token{
		UserId: 1, Key: "usagekey123", Name: "usage-token", Status: 1,
		RemainQuota: 5, UsedQuota: 2, ExpiredTime: -1,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/usage/token", nil)
	ctx.Request.Header.Set("Authorization", "Bearer sk-usagekey123-12")
	GetTokenUsage(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":true`)
	require.Contains(t, recorder.Body.String(), `"name":"usage-token"`)
}
