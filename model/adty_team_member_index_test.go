package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupActiveTeamMembershipIndexDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&AdtyTeam{}, &AdtyTeamMember{}))
	return db
}

func createTeamMembershipFixture(t *testing.T, db *gorm.DB, name string) AdtyTeam {
	t.Helper()
	team := AdtyTeam{Name: name, Status: AdtyTeamStatusActive, Discount: decimal.NewFromInt(1)}
	require.NoError(t, db.Create(&team).Error)
	return team
}

func TestActiveTeamMembershipUniqueIndexRejectsSecondActiveRow(t *testing.T) {
	db := setupActiveTeamMembershipIndexDB(t)
	require.NoError(t, EnsureActiveTeamMembershipUniqueIndex(db))
	require.NoError(t, EnsureActiveTeamMembershipUniqueIndex(db), "installing the index twice is a no-op")
	assert.True(t, db.Migrator().HasIndex(AdtyTeamMember{}.TableName(), activeTeamMembershipUniqueIndex))
	assert.True(t, db.Migrator().HasIndex(&AdtyTeamMember{}, "idx_adty_team_member"))

	teamA := createTeamMembershipFixture(t, db, "team-a")
	teamB := createTeamMembershipFixture(t, db, "team-b")
	require.NoError(t, db.Create(&AdtyTeamMember{TeamId: teamA.Id, UserId: 42, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberActive}).Error)

	err := db.Create(&AdtyTeamMember{TeamId: teamB.Id, UserId: 42, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberActive}).Error
	require.Error(t, err)
	assert.True(t, isActiveTeamMembershipUniqueViolation(err) || isAdtyUniqueViolation(err), err)

	var active int64
	require.NoError(t, db.Model(&AdtyTeamMember{}).Where("user_id = ? AND status = ?", 42, AdtyTeamMemberActive).Count(&active).Error)
	assert.Equal(t, int64(1), active)
}

func TestLeftMembershipDoesNotBlockActiveMembershipOnAnotherTeam(t *testing.T) {
	db := setupActiveTeamMembershipIndexDB(t)
	require.NoError(t, EnsureActiveTeamMembershipUniqueIndex(db))

	teamA := createTeamMembershipFixture(t, db, "left-a")
	teamB := createTeamMembershipFixture(t, db, "active-b")
	teamC := createTeamMembershipFixture(t, db, "left-c")
	require.NoError(t, db.Create(&AdtyTeamMember{TeamId: teamA.Id, UserId: 7, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberLeft}).Error)
	require.NoError(t, db.Create(&AdtyTeamMember{TeamId: teamC.Id, UserId: 7, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberLeft}).Error)
	require.NoError(t, db.Create(&AdtyTeamMember{TeamId: teamB.Id, UserId: 7, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberActive}).Error)

	var rows []AdtyTeamMember
	require.NoError(t, db.Where("user_id = ?", 7).Order("team_id ASC").Find(&rows).Error)
	require.Len(t, rows, 3)
	var left, active int
	for _, row := range rows {
		switch row.Status {
		case AdtyTeamMemberLeft:
			left++
		case AdtyTeamMemberActive:
			active++
		}
	}
	assert.Equal(t, 2, left)
	assert.Equal(t, 1, active)
}

