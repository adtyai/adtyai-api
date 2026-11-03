package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyQueryReportsDatabaseError(t *testing.T) {
	require.NoError(t, i18n.Init())
	_ = adtyTestDB(t)

	list := httptest.NewRecorder()
	listCtx, _ := gin.CreateTestContext(list)
	listCtx.Request = httptest.NewRequest(http.MethodPost, "/mj/task/list-by-condition", strings.NewReader(`{"ids":["mj-1"]}`))
	listCtx.Request.Header.Set("Content-Type", "application/json")
	listCtx.Set("id", 1)
	listCtx.Set("relay_mode", relayconstant.RelayModeMidjourneyTaskFetchByCondition)
	RelayMidjourney(listCtx)
	require.NotEqual(t, http.StatusOK, list.Code)
	require.Contains(t, list.Body.String(), "database_error")
	require.NotContains(t, list.Body.String(), "[]")

	fetch := httptest.NewRecorder()
	fetchCtx, _ := gin.CreateTestContext(fetch)
	fetchCtx.Request = httptest.NewRequest(http.MethodGet, "/mj/task/mj-1/fetch", nil)
	fetchCtx.Params = gin.Params{{Key: "id", Value: "mj-1"}}
	fetchCtx.Set("id", 1)
	fetchCtx.Set("relay_mode", relayconstant.RelayModeMidjourneyTaskFetch)
	RelayMidjourney(fetchCtx)
	require.NotEqual(t, http.StatusOK, fetch.Code)
	require.Contains(t, fetch.Body.String(), "database_error")
	require.NotContains(t, fetch.Body.String(), "task_no_found")

	require.NoError(t, model.DB.AutoMigrate(&model.Midjourney{}))
	missing := httptest.NewRecorder()
	missingCtx, _ := gin.CreateTestContext(missing)
	missingCtx.Request = httptest.NewRequest(http.MethodGet, "/mj/task/missing/fetch", nil)
	missingCtx.Params = gin.Params{{Key: "id", Value: "missing"}}
	missingCtx.Set("id", 1)
	missingCtx.Set("relay_mode", relayconstant.RelayModeMidjourneyTaskFetch)
	RelayMidjourney(missingCtx)
	require.Contains(t, missing.Body.String(), "task_no_found")
}
