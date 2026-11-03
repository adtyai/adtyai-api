package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNegativePageSizeDoesNotReturnEveryLog(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	for i := 0; i < 15; i++ {
		require.NoError(t, db.Create(&model.Log{UserId: 1, Type: model.LogTypeConsume, Content: "row", CreatedAt: int64(i + 1)}).Error)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/?page_size=-1", nil)
	ctx.Set("role", common.RoleRootUser)
	GetAllLogs(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Items    []struct {
				Id int `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success, recorder.Body.String())
	require.Equal(t, 15, body.Data.Total)
	require.Equal(t, common.ItemsPerPage, body.Data.PageSize)
	require.Len(t, body.Data.Items, common.ItemsPerPage)
}
