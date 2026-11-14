package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPriceSnapshotBackfillIsRootOnly(t *testing.T) {
	f := newAdtyGuardFixture(t)
	require.NoError(t, f.db.AutoMigrate(&model.Task{}, &model.BillingOperation{}))
	const path = "/api/adty/admin/billing/price-snapshot-backfill"
	for _, token := range []string{"", f.token} {
		assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, adtyJSON(t, f.engine, http.MethodGet, path, "", token).Code)
		assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, adtyJSON(t, f.engine, http.MethodPost, path, `{"dry_run":true}`, token).Code)
	}
	preview := adtyJSON(t, f.engine, http.MethodGet, path, "", f.root)
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	assert.Contains(t, preview.Body.String(), `"success":true`)
	apply := adtyJSON(t, f.engine, http.MethodPost, path, `{"dry_run":false,"limit":20}`, f.root)
	require.Equal(t, http.StatusOK, apply.Code, apply.Body.String())
	assert.Contains(t, apply.Body.String(), `"success":true`)
	assert.Contains(t, apply.Body.String(), `"amounts_preserved":true`)
}
