/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetPerfMetricsQueryFailureDoesNotLeakInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sentinel := errors.New("sql: connection refused")
	want := map[string]any{
		"success":     false,
		"message":     "Failed to load performance metrics",
		"message_key": "Failed to load performance metrics",
	}

	t.Run("summary", func(t *testing.T) {
		previous := queryPerfMetricsSummary
		queryPerfMetricsSummary = func(int, []string, ...string) (perfmetrics.SummaryAllResult, error) {
			return perfmetrics.SummaryAllResult{}, sentinel
		}
		t.Cleanup(func() { queryPerfMetricsSummary = previous })

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics/summary", nil)
		GetPerfMetricsSummary(ctx)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, want, body)
		require.NotContains(t, recorder.Body.String(), sentinel.Error())
	})

	t.Run("model", func(t *testing.T) {
		previous := queryPerfMetrics
		queryPerfMetrics = func(perfmetrics.QueryParams) (perfmetrics.QueryResult, error) {
			return perfmetrics.QueryResult{}, sentinel
		}
		t.Cleanup(func() { queryPerfMetrics = previous })

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics?model=gpt-test", nil)
		GetPerfMetrics(ctx)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, want, body)
		require.NotContains(t, recorder.Body.String(), sentinel.Error())
	})
}
