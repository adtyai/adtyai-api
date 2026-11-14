package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdtyTeamActiveMembershipConstraintAndErrorMapping(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	require.True(t, db.Migrator().HasIndex(&model.AdtyTeamMember{}, "ux_adty_team_members_one_active_user"),
		"MigrateAdtyTables must install the one-active-user index")
	require.True(t, db.Migrator().HasIndex(&model.AdtyTeamMember{}, "idx_adty_team_member"))

	leaderA := referralUser(t, db, "idx-leader-a")
	leaderB := referralUser(t, db, "idx-leader-b")
	member := referralUser(t, db, "idx-member")
	require.NoError(t, db.Model(leaderA).Update("site_id", 0).Error)
	require.NoError(t, db.Model(leaderB).Update("site_id", 0).Error)
	require.NoError(t, db.Model(member).Update("site_id", 0).Error)

	teamA, err := CreateAdtyTeam(leaderA.Id, 0, "Alpha", decimal.NewFromInt(1))
	require.NoError(t, err)
	teamB, err := CreateAdtyTeam(leaderB.Id, 0, "Beta", decimal.NewFromInt(1))
	require.NoError(t, err)

	joined, err := JoinAdtyTeam(member.Id, teamA.Team.Id)
	require.NoError(t, err)
	assert.Equal(t, teamA.Team.Id, joined.TeamId)

	_, err = JoinAdtyTeam(member.Id, teamB.Team.Id)
	require.ErrorIs(t, err, ErrAdtyTeamBusy)
	assertActiveMembershipCount(t, member.Id, 1)

	// Bypass the service lock and count check: the database must still reject
	// a second active row, and the mapper must return the official error.
	direct := model.AdtyTeamMember{TeamId: teamB.Team.Id, UserId: member.Id, Role: model.AdtyTeamRoleMember, Status: model.AdtyTeamMemberActive}
	err = db.Create(&direct).Error
	require.Error(t, err)
	require.ErrorIs(t, mapAdtyTeamMemberWriteError(err), ErrAdtyTeamBusy)
	assert.True(t, model.IsActiveTeamMembershipUniqueViolation(err) || model.IsAdtyUniqueViolation(err), err)
	assertActiveMembershipCount(t, member.Id, 1)

	require.NoError(t, LeaveAdtyTeam(member.Id, teamA.Team.Id))
	assertActiveMembershipCount(t, member.Id, 0)
	var left int64
	require.NoError(t, db.Model(&model.AdtyTeamMember{}).Where("user_id = ? AND team_id = ? AND status = ?", member.Id, teamA.Team.Id, model.AdtyTeamMemberLeft).Count(&left).Error)
	assert.Equal(t, int64(1), left)

	rejoined, err := JoinAdtyTeam(member.Id, teamB.Team.Id)
	require.NoError(t, err)
	assert.Equal(t, teamB.Team.Id, rejoined.TeamId)
	assert.Equal(t, model.AdtyTeamMemberActive, rejoined.Status)
	assertActiveMembershipCount(t, member.Id, 1)
	require.NoError(t, db.Model(&model.AdtyTeamMember{}).Where("user_id = ? AND team_id = ? AND status = ?", member.Id, teamA.Team.Id, model.AdtyTeamMemberLeft).Count(&left).Error)
	assert.Equal(t, int64(1), left, "left history on the previous team must remain")

	// Same-team unique index still rejects a second row for the same pair.
	duplicate := model.AdtyTeamMember{TeamId: teamB.Team.Id, UserId: member.Id, Role: model.AdtyTeamRoleMember, Status: model.AdtyTeamMemberActive}
	err = db.Create(&duplicate).Error
	require.Error(t, err)
	require.ErrorIs(t, mapAdtyTeamMemberWriteError(err), ErrAdtyTeamBusy)
	var sameTeam int64
	require.NoError(t, db.Model(&model.AdtyTeamMember{}).Where("team_id = ? AND user_id = ?", teamB.Team.Id, member.Id).Count(&sameTeam).Error)
	assert.Equal(t, int64(1), sameTeam)
}

func assertActiveMembershipCount(t *testing.T, userID int, want int64) {
	t.Helper()
	var count int64
	require.NoError(t, model.DB.Model(&model.AdtyTeamMember{}).Where("user_id = ? AND status = ?", userID, model.AdtyTeamMemberActive).Count(&count).Error)
	assert.Equal(t, want, count)
}
