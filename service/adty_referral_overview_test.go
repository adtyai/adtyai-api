package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectAdtyReferralOverviewMatchesBadgeSiteScope(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteStaff{}, &model.AdtyTeam{}, &model.AdtyTeamMember{}))
	require.NoError(t, db.AutoMigrate(model.ReferralTables()...))

	root := badgeTestUser(t, db, "overview-root", common.RoleRootUser, 0)
	admin := badgeTestUser(t, db, "overview-admin", common.RoleAdminUser, 0)
	owner := badgeTestUser(t, db, "overview-owner", common.RoleCommonUser, 0)
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "overview-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(owner).Update("site_id", site.Id).Error)
	owner.SiteId = site.Id

	mainAffiliateUser := badgeTestUser(t, db, "overview-main-aff", common.RoleCommonUser, 0)
	siteAffiliateUser := badgeTestUser(t, db, "overview-site-aff", common.RoleCommonUser, site.Id)
	mainInvitee := badgeTestUser(t, db, "overview-main-invitee", common.RoleCommonUser, 0)
	siteInvitee := badgeTestUser(t, db, "overview-site-invitee", common.RoleCommonUser, site.Id)
	// A site affiliate inviting a main-site user must not leak into the
	// non-root admin total: bindings are scoped by the affiliate (inviter).
	crossedInvitee := badgeTestUser(t, db, "overview-crossed-invitee", common.RoleCommonUser, 0)

	mainAffiliate := model.ReferralAffiliate{UserId: mainAffiliateUser.Id, InviteCode: "OVWMAIN", Status: model.ReferralAffiliateStatusApproved}
	siteAffiliate := model.ReferralAffiliate{UserId: siteAffiliateUser.Id, InviteCode: "OVWSITE", Status: model.ReferralAffiliateStatusApproved}
	require.NoError(t, db.Create(&mainAffiliate).Error)
	require.NoError(t, db.Create(&siteAffiliate).Error)

	require.NoError(t, db.Create(&model.ReferralBinding{InviteeUserId: mainInvitee.Id, InviterUserId: mainAffiliateUser.Id, AffiliateId: mainAffiliate.Id}).Error)
	require.NoError(t, db.Create(&model.ReferralBinding{InviteeUserId: siteInvitee.Id, InviterUserId: siteAffiliateUser.Id, AffiliateId: siteAffiliate.Id}).Error)
	require.NoError(t, db.Create(&model.ReferralBinding{InviteeUserId: crossedInvitee.Id, InviterUserId: siteAffiliateUser.Id, AffiliateId: siteAffiliate.Id}).Error)

	require.NoError(t, db.Create(&model.ReferralCommissionAccount{
		AffiliateId: mainAffiliate.Id, UserId: mainAffiliateUser.Id, SettlementCurrency: "USD",
		PendingAmount: 1.1, AvailableAmount: 2.2, FrozenAmount: 3.3, WithdrawnAmount: 4.4,
	}).Error)
	require.NoError(t, db.Create(&model.ReferralCommissionAccount{
		AffiliateId: siteAffiliate.Id, UserId: siteAffiliateUser.Id, SettlementCurrency: "USD",
		PendingAmount: 10, AvailableAmount: 20, FrozenAmount: 30, WithdrawnAmount: 40,
	}).Error)

	rootOverview, err := CollectAdtyReferralOverview(root.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 2, rootOverview.Affiliates, "Root sees every site")
	assert.EqualValues(t, 3, rootOverview.Bindings, "Root sees every binding")
	assert.Equal(t, "11.10", rootOverview.Pending.StringFixed(2))
	assert.Equal(t, "22.20", rootOverview.Available.StringFixed(2))
	assert.Equal(t, "33.30", rootOverview.Frozen.StringFixed(2))
	assert.Equal(t, "44.40", rootOverview.Withdrawn.StringFixed(2))

	adminOverview, err := CollectAdtyReferralOverview(admin.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1, adminOverview.Affiliates, "non-root admin is limited to main-site users")
	assert.EqualValues(t, 1, adminOverview.Bindings, "non-root admin must not count other-site inviters, including those bound to a main-site invitee")
	assert.Equal(t, "1.10", adminOverview.Pending.StringFixed(2))
	assert.Equal(t, "2.20", adminOverview.Available.StringFixed(2))
	assert.Equal(t, "3.30", adminOverview.Frozen.StringFixed(2))
	assert.Equal(t, "4.40", adminOverview.Withdrawn.StringFixed(2))
	assert.NotEqual(t, rootOverview.Affiliates, adminOverview.Affiliates)
	assert.NotEqual(t, rootOverview.Bindings, adminOverview.Bindings)
	assert.NotEqual(t, rootOverview.Pending.StringFixed(2), adminOverview.Pending.StringFixed(2))
	assert.NotEqual(t, rootOverview.Available.StringFixed(2), adminOverview.Available.StringFixed(2))
	assert.NotEqual(t, rootOverview.Frozen.StringFixed(2), adminOverview.Frozen.StringFixed(2))
	assert.NotEqual(t, rootOverview.Withdrawn.StringFixed(2), adminOverview.Withdrawn.StringFixed(2))

	_, err = CollectAdtyReferralOverview(owner.Id)
	require.ErrorIs(t, err, ErrAdtyTicketForbidden)
	_, err = CollectAdtyReferralOverview(0)
	require.Error(t, err)
}

func TestCollectAdtyReferralOverviewFailsClosedOnQueryError(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	root := badgeTestUser(t, db, "overview-fail", common.RoleRootUser, 0)
	_, err := CollectAdtyReferralOverview(root.Id)
	require.Error(t, err, "missing overview source tables must fail, not return a successful zero total")
}
