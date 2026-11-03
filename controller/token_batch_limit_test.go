package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteTokenBatchRejectsMoreThanOneHundredIDs(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "batch-keep", "batch-keep-key")
	ids := make([]int, maxTokenBatchSize+1)
	ids[0] = token.Id
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/batch", map[string]any{"ids": ids}, 1)
	DeleteTokenBatch(ctx)
	response := decodeAPIResponse(t, recorder)
	assert.False(t, response.Success, response.Message)
	assert.Contains(t, response.Message, "100")
	var kept int64
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Count(&kept).Error)
	assert.EqualValues(t, 1, kept)
}

func TestTokenBatchRouteRejectsOversizedBody(t *testing.T) {
	previous := constant.AnonymousRequestBodyLimitKB
	constant.AnonymousRequestBodyLimitKB = 1
	t.Cleanup(func() { constant.AnonymousRequestBodyLimitKB = previous })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/token/batch", middleware.AnonymousRequestBodyLimit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	body := `{"ids":[` + strings.Repeat("1,", int(common.GetAnonymousRequestBodyLimitBytes())) + `1]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/token/batch", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}
