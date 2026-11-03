package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func postModelMeta(t *testing.T, name string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/", strings.NewReader(`{"model_name":"`+name+`","status":1}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	CreateModelMeta(ctx)
	return recorder
}

func TestCreateModelMetaRejectsDuplicateLiveName(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Option{}, &model.Vendor{}))
	require.NoError(t, model.EnsureLiveModelNameIndex(db))

	first := postModelMeta(t, "dup-live-model")
	require.Contains(t, first.Body.String(), `"success":true`)

	second := postModelMeta(t, "dup-live-model")
	require.Contains(t, second.Body.String(), `"success":false`)
	require.Contains(t, second.Body.String(), "Model name already exists")

	var results [2]bool
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			recorder := postModelMeta(t, "dup-race-model")
			results[slot] = strings.Contains(recorder.Body.String(), `"success":true`)
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, ok := range results {
		if ok {
			successes++
		}
	}
	require.Equal(t, 1, successes)
	var live int64
	require.NoError(t, db.Model(&model.Model{}).Where("model_name = ?", "dup-race-model").Count(&live).Error)
	require.Equal(t, int64(1), live)

	err := db.Create(&model.Model{ModelName: "dup-live-model", Status: 1}).Error
	require.True(t, model.IsAdtyUniqueViolation(err))
}
