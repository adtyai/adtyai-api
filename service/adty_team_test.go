package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	code, err := CreateAdtyTeamInvite(leader.Id, created.Team.Id, time.Hour)
	require.NoError(t, err)
	joined, err := AcceptAdtyTeamInvite(member.Id, code)
	require.NoError(t, err)
	assert.Equal(t, created.Team.Id, joined.TeamId)
	_, err = AcceptAdtyTeamInvite(member.Id, code)
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

func decimalPtr(value string) *decimal.Decimal {
	parsed := decimal.RequireFromString(value)
	return &parsed
}
