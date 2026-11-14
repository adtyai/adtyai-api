package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetReferralOverviewUsesSameSiteScopeAsBadges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteStaff{}, &model.AdtyTeam{}, &model.AdtyTeamMember{}))
	require.NoError(t, db.AutoMigrate(model.ReferralTables()...))

	root := overviewTestUser(t, db, "ctrl-overview-root", common.RoleRootUser, 0)
	admin := overviewTestUser(t, db, "ctrl-overview-admin", common.RoleAdminUser, 0)
	owner := overviewTestUser(t, db, "ctrl-overview-owner", common.RoleCommonUser, 0)
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "ctrl-overview-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(owner).Update("site_id", site.Id).Error)

	mainUser := overviewTestUser(t, db, "ctrl-overview-main", common.RoleCommonUser, 0)
	siteUser := overviewTestUser(t, db, "ctrl-overview-site-user", common.RoleCommonUser, site.Id)
	mainInvitee := overviewTestUser(t, db, "ctrl-overview-main-invitee", common.RoleCommonUser, 0)
	siteInvitee := overviewTestUser(t, db, "ctrl-overview-site-invitee", common.RoleCommonUser, site.Id)

	mainAffiliate := model.ReferralAffiliate{UserId: mainUser.Id, InviteCode: "CTRLOVWM", Status: model.ReferralAffiliateStatusApproved}
	siteAffiliate := model.ReferralAffiliate{UserId: siteUser.Id, InviteCode: "CTRLOVWS", Status: model.ReferralAffiliateStatusApproved}
	require.NoError(t, db.Create(&mainAffiliate).Error)
	require.NoError(t, db.Create(&siteAffiliate).Error)
	require.NoError(t, db.Create(&model.ReferralBinding{InviteeUserId: mainInvitee.Id, InviterUserId: mainUser.Id, AffiliateId: mainAffiliate.Id}).Error)
	require.NoError(t, db.Create(&model.ReferralBinding{InviteeUserId: siteInvitee.Id, InviterUserId: siteUser.Id, AffiliateId: siteAffiliate.Id}).Error)
	require.NoError(t, db.Create(&model.ReferralCommissionAccount{
		AffiliateId: mainAffiliate.Id, UserId: mainUser.Id, SettlementCurrency: "USD",
		PendingAmount: 1.25, AvailableAmount: 2.5, FrozenAmount: 0.75, WithdrawnAmount: 4,
	}).Error)
	require.NoError(t, db.Create(&model.ReferralCommissionAccount{
		AffiliateId: siteAffiliate.Id, UserId: siteUser.Id, SettlementCurrency: "USD",
		PendingAmount: 8, AvailableAmount: 16, FrozenAmount: 24, WithdrawnAmount: 32,
	}).Error)

	rootBody := callReferralOverview(t, root.Id)
	assert.EqualValues(t, 2, rootBody["affiliates"])
	assert.EqualValues(t, 2, rootBody["bindings"])
	assert.Equal(t, "9.25", rootBody["pending"])
	assert.Equal(t, "18.50", rootBody["available"])
	assert.Equal(t, "24.75", rootBody["frozen"])
	assert.Equal(t, "36.00", rootBody["withdrawn"])

	adminBody := callReferralOverview(t, admin.Id)
	assert.EqualValues(t, 1, adminBody["affiliates"], "non-root admin with referral-read standing only sees main-site users")
	assert.EqualValues(t, 1, adminBody["bindings"])
	assert.Equal(t, "1.25", adminBody["pending"])
	assert.Equal(t, "2.50", adminBody["available"])
	assert.Equal(t, "0.75", adminBody["frozen"])
	assert.Equal(t, "4.00", adminBody["withdrawn"])
	assert.NotEqual(t, rootBody["affiliates"], adminBody["affiliates"])
	assert.NotEqual(t, rootBody["bindings"], adminBody["bindings"])
	assert.NotEqual(t, rootBody["pending"], adminBody["pending"])
	assert.NotEqual(t, rootBody["available"], adminBody["available"])
	assert.NotEqual(t, rootBody["frozen"], adminBody["frozen"])
	assert.NotEqual(t, rootBody["withdrawn"], adminBody["withdrawn"])

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/referral/admin/overview", nil)
	ctx.Set("id", owner.Id)
	GetReferralOverview(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":false`)
	require.NotContains(t, recorder.Body.String(), `"affiliates"`)
	require.NotContains(t, recorder.Body.String(), `"bindings"`)
	require.NotContains(t, recorder.Body.String(), "9.25")
	require.NotContains(t, recorder.Body.String(), "8.00")
}

func callReferralOverview(t *testing.T, userID int) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/referral/admin/overview", nil)
	ctx.Set("id", userID)
	GetReferralOverview(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.NotNil(t, payload.Data)
	return payload.Data
}

func overviewTestUser(t *testing.T, db *gorm.DB, name string, role, siteID int) *model.User {
	t.Helper()
	user := &model.User{Username: name, Password: "hashed-password", Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: name + "@example.com", SiteId: siteID}
	require.NoError(t, db.Create(user).Error)
	return user
}
