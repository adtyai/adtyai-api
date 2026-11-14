package router

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
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
	reviewOp := service.AdtySiteEarningReviewOperation("task:1001:refund", "skipped", "verified original charge remained skipped")
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodPost, resolve, body, f.token).Body.String(), "SECURITY_PROOF_REQUIRED")
	for range 2 {
		response := adtyJSONWithProof(t, f.engine, http.MethodPost, resolve, body, f.token, financeAdminProof(t, f, reviewOp))
		require.Contains(t, response.Body.String(), `"success":true`, response.Body.String())
	}
	require.NoError(t, f.db.Where("operation_key = ?", event.OperationKey).First(&event).Error)
	assert.Equal(t, model.AdtyEarningOriginalSkipped, event.ReviewStatus)
	assert.True(t, event.Projected)
	var audits int64
	require.NoError(t, f.db.Model(&model.AdtySiteAudit{}).Where("site_id = ? AND action = ?", site.Id, "earning_review").Count(&audits).Error)
	assert.EqualValues(t, 1, audits)
	conflictBody := `{"operation_key":"task:1001:refund","original_charge":"released","reason":"conflicting replay"}`
	conflict := adtyJSONWithProof(t, f.engine, http.MethodPost, resolve, conflictBody, f.token,
		financeAdminProof(t, f, service.AdtySiteEarningReviewOperation("task:1001:refund", "released", "conflicting replay")))
	assert.Contains(t, conflict.Body.String(), `"success":false`)
}

func TestSiteEarningReviewRequiresSecurityProof(t *testing.T) {
	f := newAdtyGuardFixture(t)
	require.NoError(t, f.db.AutoMigrate(&model.BillingOperation{}, &model.AdtySiteEarningEvent{}))
	var rootUser model.User
	require.NoError(t, f.db.Where("username = ?", "guard-root").First(&rootUser).Error)
	site := model.AdtySite{OwnerUserId: f.admin.Id, Subdomain: "finance-legacy-proof", Status: model.AdtySiteStatusActive}
	require.NoError(t, f.db.Create(&site).Error)
	root := model.BillingOperation{OperationKey: "legacy:task:2002", Kind: "earning_root", State: model.BillingSettled,
		UserId: f.admin.Id, SiteId: site.Id, ReferenceId: 2002, EarningRiskFrozen: true, EarningReviewStatus: model.AdtyEarningNeedsReview}
	require.NoError(t, f.db.Create(&root).Error)
	event := model.AdtySiteEarningEvent{OperationKey: "task:2002:refund", RootOperationKey: root.OperationKey,
		SiteId: site.Id, UserId: f.admin.Id, Quota: -125, Markup: 1.25, CostFactor: .8,
		Day: time.Now().Format("2006-01-02"), ReviewStatus: model.AdtyEarningNeedsReview}
	require.NoError(t, f.db.Create(&event).Error)

	const resolve = "/api/user/admin/finance/site-earning-reviews/resolve"
	const body = `{"operation_key":"task:2002:refund","original_charge":"released","reason":"verified the original task charge evidence"}`
	op := service.AdtySiteEarningReviewOperation("task:2002:refund", "released", "verified the original task charge evidence")
	otherOp := service.AdtySiteEarningReviewOperation("task:2002:refund", "skipped", "verified the original task charge evidence")

	f.grant(t, authz.AdtyFinanceRead, authz.AdtyFinanceManualCredit)
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodPost, resolve, body, f.token).Body.String(), "SECURITY_PROOF_REQUIRED")
	assert.Contains(t, adtyJSONWithProof(t, f.engine, http.MethodPost, resolve, body, f.token, financeAdminProof(t, f, otherOp)).Body.String(), "SECURITY_PROOF_CONTEXT_MISMATCH")
	assert.Contains(t, adtyJSONWithProof(t, f.engine, http.MethodPost, resolve, body, f.token, financeAdminProof(t, f, service.AdtyTopUpQueryOperation("EPU-unrelated"))).Body.String(), "SECURITY_PROOF_SCOPE_MISMATCH")

	okProof := financeAdminProof(t, f, op)
	ok := adtyJSONWithProof(t, f.engine, http.MethodPost, resolve, body, f.token, okProof)
	require.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
	replay := adtyJSONWithProof(t, f.engine, http.MethodPost, resolve, body, f.token, okProof)
	assert.Contains(t, replay.Body.String(), "SECURITY_PROOF_CONSUMED")

	rootOK := adtyJSONWithProof(t, f.engine, http.MethodPost, resolve, body, f.root, financeUserProof(t, rootUser, service.VerificationMethodRootExempt, op))
	assert.NotContains(t, rootOK.Body.String(), "SECURITY_PROOF", rootOK.Body.String())
	requirements, err := service.GetVerificationRequirements(service.AuthIdentity{UserID: rootUser.Id, SessionID: "sid-" + rootUser.Username, UserAuthVersion: rootUser.AuthVersion, SessionVersion: 1}, service.VerificationScopeAdtySiteEarningReview)
	require.NoError(t, err)
	require.Len(t, requirements.Methods, 1)
	assert.Equal(t, service.VerificationMethodRootExempt, requirements.Methods[0].Method)
}
