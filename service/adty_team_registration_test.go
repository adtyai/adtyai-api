package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func teamRegName(label string) string {
	name := fmt.Sprintf("r%s%d", label, time.Now().UnixNano()%1000000)
	if len(name) > 20 {
		return name[:20]
	}
	return name
}

func hashTeamRegPassword(t *testing.T, db *gorm.DB, user *model.User, password string) {
	t.Helper()
	hash, err := common.HashAccountPassword(password)
	require.NoError(t, err)
	require.NoError(t, db.Model(user).Update("password", hash).Error)
	user.Password = hash
}

func TestAdtyTeamRegistrationIssueAuthorization(t *testing.T) {
	db := prepareReferralDB(t)
	root := referralUser(t, db, teamRegName("rt"))
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	root.Role = common.RoleRootUser
	admin := referralUser(t, db, teamRegName("ad"))
	require.NoError(t, db.Model(admin).Update("role", common.RoleAdminUser).Error)
	user := referralUser(t, db, teamRegName("u"))
	ownerA := referralUser(t, db, teamRegName("oa"))
	ownerB := referralUser(t, db, teamRegName("ob"))
	siteA := model.AdtySite{OwnerUserId: ownerA.Id, Subdomain: teamRegName("sa"), Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	siteB := model.AdtySite{OwnerUserId: ownerB.Id, Subdomain: teamRegName("sb"), Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	require.NoError(t, db.Create(&siteA).Error)
	require.NoError(t, db.Create(&siteB).Error)
	staff := referralUser(t, db, teamRegName("st"))
	require.NoError(t, db.Model(staff).Updates(map[string]any{"site_id": siteA.Id, "staff_site_id": siteA.Id}).Error)

	_, err := CreateAdtyMainTeamRegistration(user.Id, AdtyTeamInviteDefaultTTL)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyMainTeamRegistration(admin.Id, AdtyTeamInviteDefaultTTL)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyMainTeamRegistration(ownerA.Id, AdtyTeamInviteDefaultTTL)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	_, err = CreateAdtyOwnerSiteTeamRegistration(user.Id, siteA.Id, AdtyTeamInviteDefaultTTL)
	require.Error(t, err)
	_, err = CreateAdtyOwnerSiteTeamRegistration(staff.Id, siteA.Id, AdtyTeamInviteDefaultTTL)
	require.Error(t, err)
	_, err = CreateAdtyOwnerSiteTeamRegistration(ownerB.Id, siteA.Id, AdtyTeamInviteDefaultTTL)
	require.Error(t, err)
	_, err = CreateAdtyOwnerSiteTeamRegistration(root.Id, siteA.Id, AdtyTeamInviteDefaultTTL)
	require.Error(t, err)
	_, err = CreateAdtyOwnerSiteTeamRegistration(ownerA.Id, 0, AdtyTeamInviteDefaultTTL)
	require.Error(t, err)
	_, err = CreateAdtyMainTeamRegistration(root.Id, 0)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
	_, err = CreateAdtyMainTeamRegistration(root.Id, adtyTeamInviteMaxTTL+time.Second)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)

	issued, err := CreateAdtyMainTeamRegistration(root.Id, AdtyTeamInviteDefaultTTL)
	require.NoError(t, err)
	assert.Equal(t, 0, issued.SiteId)
	assert.NotEmpty(t, issued.Code)
	assert.Equal(t, model.AdtyTeamRegistrationStatusOutstanding, issued.Status)
	assert.InDelta(t, time.Now().Unix()+int64(AdtyTeamInviteDefaultTTL.Seconds()), issued.ExpiresAt, 3)

	owned, err := CreateAdtyOwnerSiteTeamRegistration(ownerA.Id, siteA.Id, 2*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, siteA.Id, owned.SiteId)
	assert.NotEqual(t, issued.Code, owned.Code)
}

func TestAdtyTeamRegistrationCompleteBindsIssuanceSite(t *testing.T) {
	db := prepareReferralDB(t)
	root := referralUser(t, db, teamRegName("rt"))
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	root.Role = common.RoleRootUser
	owner := referralUser(t, db, teamRegName("ow"))
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: teamRegName("s"), Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	require.NoError(t, db.Create(&site).Error)
	other := model.AdtySite{OwnerUserId: owner.Id, Subdomain: teamRegName("s2"), Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	require.NoError(t, db.Create(&other).Error)

	issued, err := CreateAdtyOwnerSiteTeamRegistration(owner.Id, site.Id, time.Hour)
	require.NoError(t, err)
	wrongSite := other.Id
	_, err = CompleteAdtyTeamRegistration(issued.Code, teamRegName("nu"), "password1", "Desk", &wrongSite)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	var n int64
	require.NoError(t, db.Model(&model.AdtyTeam{}).Where("site_id = ?", site.Id).Count(&n).Error)
	assert.Zero(t, n)

	created, err := CompleteAdtyTeamRegistration(issued.Code, teamRegName("nu"), "password1", "Site Desk", nil)
	require.NoError(t, err)
	assert.Equal(t, site.Id, created.Team.SiteId)
	assert.Equal(t, "Site Desk", created.Team.Name)
	assert.Equal(t, model.AdtyTeamRoleLeader, created.Member.Role)
	var leader model.User
	require.NoError(t, db.First(&leader, created.Team.LeaderUserId).Error)
	assert.Equal(t, site.Id, leader.SiteId)
	assert.True(t, created.Team.Discount.Equal(decimal.NewFromInt(1)))

	_, err = CompleteAdtyTeamRegistration(issued.Code, teamRegName("n2"), "password1", "Again", nil)
	require.ErrorIs(t, err, ErrAdtyTeamRegistrationUsed)
}

func TestAdtyTeamRegistrationExistingIdentityAndDEC022(t *testing.T) {
	db := prepareReferralDB(t)
	owner := referralUser(t, db, teamRegName("ow"))
	siteA := model.AdtySite{OwnerUserId: owner.Id, Subdomain: teamRegName("sa"), Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	siteB := model.AdtySite{OwnerUserId: owner.Id, Subdomain: teamRegName("sb"), Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	require.NoError(t, db.Create(&siteA).Error)
	require.NoError(t, db.Create(&siteB).Error)

	sameSite := referralUser(t, db, teamRegName("ea"))
	require.NoError(t, db.Model(sameSite).Update("site_id", siteA.Id).Error)
	sameSite.SiteId = siteA.Id
	hashTeamRegPassword(t, db, sameSite, "password1")

	otherSite := referralUser(t, db, teamRegName("eb"))
	require.NoError(t, db.Model(otherSite).Update("site_id", siteB.Id).Error)
	otherSite.SiteId = siteB.Id
	hashTeamRegPassword(t, db, otherSite, "password1")

	issued, err := CreateAdtyOwnerSiteTeamRegistration(owner.Id, siteA.Id, time.Hour)
	require.NoError(t, err)

	_, err = CompleteAdtyTeamRegistration(issued.Code, sameSite.Username, "wrong-password", "Desk", nil)
	require.ErrorIs(t, err, ErrAdtyTeamRegistrationUnverified)
	var n int64
	require.NoError(t, db.Model(&model.AdtyTeam{}).Where("leader_user_id = ?", sameSite.Id).Count(&n).Error)
	assert.Zero(t, n)

	_, err = CompleteAdtyTeamRegistration(issued.Code, otherSite.Username, "password1", "Desk", nil)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	require.NoError(t, db.First(otherSite, otherSite.Id).Error)
	assert.Equal(t, siteB.Id, otherSite.SiteId)
	require.NoError(t, db.Model(&model.AdtyTeam{}).Where("leader_user_id = ?", otherSite.Id).Count(&n).Error)
	assert.Zero(t, n)

	created, err := CompleteAdtyTeamRegistration(issued.Code, sameSite.Username, "password1", "Existing Desk", nil)
	require.NoError(t, err)
	assert.Equal(t, sameSite.Id, created.Team.LeaderUserId)
	assert.Equal(t, siteA.Id, created.Team.SiteId)
	require.NoError(t, db.First(sameSite, sameSite.Id).Error)
	assert.Equal(t, siteA.Id, sameSite.SiteId)
}

func TestAdtyTeamRegistrationExpiryRevokeAndConcurrency(t *testing.T) {
	db := prepareReferralDB(t)
	root := referralUser(t, db, teamRegName("rt"))
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	root.Role = common.RoleRootUser
	admin := referralUser(t, db, teamRegName("ad"))
	require.NoError(t, db.Model(admin).Update("role", common.RoleAdminUser).Error)

	expired, err := CreateAdtyMainTeamRegistration(root.Id, time.Hour)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.AdtyTeamRegistrationLink{}).Where("id = ?", expired.Id).Update("expires_at", time.Now().Add(-time.Second).Unix()).Error)
	_, err = CompleteAdtyTeamRegistration(expired.Code, teamRegName("ex"), "password1", "Expired", nil)
	require.ErrorIs(t, err, ErrAdtyTeamRegistrationExpired)

	toRevoke, err := CreateAdtyMainTeamRegistration(root.Id, time.Hour)
	require.NoError(t, err)
	_, err = RevokeAdtyMainTeamRegistration(admin.Id, toRevoke.Id)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	revoked, err := RevokeAdtyMainTeamRegistration(root.Id, toRevoke.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTeamRegistrationStatusRevoked, revoked.Status)
	_, err = CompleteAdtyTeamRegistration(toRevoke.Code, teamRegName("rv"), "password1", "Revoked", nil)
	require.ErrorIs(t, err, ErrAdtyTeamRegistrationRevoked)

	listed, err := ListAdtyMainTeamRegistrations(root.Id)
	require.NoError(t, err)
	_, err = ListAdtyMainTeamRegistrations(admin.Id)
	require.ErrorIs(t, err, ErrAdtyTeamForbidden)
	statuses := map[int64]string{}
	for _, row := range listed {
		statuses[row.Id] = row.Status
		assert.NotContains(t, fmt.Sprintf("%+v", row), toRevoke.Code)
	}
	assert.Equal(t, model.AdtyTeamRegistrationStatusExpired, statuses[expired.Id])
	assert.Equal(t, model.AdtyTeamRegistrationStatusRevoked, statuses[toRevoke.Id])

	concurrent, err := CreateAdtyMainTeamRegistration(root.Id, time.Hour)
	require.NoError(t, err)
	var teams int64
	require.NoError(t, db.Model(&model.AdtyTeam{}).Count(&teams).Error)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = CompleteAdtyTeamRegistration(concurrent.Code, teamRegName(fmt.Sprintf("c%d", i)), "password1", fmt.Sprintf("Concurrent %d", i), nil)
		}(i)
	}
	wg.Wait()
	var success, fail int
	for _, err := range errs {
		if err == nil {
			success++
		} else {
			fail++
		}
	}
	assert.Equal(t, 1, success)
	assert.Equal(t, 1, fail)
	var after int64
	require.NoError(t, db.Model(&model.AdtyTeam{}).Count(&after).Error)
	assert.Equal(t, teams+1, after)
}

