package router

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteEarningReviewRequiresFinanceWritePermission(t *testing.T) {
	f := newAdtyGuardFixture(t)
	require.NoError(t, f.db.AutoMigrate(&model.BillingOperation{}, &model.AdtySiteEarningEvent{}))
	site := model.AdtySite{OwnerUserId: f.admin.Id, Subdomain: "finance-legacy-review", Status: model.AdtySiteStatusActive}
	require.NoError(t, f.db.Create(&site).Error)
	root := model.BillingOperation{OperationKey: "legacy:task:1001", Kind: "earning_root", State: model.BillingSettled,
		UserId: f.admin.Id, SiteId: site.Id, ReferenceId: 1001, EarningRiskFrozen: true, EarningReviewStatus: model.AdtyEarningNeedsReview}
	require.NoError(t, f.db.Create(&root).Error)
	event := model.AdtySiteEarningEvent{OperationKey: "task:1001:refund", RootOperationKey: root.OperationKey,
		SiteId: site.Id, UserId: f.admin.Id, Quota: -125, Markup: 1.25, CostFactor: .8,
		Day: time.Now().Format("2006-01-02"), ReviewStatus: model.AdtyEarningNeedsReview}
	require.NoError(t, f.db.Create(&event).Error)
	const list = "/api/user/admin/finance/site-earning-reviews"
	const resolve = list + "/resolve"
	const body = `{"operation_key":"task:1001:refund","original_charge":"skipped","reason":"verified original charge remained skipped"}`
	f.grant(t, authz.AdtySiteReview)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, list, "", f.token).Code)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, resolve, body, f.token).Code)
	f.grant(t, authz.AdtyFinanceRead, authz.AdtySiteReview)
	read := adtyJSON(t, f.engine, http.MethodGet, list, "", f.token)
	require.Contains(t, read.Body.String(), `"success":true`, read.Body.String())
	assert.Contains(t, read.Body.String(), `"root_operation_key":"legacy:task:1001"`)
	assert.Contains(t, read.Body.String(), `"review_status":"needs_review"`)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, resolve, body, f.token).Code)
	f.grant(t, authz.AdtyFinanceRead, authz.AdtyFinanceManualCredit)
	for range 2 {
		response := adtyJSON(t, f.engine, http.MethodPost, resolve, body, f.token)
		require.Contains(t, response.Body.String(), `"success":true`, response.Body.String())
	}
	require.NoError(t, f.db.Where("operation_key = ?", event.OperationKey).First(&event).Error)
	assert.Equal(t, model.AdtyEarningOriginalSkipped, event.ReviewStatus)
	assert.True(t, event.Projected)
	var audits int64
	require.NoError(t, f.db.Model(&model.AdtySiteAudit{}).Where("site_id = ? AND action = ?", site.Id, "earning_review").Count(&audits).Error)
	assert.EqualValues(t, 1, audits)
	conflict := adtyJSON(t, f.engine, http.MethodPost, resolve,
		`{"operation_key":"task:1001:refund","original_charge":"released","reason":"conflicting replay"}`, f.token)
	assert.Contains(t, conflict.Body.String(), `"success":false`)
}
