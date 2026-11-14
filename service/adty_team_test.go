package service

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdtyTeamSharedBalanceAndSingleMembership(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leader := referralUser(t, db, "team-leader")
	member := referralUser(t, db, "team-member")
	other := referralUser(t, db, "team-other")
	site := model.AdtySite{OwnerUserId: leader.Id, Subdomain: "team-site", Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(leader).Update("site_id", site.Id).Error)
	require.NoError(t, db.Model(member).Update("site_id", site.Id).Error)

	_, err := CreateAdtyTeam(leader.Id, site.Id, "Ops", decimal.RequireFromString("0.70"))
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
	created, err := CreateAdtyTeam(leader.Id, site.Id, "Ops", decimal.RequireFromString("0.90"))
	require.NoError(t, err)
	_, err = CreateAdtyTeam(leader.Id, site.Id, "Second", decimal.NewFromInt(1))
	require.ErrorIs(t, err, ErrAdtyTeamBusy)
	_, err = JoinAdtyTeam(other.Id, created.Team.Id)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	joined, err := JoinAdtyTeam(member.Id, created.Team.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTeamRoleMember, joined.Role)
	_, err = JoinAdtyTeam(member.Id, created.Team.Id)
	require.ErrorIs(t, err, ErrAdtyTeamBusy)

	require.NoError(t, CreditAdtyTeam(int64(leader.Id), created.Team.Id, 1_000_000, "credit-1"))
	require.ErrorIs(t, CreditAdtyTeam(int64(member.Id), created.Team.Id, 1, "credit-member"), ErrAdtyTeamForbidden)
	require.NoError(t, SpendAdtyTeam(member.Id, 400_000, "call-1"))
	require.ErrorIs(t, SpendAdtyTeam(member.Id, 700_000, "call-2"), ErrAdtyTeamFunds)
	var team model.AdtyTeam
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	assert.EqualValues(t, 600_000, team.SharedQuota)
	var personal model.User
	require.NoError(t, db.First(&personal, member.Id).Error)
	assert.Zero(t, personal.Quota)

	require.NoError(t, CloseAdtyTeam(leader.Id, created.Team.Id))
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	assert.Equal(t, model.AdtyTeamStatusClosed, team.Status)
	assert.EqualValues(t, 600_000, team.SharedQuota)
	require.ErrorIs(t, SpendAdtyTeam(member.Id, 1, "after-close"), ErrAdtyTeamClosed)
}

func TestAdtyTeamInviteKeyAndCloseKeepSharedBalance(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	leader := referralUser(t, db, "team-invite-leader")
	member := referralUser(t, db, "team-invite-member")
	require.NoError(t, db.Model(leader).Update("site_id", 0).Error)
	require.NoError(t, db.Model(member).Update("site_id", 0).Error)
	created, err := CreateAdtyTeam(leader.Id, 0, "Main", decimal.NewFromInt(1))
	require.NoError(t, err)
	issued, err := CreateAdtyTeamInvite(leader.Id, created.Team.Id, time.Hour)
	require.NoError(t, err)
	joined, err := AcceptAdtyTeamInvite(member.Id, issued.Code)
	require.NoError(t, err)
	assert.Equal(t, created.Team.Id, joined.TeamId)
	_, err = AcceptAdtyTeamInvite(member.Id, issued.Code)
	require.Error(t, err)
	token := model.Token{UserId: member.Id, Key: "team-key-secret", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, BindAdtyTeamKey(member.Id, token.Id))
	require.NoError(t, CreditAdtyTeam(int64(leader.Id), created.Team.Id, 500000, "team-credit"))
	handled, err := reserveAdtyTeamQuotaHook(db, token.Id, member.Id, 200000, "request:team-1")
	require.NoError(t, err)
	assert.True(t, handled)
	var team model.AdtyTeam
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	assert.EqualValues(t, 300000, team.SharedQuota)
	var personal model.User
	require.NoError(t, db.First(&personal, member.Id).Error)
	assert.Zero(t, personal.Quota)

	require.NoError(t, closeAdtyTeamForDeletedLeader(db, leader.Id))
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	assert.Equal(t, model.AdtyTeamStatusClosed, team.Status)
	assert.EqualValues(t, 300000, team.SharedQuota)
}

