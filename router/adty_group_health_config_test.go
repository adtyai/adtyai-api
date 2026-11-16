package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootReadsAndSavesGroupHealthSettings(t *testing.T) {
	f := newOwnerFixture(t)
	require.NoError(t, f.db.AutoMigrate(&model.AdtyGroupHealthState{}))
	root := adtySessionToken(t, f.db, "health-root", common.RoleRootUser)

	defaults := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/group-health", "", root)
	require.Contains(t, defaults.Body.String(), `"success":true`, defaults.Body.String())
	assert.Contains(t, defaults.Body.String(), `"success_min":"0.95"`)
	assert.Contains(t, defaults.Body.String(), `"stale_seconds":21600`)

	saved := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/group-health", `{"success_min":"0.9","stale_seconds":600}`, root)
	require.Contains(t, saved.Body.String(), `"success":true`, saved.Body.String())
	after := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/group-health", "", root)
	assert.Contains(t, after.Body.String(), `"success_min":"0.9"`)
	assert.Contains(t, after.Body.String(), `"stale_seconds":600`)

	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/group-health", "", f.token).Body.String(), `"success":true`)
}
