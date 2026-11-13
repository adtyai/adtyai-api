package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUsernameTestState(t *testing.T) {
	t.Helper()
	setupUserUpdateTestState(t)
}

func createUsernameTestUser(t *testing.T, username string, siteID int) User {
	t.Helper()
	user := User{
		Username: username,
		Password: "password-123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		SiteId:   siteID,
	}
	require.NoError(t, user.Insert(0))
	return user
}

func TestNormalizeUsernameFoldsCaseAndTrim(t *testing.T) {
	assert.Equal(t, "alice", NormalizeUsername(" Alice "))
	assert.Equal(t, "alice", NormalizeUsername("ALICE"))
	assert.Equal(t, "alice", NormalizeUsername("alice"))
}

func TestUsernameCaseVariantRegistrationIsRejected(t *testing.T) {
	setupUsernameTestState(t)
	createUsernameTestUser(t, "Alice", 0)

	exist, err := CheckUserExistOrDeleted("alice", "", 0)
	require.NoError(t, err)
	assert.True(t, exist)
	exist, err = CheckUserExistOrDeleted("ALICE", "", 7)
	require.NoError(t, err)
	assert.True(t, exist, "username uniqueness is platform-wide, not site-scoped")

	variant := User{Username: "ALICE", Password: "password-123", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, SiteId: 7}
	require.ErrorIs(t, variant.Insert(0), ErrUsernameAlreadyTaken)

	other := createUsernameTestUser(t, "Bob", 7)
	assert.Equal(t, "Bob", other.Username)
	assert.NotZero(t, other.Id)
}

func TestUsernameRenameRejectsCaseVariantOfAnotherAccount(t *testing.T) {
	setupUsernameTestState(t)
	existing := createUsernameTestUser(t, "Carol", 0)
	target := createUsernameTestUser(t, "Dave", 0)

	target.Username = "carol"
	require.ErrorIs(t, target.Edit(false), ErrUsernameAlreadyTaken)
	var stored User
	require.NoError(t, DB.First(&stored, target.Id).Error)
	assert.Equal(t, "Dave", stored.Username)

	existing.Username = "CAROL"
	require.NoError(t, existing.Edit(false))
	var renamed User
	require.NoError(t, DB.First(&renamed, existing.Id).Error)
	assert.Equal(t, "CAROL", renamed.Username, "an account may change only the display spelling of its own username")
}

func TestUsernameLoginIsCaseInsensitiveAndPreservesDisplaySpelling(t *testing.T) {
	setupUsernameTestState(t)
	created := createUsernameTestUser(t, "Alice", 0)

	for _, typed := range []string{"Alice", "alice", "ALICE", "AlIce"} {
		login := User{Username: typed, Password: "password-123"}
		require.NoError(t, login.ValidateAndFill(), typed)
		assert.Equal(t, "Alice", login.Username, typed)
		assert.Equal(t, created.Id, login.Id, typed)
	}

	missing := User{Username: "Alice1", Password: "password-123"}
	require.ErrorIs(t, missing.ValidateAndFill(), ErrInvalidCredentials)
}

func TestUsernameLegacySoftDeletedRowStillOccupiesCaseVariants(t *testing.T) {
	setupUsernameTestState(t)
	hash, err := common.HashAccountPassword("password-123")
	require.NoError(t, err)
	legacy := User{Username: "Alice", Password: hash, Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&legacy).Error)
	require.NoError(t, DB.Delete(&legacy).Error)

	var tombstoned int64
	require.NoError(t, DB.Unscoped().Model(&User{}).Where("id = ? AND deleted_at IS NOT NULL AND username = ?", legacy.Id, "Alice").Count(&tombstoned).Error)
	require.Equal(t, int64(1), tombstoned)

	exist, err := CheckUserExistOrDeleted("alice", "", 0)
	require.NoError(t, err)
	assert.True(t, exist)

	variant := User{Username: "alice", Password: "password-123", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.ErrorIs(t, variant.Insert(0), ErrUsernameAlreadyTaken)
}