func TestAdtyTeamRegistrationInvalidFieldsLeaveNoOrphans(t *testing.T) {
	db := prepareReferralDB(t)
	root := referralUser(t, db, teamRegName("rt"))
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	root.Role = common.RoleRootUser
	issued, err := CreateAdtyMainTeamRegistration(root.Id, time.Hour)
	require.NoError(t, err)
	var users, teams int64
	require.NoError(t, db.Model(&model.User{}).Count(&users).Error)
	require.NoError(t, db.Model(&model.AdtyTeam{}).Count(&teams).Error)

	_, err = CompleteAdtyTeamRegistration(issued.Code, "", "password1", "Desk", nil)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
	_, err = CompleteAdtyTeamRegistration(issued.Code, "bad@name", "password1", "Desk", nil)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
	_, err = CompleteAdtyTeamRegistration(issued.Code, teamRegName("pw"), "short", "Desk", nil)
	require.Error(t, err)
	_, err = CompleteAdtyTeamRegistration(issued.Code, teamRegName("nm"), "password1", "", nil)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)

	var usersAfter, teamsAfter int64
	require.NoError(t, db.Model(&model.User{}).Count(&usersAfter).Error)
	require.NoError(t, db.Model(&model.AdtyTeam{}).Count(&teamsAfter).Error)
	assert.Equal(t, users, usersAfter)
	assert.Equal(t, teams, teamsAfter)
}

func TestAdtyTeamRegistrationDoesNotReplaceDesignatePath(t *testing.T) {
	db := prepareReferralDB(t)
	root := referralUser(t, db, teamRegName("rt"))
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	root.Role = common.RoleRootUser
	leader := referralUser(t, db, teamRegName("ld"))
	created, err := CreateAdtyMainTeamByRoot(root.Id, leader.Id, "Designate Desk", "0.80")
	require.NoError(t, err)
	assert.Equal(t, 0, created.Team.SiteId)
	assert.Equal(t, leader.Id, created.Team.LeaderUserId)
	assert.True(t, created.Team.Discount.Equal(decimal.RequireFromString("0.80")))
}
