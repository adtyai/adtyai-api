package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
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
	marked, k := AdtyApplySiteMarkup(ctx, member.Id, "default", 2)
	assert.Equal(t, 1.0, k)
	assert.InDelta(t, 1.0, marked, 0.000001)
	assert.False(t, AdtyTeamPriceBlocked(ctx))

	personal, _ := gin.CreateTestContext(httptest.NewRecorder())
	personal.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	unchanged, _ := AdtyApplySiteMarkup(personal, member.Id, "default", 2)
	assert.InDelta(t, 2.0, unchanged, 0.000001)
}

func decimalPtr(value string) *decimal.Decimal {
	parsed := decimal.RequireFromString(value)
	return &parsed
}