func TestUsernameRuleDoesNotChangeSiteScopedEmailUniqueness(t *testing.T) {
	setupUsernameTestState(t)
	createUsernameTestUser(t, "mail-user-a", 0)
	require.NoError(t, DB.Model(&User{}).Where("username = ?", "mail-user-a").Update("email", "shared@example.com").Error)

	sameSite := User{Username: "mail-user-b", Password: "password-123", Email: "Shared@Example.com", SiteId: 0, Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.ErrorIs(t, sameSite.Insert(0), ErrEmailAlreadyTaken)

	otherSite := User{Username: "mail-user-c", Password: "password-123", Email: "shared@example.com", SiteId: 8, Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, otherSite.Insert(0))
	assert.Equal(t, "shared@example.com", otherSite.Email)
	assert.Equal(t, 8, otherSite.SiteId)
}

func TestUsernameUniqueIndexRejectsConcurrentCaseVariants(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))
	require.NoError(t, EnsureUsernameUniqueIndex(db))
	require.NoError(t, EnsureUsernameUniqueIndex(db), "installing the index twice is a no-op")

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"Alice", "alice"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			user := User{Username: name, Password: "password-123", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
			results <- user.Insert(0)
		}(name)
	}
	wg.Wait()
	close(results)

	var succeeded, taken int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrUsernameAlreadyTaken):
			taken++
		default:
			t.Fatalf("unexpected insert error: %v", err)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, taken)

	var count int64
	require.NoError(t, db.Unscoped().Model(&User{}).Where("LOWER(username) = ?", "alice").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestUsernameUniqueIndexSkipsHistoricalCaseCollisions(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))

	hash, err := common.HashAccountPassword("correct-horse-1")
	require.NoError(t, err)
	require.NoError(t, db.Create(&User{Username: "Alice", Password: hash, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
	require.NoError(t, db.Create(&User{Username: "alice", Password: hash, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, SiteId: 3}).Error)

	err = EnsureUsernameUniqueIndex(db)
	require.ErrorIs(t, err, ErrUsernameCaseCollision)

	exactAlice := User{Username: "Alice", Password: "correct-horse-1"}
	require.NoError(t, exactAlice.ValidateAndFill())
	assert.Equal(t, "Alice", exactAlice.Username)
	exactLower := User{Username: "alice", Password: "correct-horse-1"}
	require.NoError(t, exactLower.ValidateAndFill())
	assert.Equal(t, "alice", exactLower.Username)

	ambiguous := User{Username: "ALICE", Password: "correct-horse-1"}
	require.ErrorIs(t, ambiguous.ValidateAndFill(), ErrInvalidCredentials)

	newbie := User{Username: "ALICE", Password: "password-123", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.ErrorIs(t, newbie.Insert(0), ErrUsernameAlreadyTaken)

	other := User{Username: "Bob", Password: "password-123", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, other.Insert(0))
	assert.Equal(t, "Bob", other.Username)

	var colliding int64
	require.NoError(t, db.Unscoped().Model(&User{}).Where("LOWER(username) = ?", "alice").Count(&colliding).Error)
	assert.Equal(t, int64(2), colliding, "historical colliding accounts were not merged or renamed")
}

func TestUsernameUniqueIndexExistsAfterEnsure(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, EnsureUsernameUniqueIndex(db))
	current, err := usernameUniqueIndexIsCurrent(db)
	require.NoError(t, err)
	assert.True(t, current)
	assert.True(t, db.Migrator().HasIndex("users", usernameUniqueIndex))

	// Direct writes must also be rejected by the database constraint.
	require.NoError(t, db.Create(&User{Username: "Eve", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
	err = db.Create(&User{Username: "eve", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error
	require.Error(t, err)
	assert.True(t, isUsernameUniqueViolation(err) || errors.Is(err, gorm.ErrDuplicatedKey) || isAdtyUniqueViolation(err), err)
}
