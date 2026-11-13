package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataRetentionAPIIsRootOnlyAndRejectsNotificationPeriod(t *testing.T) {
	disableAdtyRedis(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.UserSession{},
		&model.AdtyDataRetentionPolicy{},
		&model.AdtyDataRetentionRun{},
		&model.AdtyDataRetentionRunItem{},
		&model.AuditLog{},
	))
	engine := newAdtyGroupEngine()
	root := adtySessionToken(t, db, "retention-root", common.RoleRootUser)
	admin := adtySessionToken(t, db, "retention-admin", common.RoleAdminUser)

	assert.Equal(t, http.StatusUnauthorized, adtyJSON(t, engine, http.MethodGet, "/api/adty/admin/data-retention", "", "").Code)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, engine, http.MethodGet, "/api/adty/admin/data-retention", "", admin).Code)

	got := adtyJSON(t, engine, http.MethodGet, "/api/adty/admin/data-retention", "", root)
	require.Equal(t, http.StatusOK, got.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Categories []struct {
				Category      string `json:"category"`
				AutoCleanable bool   `json:"auto_cleanable"`
				Configured    bool   `json:"configured"`
				Action        string `json:"action"`
			} `json:"categories"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(got.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Categories, 9)
	for _, category := range payload.Data.Categories {
		assert.False(t, category.Configured)
		if category.Category == model.DataRetentionCategoryNotificationHistory {
			assert.False(t, category.AutoCleanable)
			assert.Equal(t, model.DataRetentionActionExclude, category.Action)
		}
	}

	rejected := adtyJSON(t, engine, http.MethodPut, "/api/adty/admin/data-retention", `{"policies":[{"category":"notification_history","retention_days":30,"action":"delete"}]}`, root)
	assert.Equal(t, http.StatusOK, rejected.Code)
	assert.Contains(t, rejected.Body.String(), "Notification history cannot be given an automatic cleanup period")
	assert.NotContains(t, rejected.Body.String(), `"success":true`)

	saved := adtyJSON(t, engine, http.MethodPut, "/api/adty/admin/data-retention", `{"policies":[{"category":"api_usage_logs","retention_days":180,"action":"delete"}]}`, root)
	require.Equal(t, http.StatusOK, saved.Code)
	assert.Contains(t, saved.Body.String(), `"success":true`)
	assert.Contains(t, saved.Body.String(), `"configured":true`)
	assert.Contains(t, saved.Body.String(), `"retention_days":180`)

	assert.Equal(t, http.StatusForbidden, adtyJSON(t, engine, http.MethodPut, "/api/adty/admin/data-retention", `{"policies":[{"category":"api_usage_logs","retention_days":180,"action":"delete"}]}`, admin).Code)
	assert.Equal(t, http.StatusOK, adtyJSON(t, engine, http.MethodGet, "/api/adty/admin/data-retention/runs", "", root).Code)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, engine, http.MethodGet, "/api/adty/admin/data-retention/runs", "", admin).Code)
}