func TestAdtyTeamMineUsesTheSignedInSite(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leader := referralUser(t, db, "team-mine-leader")
	_, err := CreateAdtyTeamForCurrentUser(leader.Id, "Desk", decimal.RequireFromString("0.80"))
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	var leftover int64
	require.NoError(t, db.Model(&model.AdtyTeam{}).Count(&leftover).Error)
	assert.Zero(t, leftover)
	created, err := CreateAdtyTeam(leader.Id, 0, "Desk", decimal.RequireFromString("0.80"))
	require.NoError(t, err)
	assert.Equal(t, 0, created.Team.SiteId)
	mine, err := GetAdtyTeamMine(leader.Id)
	require.NoError(t, err)
	require.NotNil(t, mine)
	assert.Equal(t, model.AdtyTeamRoleLeader, mine.Role)
	assert.Equal(t, "Desk", mine.Name)
	empty, err := GetAdtyTeamMine(0)
	require.NoError(t, err)
	assert.Nil(t, empty)
}

func TestAdtyTeamProtocolPriceUsesBoundKey(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	leader := referralUser(t, db, "team-price-leader")
	member := referralUser(t, db, "team-price-member")
	created, err := CreateAdtyTeam(leader.Id, 0, "Price", decimal.RequireFromString("0.50"))
	require.NoError(t, err)
	token := model.Token{UserId: member.Id, Key: "team-price-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, db.Model(member).Update("site_id", 0).Error)
	_, err = JoinAdtyTeam(member.Id, created.Team.Id)
	require.NoError(t, err)
	require.NoError(t, BindAdtyTeamKey(member.Id, token.Id))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, token.Id)
	marked, k := AdtyApplySiteMarkup(ctx, member.Id, "default", 2, "")
	assert.Equal(t, 1.0, k)
	assert.InDelta(t, 1.0, marked, 0.000001)
	assert.False(t, AdtyTeamPriceBlocked(ctx))

	personal, _ := gin.CreateTestContext(httptest.NewRecorder())
	personal.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	unchanged, _ := AdtyApplySiteMarkup(personal, member.Id, "default", 2, "")
	assert.InDelta(t, 2.0, unchanged, 0.000001)
}

func decimalPtr(value string) *decimal.Decimal {
	parsed := decimal.RequireFromString(value)
	return &parsed
}

