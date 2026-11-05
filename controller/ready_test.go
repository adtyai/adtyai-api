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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReadyFailsWhenDatabaseIsDown(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		_ = sqlDB.Close()
	})

	ready := callReady(t)
	require.Equal(t, http.StatusOK, ready.Code)
	require.Contains(t, ready.Body.String(), `"success":true`)
	require.NotContains(t, ready.Body.String(), "postgres")
	require.NotContains(t, ready.Body.String(), "password")

	require.NoError(t, sqlDB.Close())
	status := httptest.NewRecorder()
	statusCtx, _ := gin.CreateTestContext(status)
	statusCtx.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	GetStatus(statusCtx)
	require.Equal(t, http.StatusOK, status.Code, "public status stays a configuration response")
	down := callReady(t)
	require.Equal(t, http.StatusServiceUnavailable, down.Code)
	require.Contains(t, down.Body.String(), `"success":false`)
	require.NotContains(t, down.Body.String(), "dsn")
	require.NotContains(t, down.Body.String(), "password")
}

func callReady(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	GetReady(c)
	return recorder
}