func TestSameTeamMemberUniqueIndexStillRejectsDuplicateRows(t *testing.T) {
	db := setupActiveTeamMembershipIndexDB(t)
	require.NoError(t, EnsureActiveTeamMembershipUniqueIndex(db))

	team := createTeamMembershipFixture(t, db, "same-team")
	require.NoError(t, db.Create(&AdtyTeamMember{TeamId: team.Id, UserId: 9, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberLeft}).Error)
	err := db.Create(&AdtyTeamMember{TeamId: team.Id, UserId: 9, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberLeft}).Error
	require.Error(t, err)
	assert.True(t, isActiveTeamMembershipUniqueViolation(err) || isAdtyUniqueViolation(err) || errors.Is(err, gorm.ErrDuplicatedKey), err)

	err = db.Create(&AdtyTeamMember{TeamId: team.Id, UserId: 9, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberActive}).Error
	require.Error(t, err)
	assert.True(t, isActiveTeamMembershipUniqueViolation(err) || isAdtyUniqueViolation(err) || errors.Is(err, gorm.ErrDuplicatedKey), err)

	var count int64
	require.NoError(t, db.Model(&AdtyTeamMember{}).Where("team_id = ? AND user_id = ?", team.Id, 9).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	assert.True(t, db.Migrator().HasIndex(&AdtyTeamMember{}, "idx_adty_team_member"))
}

func TestActiveTeamMembershipUniqueIndexRejectsConcurrentSecondInsert(t *testing.T) {
	db := setupActiveTeamMembershipIndexDB(t)
	require.NoError(t, EnsureActiveTeamMembershipUniqueIndex(db))

	teamA := createTeamMembershipFixture(t, db, "race-a")
	teamB := createTeamMembershipFixture(t, db, "race-b")

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, teamID := range []int64{teamA.Id, teamB.Id} {
		wg.Add(1)
		go func(teamID int64) {
			defer wg.Done()
			results <- db.Create(&AdtyTeamMember{TeamId: teamID, UserId: 100, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberActive}).Error
		}(teamID)
	}
	wg.Wait()
	close(results)

	var succeeded, conflicted int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case isActiveTeamMembershipUniqueViolation(err) || isAdtyUniqueViolation(err) || errors.Is(err, gorm.ErrDuplicatedKey):
			conflicted++
		default:
			t.Fatalf("unexpected insert error: %v", err)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, conflicted)

	var active int64
	require.NoError(t, db.Model(&AdtyTeamMember{}).Where("user_id = ? AND status = ?", 100, AdtyTeamMemberActive).Count(&active).Error)
	assert.Equal(t, int64(1), active)
}

func TestActiveTeamMembershipUniqueIndexSkipsHistoricalCollisions(t *testing.T) {
	db := setupActiveTeamMembershipIndexDB(t)

	teamA := createTeamMembershipFixture(t, db, "collision-a")
	teamB := createTeamMembershipFixture(t, db, "collision-b")
	first := AdtyTeamMember{TeamId: teamA.Id, UserId: 21, Role: AdtyTeamRoleLeader, Status: AdtyTeamMemberActive}
	second := AdtyTeamMember{TeamId: teamB.Id, UserId: 21, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberActive}
	left := AdtyTeamMember{TeamId: teamA.Id, UserId: 22, Role: AdtyTeamRoleMember, Status: AdtyTeamMemberLeft}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)
	require.NoError(t, db.Create(&left).Error)

	err := EnsureActiveTeamMembershipUniqueIndex(db)
	require.ErrorIs(t, err, ErrActiveTeamMembershipCollision)
	assert.False(t, db.Migrator().HasIndex(AdtyTeamMember{}.TableName(), activeTeamMembershipUniqueIndex))
	assert.True(t, db.Migrator().HasIndex(&AdtyTeamMember{}, "idx_adty_team_member"))

	var stored []AdtyTeamMember
	require.NoError(t, db.Where("id IN ?", []int64{first.Id, second.Id, left.Id}).Order("id ASC").Find(&stored).Error)
	require.Len(t, stored, 3)
	assert.Equal(t, first.Id, stored[0].Id)
	assert.Equal(t, first.TeamId, stored[0].TeamId)
	assert.Equal(t, first.UserId, stored[0].UserId)
	assert.Equal(t, AdtyTeamMemberActive, stored[0].Status)
	assert.Equal(t, second.Id, stored[1].Id)
	assert.Equal(t, second.TeamId, stored[1].TeamId)
	assert.Equal(t, second.UserId, stored[1].UserId)
	assert.Equal(t, AdtyTeamMemberActive, stored[1].Status)
	assert.Equal(t, left.Id, stored[2].Id)
	assert.Equal(t, AdtyTeamMemberLeft, stored[2].Status)

	var colliding int64
	require.NoError(t, db.Model(&AdtyTeamMember{}).Where("user_id = ? AND status = ?", 21, AdtyTeamMemberActive).Count(&colliding).Error)
	assert.Equal(t, int64(2), colliding, "historical colliding memberships were not merged or deleted")
}

func TestActiveTeamMembershipUniqueIndexIsInstalledByMigrateAdtyTables(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))
	require.NoError(t, MigrateAdtyTables())
	assert.True(t, db.Migrator().HasIndex(AdtyTeamMember{}.TableName(), activeTeamMembershipUniqueIndex))
	current, err := activeTeamMembershipUniqueIndexIsCurrent(db)
	require.NoError(t, err)
	if !current {
		var definition string
		require.NoError(t, db.Raw(
			"SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?",
			activeTeamMembershipUniqueIndex,
		).Scan(&definition).Error)
		t.Fatalf("index exists but is not recognized as current: %s", definition)
	}
	assert.True(t, db.Migrator().HasIndex(&AdtyTeamMember{}, "idx_adty_team_member"))
}