func TestAdtyTeamCreateRequiresRootOrAuthorizedOwner(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	root := referralUser(t, db, "auth-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	root.Role = common.RoleRootUser
	mainLeader := referralUser(t, db, "auth-main-leader")
	mainUser := referralUser(t, db, "auth-main-user")
	admin := referralUser(t, db, "auth-admin")
	require.NoError(t, db.Model(admin).Update("role", common.RoleAdminUser).Error)

	ownerA := referralUser(t, db, "auth-owner-a")
	ownerB := referralUser(t, db, "auth-owner-b")
	siteA := model.AdtySite{OwnerUserId: ownerA.Id, Subdomain: "auth-a", Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	siteB := model.AdtySite{OwnerUserId: ownerB.Id, Subdomain: "auth-b", Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	require.NoError(t, db.Create(&siteA).Error)
	require.NoError(t, db.Create(&siteB).Error)
	// Owners stay on the main site (users.site_id = 0). Using that field as the
	// team site would wrongly place an owner-created team on the main site.
	require.Equal(t, 0, ownerA.SiteId)
	leaderA := referralUser(t, db, "auth-leader-a")
	memberA := referralUser(t, db, "auth-member-a")
	leaderB := referralUser(t, db, "auth-leader-b")
	staffA := referralUser(t, db, "auth-staff-a")
	require.NoError(t, db.Model(leaderA).Update("site_id", siteA.Id).Error)
	require.NoError(t, db.Model(memberA).Update("site_id", siteA.Id).Error)
	require.NoError(t, db.Model(leaderB).Update("site_id", siteB.Id).Error)
	require.NoError(t, db.Model(staffA).Updates(map[string]any{"site_id": siteA.Id, "staff_site_id": siteA.Id}).Error)
	leaderA.SiteId, memberA.SiteId, leaderB.SiteId = siteA.Id, siteA.Id, siteB.Id
	staffA.SiteId, staffA.StaffSiteId = siteA.Id, siteA.Id

	teamCount := func() int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.Model(&model.AdtyTeam{}).Count(&n).Error)
		return n
	}
	start := teamCount()

	_, err := CreateAdtyTeamForCurrentUser(mainUser.Id, "Self", decimal.RequireFromString("0.50"))
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyTeamForCurrentUser(leaderA.Id, "Self", decimal.RequireFromString("0.90"))
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyMainTeamByRoot(mainUser.Id, mainLeader.Id, "Denied", "0.80")
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyMainTeamByRoot(admin.Id, mainLeader.Id, "Denied", "0.80")
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyOwnerSiteTeam(mainUser.Id, siteA.Id, leaderA.Id, "Denied", "0.90")
	require.Error(t, err)
	assert.Equal(t, start, teamCount())

	created, err := CreateAdtyMainTeamByRoot(root.Id, mainLeader.Id, "Main Desk", "0.80")
	require.NoError(t, err)
	assert.Equal(t, 0, created.Team.SiteId)
	assert.Equal(t, mainLeader.Id, created.Team.LeaderUserId)
	assert.NotEqual(t, root.Id, created.Team.LeaderUserId)
	assert.True(t, created.Team.Discount.Equal(decimal.RequireFromString("0.80")))

	defaulted, err := CreateAdtyMainTeamByRoot(root.Id, mainUser.Id, "Default Price", "")
	require.NoError(t, err)
	assert.Equal(t, 0, defaulted.Team.SiteId)
	assert.Equal(t, mainUser.Id, defaulted.Team.LeaderUserId)
	assert.True(t, defaulted.Team.Discount.Equal(decimal.NewFromInt(1)))

	owned, err := CreateAdtyOwnerSiteTeam(ownerA.Id, siteA.Id, leaderA.Id, "Site Desk", "0.90")
	require.NoError(t, err)
	assert.Equal(t, siteA.Id, owned.Team.SiteId)
	assert.NotEqual(t, 0, owned.Team.SiteId)
	assert.Equal(t, leaderA.Id, owned.Team.LeaderUserId)
	assert.NotEqual(t, ownerA.Id, owned.Team.LeaderUserId)
	assert.True(t, owned.Team.Discount.Equal(decimal.RequireFromString("0.90")))

	_, err = CreateAdtyOwnerSiteTeam(ownerA.Id, 0, mainLeader.Id, "Main From Owner", "1")
	require.Error(t, err)
	_, err = CreateAdtyOwnerSiteTeam(ownerA.Id, siteB.Id, leaderB.Id, "Other Site", "0.90")
	require.Error(t, err)
	_, err = CreateAdtyOwnerSiteTeam(ownerB.Id, siteA.Id, leaderA.Id, "Cross Owner", "0.90")
	require.Error(t, err)
	_, err = CreateAdtyOwnerSiteTeam(staffA.Id, siteA.Id, memberA.Id, "Staff Create", "0.90")
	require.Error(t, err)

	missingID := 9_000_001
	_, err = CreateAdtyOwnerSiteTeam(ownerA.Id, siteA.Id, missingID, "Missing", "0.90")
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyOwnerSiteTeam(ownerA.Id, siteA.Id, leaderB.Id, "Wrong Site Leader", "0.90")
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyOwnerSiteTeam(ownerA.Id, siteA.Id, staffA.Id, "Staff Leader", "0.90")
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyOwnerSiteTeam(ownerA.Id, siteA.Id, leaderA.Id, "Already Leader", "0.90")
	require.ErrorIs(t, err, ErrAdtyTeamBusy)

	busyMember, err := JoinAdtyTeam(memberA.Id, owned.Team.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTeamRoleMember, busyMember.Role)
	_, err = CreateAdtyOwnerSiteTeam(ownerA.Id, siteA.Id, memberA.Id, "Already Member", "0.90")
	require.ErrorIs(t, err, ErrAdtyTeamBusy)

	require.NoError(t, SaveAdtyMainTeamAgreementPrice(created.Team.Id, "0.70"))
	require.Error(t, SaveAdtyMainTeamAgreementPrice(owned.Team.Id, "0.70"))
	require.NoError(t, SaveAdtyOwnerSiteTeamDiscount(siteA.Id, ownerA.Id, owned.Team.Id, "0.85"))
	require.Error(t, SaveAdtyOwnerSiteTeamDiscount(siteA.Id, ownerB.Id, owned.Team.Id, "0.95"))
	require.Error(t, SaveAdtyOwnerSiteTeamDiscount(siteA.Id, mainUser.Id, owned.Team.Id, "0.95"))
	require.Error(t, SaveAdtyOwnerSiteTeamDiscount(siteA.Id, leaderA.Id, owned.Team.Id, "0.95"))
}

func TestAdtyTeamInviteRevokeTTLAndAuthorization(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	leader := referralUser(t, db, "invite-rev-leader")
	member := referralUser(t, db, "invite-rev-member")
	otherLeader := referralUser(t, db, "invite-rev-other-leader")
	staff := referralUser(t, db, "invite-rev-staff")
	require.NoError(t, db.Model(staff).Updates(map[string]any{"staff_site_id": 1}).Error)
	require.NoError(t, db.Model(leader).Update("site_id", 0).Error)
	require.NoError(t, db.Model(member).Update("site_id", 0).Error)
	require.NoError(t, db.Model(otherLeader).Update("site_id", 0).Error)
	created, err := CreateAdtyTeam(leader.Id, 0, "Invite Desk", decimal.NewFromInt(1))
	require.NoError(t, err)
	otherTeam, err := CreateAdtyTeam(otherLeader.Id, 0, "Other", decimal.NewFromInt(1))
	require.NoError(t, err)

	now := time.Now().Unix()
	defaultIssued, err := CreateAdtyTeamInvite(leader.Id, created.Team.Id, AdtyTeamInviteDefaultTTL)
	require.NoError(t, err)
	require.NotEmpty(t, defaultIssued.Code)
	assert.Equal(t, leader.Id, defaultIssued.CreatedBy)
	assert.Equal(t, model.AdtyTeamInviteStatusOutstanding, defaultIssued.Status)
	assert.InDelta(t, now+int64(AdtyTeamInviteDefaultTTL.Seconds()), defaultIssued.ExpiresAt, 3)

	customIssued, err := CreateAdtyTeamInvite(leader.Id, created.Team.Id, 2*time.Hour)
	require.NoError(t, err)
	assert.InDelta(t, now+int64((2*time.Hour).Seconds()), customIssued.ExpiresAt, 3)
	assert.NotEqual(t, defaultIssued.ExpiresAt, customIssued.ExpiresAt)

	_, err = CreateAdtyTeamInvite(leader.Id, created.Team.Id, 0)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
	_, err = CreateAdtyTeamInvite(leader.Id, created.Team.Id, -time.Hour)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
	_, err = CreateAdtyTeamInvite(leader.Id, created.Team.Id, 7*24*time.Hour+time.Second)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
	_, err = CreateAdtyTeamInvite(member.Id, created.Team.Id, time.Hour)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyTeamInvite(otherLeader.Id, created.Team.Id, time.Hour)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyTeamInvite(staff.Id, created.Team.Id, time.Hour)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)

	expired, err := CreateAdtyTeamInvite(leader.Id, created.Team.Id, time.Second)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.AdtyTeamInvite{}).Where("id = ?", expired.Id).Update("expires_at", time.Now().Add(-time.Second).Unix()).Error)
	_, err = AcceptAdtyTeamInvite(member.Id, expired.Code)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)

	toRevoke, err := CreateAdtyTeamInvite(leader.Id, created.Team.Id, time.Hour)
	require.NoError(t, err)
	_, err = RevokeAdtyTeamInvite(member.Id, created.Team.Id, toRevoke.Id)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = RevokeAdtyTeamInvite(otherLeader.Id, created.Team.Id, toRevoke.Id)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = RevokeAdtyTeamInvite(staff.Id, created.Team.Id, toRevoke.Id)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = RevokeAdtyTeamInvite(otherLeader.Id, otherTeam.Team.Id, toRevoke.Id)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)

	revoked, err := RevokeAdtyTeamInvite(leader.Id, created.Team.Id, toRevoke.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTeamInviteStatusRevoked, revoked.Status)
	assert.Equal(t, leader.Id, revoked.CreatedBy)
	assert.Equal(t, leader.Id, revoked.RevokedBy)
	assert.NotZero(t, revoked.RevokedAt)
	_, err = AcceptAdtyTeamInvite(member.Id, toRevoke.Code)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
	var stillOpen int64
	require.NoError(t, db.Model(&model.AdtyTeamMember{}).Where("user_id = ? AND status = ?", member.Id, model.AdtyTeamMemberActive).Count(&stillOpen).Error)
	assert.Zero(t, stillOpen)

	used, err := CreateAdtyTeamInvite(leader.Id, created.Team.Id, time.Hour)
	require.NoError(t, err)
	joined, err := AcceptAdtyTeamInvite(member.Id, used.Code)
	require.NoError(t, err)
	assert.Equal(t, created.Team.Id, joined.TeamId)
	_, err = RevokeAdtyTeamInvite(leader.Id, created.Team.Id, used.Id)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
	var membership model.AdtyTeamMember
	require.NoError(t, db.Where("user_id = ? AND team_id = ?", member.Id, created.Team.Id).First(&membership).Error)
	assert.Equal(t, model.AdtyTeamMemberActive, membership.Status)
	_, err = AcceptAdtyTeamInvite(member.Id, used.Code)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)

	listed, err := ListAdtyTeamInvites(leader.Id, created.Team.Id)
	require.NoError(t, err)
	require.NotEmpty(t, listed)
	_, err = ListAdtyTeamInvites(member.Id, created.Team.Id)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = ListAdtyTeamInvites(staff.Id, created.Team.Id)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	statuses := map[int64]string{}
	for _, row := range listed {
		statuses[row.Id] = row.Status
		assert.Equal(t, created.Team.Id, row.TeamId)
		assert.Equal(t, leader.Id, row.CreatedBy)
	}
	assert.Equal(t, model.AdtyTeamInviteStatusRevoked, statuses[toRevoke.Id])
	assert.Equal(t, model.AdtyTeamInviteStatusUsed, statuses[used.Id])
	assert.Equal(t, model.AdtyTeamInviteStatusExpired, statuses[expired.Id])
	assert.Equal(t, model.AdtyTeamInviteStatusOutstanding, statuses[customIssued.Id])

	requireTeamInviteAudit(t, defaultIssued.Id, "adty.team.invite.create", leader.Id, defaultIssued.Code)
	requireTeamInviteAudit(t, toRevoke.Id, "adty.team.invite.revoke", leader.Id, toRevoke.Code)
	requireTeamInviteAudit(t, used.Id, "adty.team.invite.accept", member.Id, used.Code)
}

func requireTeamInviteAudit(t *testing.T, inviteId int64, action string, actorId int, secret string) {
	t.Helper()
	var log model.AuditLog
	require.NoError(t, model.DB.Where("object_type = ? AND object_id = ? AND action = ?", "team_invite", strconv.FormatInt(inviteId, 10), action).First(&log).Error)
	assert.Equal(t, model.AuditResultSuccess, log.Result)
	assert.True(t, log.Success)
	assert.Equal(t, actorId, log.UserId)
	assert.Contains(t, log.AfterValue, "created_by")
	assert.Contains(t, log.AfterValue, `"result":"success"`)
	assert.NotContains(t, log.AfterValue, "code_hash")
	assert.NotContains(t, log.BeforeValue, secret)
	assert.NotContains(t, log.AfterValue, secret)
	assert.NotContains(t, log.Content, secret)
}

func TestInheritAdtyTeamsOnCloseRecordsSourceSiteAndKeepsIdentity(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "team-src")
	_, other := openActiveSite(t, db, "team-other")
	leader := siteCustomer(t, db, site, "team-src-leader")
	member := siteCustomer(t, db, site, "team-src-member")
	created, err := CreateAdtyTeam(leader.Id, site.Id, "Migrating", decimal.RequireFromString("0.90"))
	require.NoError(t, err)
	teamID := created.Team.Id
	joined, err := JoinAdtyTeam(member.Id, teamID)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTeamRoleMember, joined.Role)

	token := model.Token{UserId: member.Id, Key: "team-src-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, BindAdtyTeamKey(member.Id, token.Id))

	historical := model.AdtyTeam{
		SiteId: 0, LeaderUserId: leader.Id, Name: "legacy-main",
		Discount: decimal.NewFromInt(1), Status: model.AdtyTeamStatusActive,
	}
	require.NoError(t, db.Create(&historical).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return InheritAdtyTeamsOnCloseTx(tx, site.Id)
	}))

	var inherited model.AdtyTeam
	require.NoError(t, db.First(&inherited, teamID).Error)
	assert.Equal(t, teamID, inherited.Id)
	assert.Zero(t, inherited.SiteId)
	assert.Equal(t, site.Id, inherited.SourceSiteId)
	assert.Equal(t, model.AdtyTeamStatusActive, inherited.Status)
	assert.Zero(t, inherited.ClosedAt)
	assert.Equal(t, leader.Id, inherited.LeaderUserId)

	var members []model.AdtyTeamMember
	require.NoError(t, db.Where("team_id = ?", teamID).Order("id").Find(&members).Error)
	require.Len(t, members, 2)
	assert.Equal(t, leader.Id, members[0].UserId)
	assert.Equal(t, model.AdtyTeamRoleLeader, members[0].Role)
	assert.Equal(t, model.AdtyTeamMemberActive, members[0].Status)
	assert.Equal(t, member.Id, members[1].UserId)
	assert.Equal(t, model.AdtyTeamRoleMember, members[1].Role)
	assert.Equal(t, model.AdtyTeamMemberActive, members[1].Status)

	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	assert.False(t, token.DeletedAt.Valid)
	var key model.AdtyTeamKey
	require.NoError(t, db.Where("token_id = ?", token.Id).First(&key).Error)
	assert.Equal(t, model.AdtyTeamMemberLeft, key.Status)

	var leftover model.AdtyTeam
	require.NoError(t, db.First(&leftover, historical.Id).Error)
	assert.Zero(t, leftover.SiteId)
	assert.Zero(t, leftover.SourceSiteId)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return InheritAdtyTeamsOnCloseTx(tx, site.Id)
	}))
	require.NoError(t, db.First(&inherited, teamID).Error)
	assert.Zero(t, inherited.SiteId)
	assert.Equal(t, site.Id, inherited.SourceSiteId)
	assert.Equal(t, model.AdtyTeamStatusActive, inherited.Status)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)

	require.NoError(t, db.Model(&inherited).Update("site_id", site.Id).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return InheritAdtyTeamsOnCloseTx(tx, site.Id)
	}))
	require.NoError(t, db.First(&inherited, teamID).Error)
	assert.Zero(t, inherited.SiteId)
	assert.Equal(t, site.Id, inherited.SourceSiteId)

	require.NoError(t, db.Model(&inherited).Updates(map[string]any{"site_id": site.Id, "source_site_id": other.Id}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return InheritAdtyTeamsOnCloseTx(tx, site.Id)
	}))
	require.NoError(t, db.First(&inherited, teamID).Error)
	assert.Zero(t, inherited.SiteId)
	assert.Equal(t, other.Id, inherited.SourceSiteId)

	require.NoError(t, db.Model(&inherited).Updates(map[string]any{"site_id": 0, "source_site_id": site.Id}).Error)

	_, err = ScheduleAdtySiteClose(site.Id, 1, AdtySiteClosePlan{TransferTo: other.Id, Immediate: true})
	assert.ErrorIs(t, err, ErrAdtySiteTransfer)

	closed, err := ScheduleAdtySiteClose(site.Id, 1, AdtySiteClosePlan{Immediate: true})
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, closed.Status)
	assert.Zero(t, closed.CloseTransferTo)

	require.NoError(t, db.First(&inherited, teamID).Error)
	assert.Equal(t, teamID, inherited.Id)
	assert.Zero(t, inherited.SiteId)
	assert.Equal(t, site.Id, inherited.SourceSiteId)
	assert.Equal(t, model.AdtyTeamStatusActive, inherited.Status)
	assert.Equal(t, leader.Id, inherited.LeaderUserId)

	require.NoError(t, db.Where("team_id = ?", teamID).Order("id").Find(&members).Error)
	require.Len(t, members, 2)
	assert.Equal(t, leader.Id, members[0].UserId)
	assert.Equal(t, member.Id, members[1].UserId)
	assert.Equal(t, model.AdtyTeamMemberActive, members[0].Status)
	assert.Equal(t, model.AdtyTeamMemberActive, members[1].Status)

	require.NoError(t, db.Unscoped().First(&token, token.Id).Error)
	assert.True(t, token.DeletedAt.Valid)
	require.NoError(t, db.Where("token_id = ?", token.Id).First(&key).Error)
	assert.Equal(t, model.AdtyTeamMemberLeft, key.Status)

	require.NoError(t, db.First(&leftover, historical.Id).Error)
	assert.Zero(t, leftover.SiteId)
	assert.Zero(t, leftover.SourceSiteId)

	var storedLeader, storedMember model.User
	require.NoError(t, db.First(&storedLeader, leader.Id).Error)
	require.NoError(t, db.First(&storedMember, member.Id).Error)
	assert.Zero(t, storedLeader.SiteId)
	assert.Zero(t, storedMember.SiteId)
	assert.Equal(t, leader.Id, storedLeader.Id)
	assert.Equal(t, member.Id, storedMember.Id)
	assert.NotEqual(t, other.Id, storedLeader.SiteId)
	assert.NotEqual(t, other.Id, storedMember.SiteId)

	mine, err := GetAdtyTeamMine(leader.Id)
	require.NoError(t, err)
	require.NotNil(t, mine)
	assert.Equal(t, teamID, mine.TeamId)
	assert.Zero(t, mine.SiteId)
	assert.Equal(t, site.Id, mine.SourceSiteId)
	assert.Equal(t, model.AdtyTeamStatusActive, mine.Status)

	require.NoError(t, db.Model(&inherited).Updates(map[string]any{"site_id": 0, "source_site_id": 0}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return InheritAdtyTeamsOnCloseTx(tx, site.Id)
	}))
	require.NoError(t, db.First(&inherited, teamID).Error)
	assert.Zero(t, inherited.SiteId)
	assert.Zero(t, inherited.SourceSiteId)
}
