package model

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSiteEmailDB(t *testing.T) {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &UserSession{}, &UserAccessToken{}))
	require.NoError(t, EnsureSiteEmailUniqueIndex(db))
}

func insertSiteEmailUser(t *testing.T, username, email, password string, siteID int) *User {
	t.Helper()
	user := &User{
		Username: username,
		Password: password,
		Email:    email,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		SiteId:   siteID,
	}
	require.NoError(t, user.Insert(0))
	require.NoError(t, DB.Where("username = ?", username).First(user).Error)
	return user
}

func TestSiteEmailAccountsAreIndependent(t *testing.T) {
	setupSiteEmailDB(t)

	mainUser := insertSiteEmailUser(t, "main-shared-mail", " Shared@Example.com ", "MainPass123", 0)
	siteUser := insertSiteEmailUser(t, "site-shared-mail", "shared@example.com", "SitePass123", 11)

	assert.NotEqual(t, mainUser.Id, siteUser.Id)
	assert.Equal(t, "shared@example.com", mainUser.Email)
	assert.Equal(t, "shared@example.com", siteUser.Email)
	assert.Zero(t, mainUser.SiteId)
	assert.Equal(t, 11, siteUser.SiteId)
	assert.True(t, common.ValidatePasswordAndHash("MainPass123", mainUser.Password))
	assert.True(t, common.ValidatePasswordAndHash("SitePass123", siteUser.Password))
	assert.False(t, common.ValidatePasswordAndHash("MainPass123", siteUser.Password))

	require.ErrorIs(t, EnsureEmailAvailable("SHARED@example.com", 0, 0), ErrEmailAlreadyTaken)
	require.ErrorIs(t, EnsureEmailAvailable("shared@example.com", 11, 0), ErrEmailAlreadyTaken)
	require.NoError(t, EnsureEmailAvailable("shared@example.com", 12, 0))
	require.NoError(t, EnsureEmailAvailable("shared@example.com", 0, mainUser.Id))

	assert.True(t, IsEmailAlreadyTaken("shared@example.com", 0))
	assert.False(t, IsEmailAlreadyTaken("shared@example.com", 12))

	sameSite := &User{
		Username: "same-site-dup",
		Password: "OtherPass123",
		Email:    "SHARED@example.COM",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		SiteId:   0,
	}
	require.ErrorIs(t, sameSite.Insert(0), ErrEmailAlreadyTaken)

	foundMain, err := GetUniqueUserByEmail("SHARED@example.com", 0)
	require.NoError(t, err)
	assert.Equal(t, mainUser.Id, foundMain.Id)
	foundSite, err := GetUniqueUserByEmail("shared@example.com", 11)
	require.NoError(t, err)
	assert.Equal(t, siteUser.Id, foundSite.Id)
	_, err = GetUniqueUserByEmail("shared@example.com", 12)
	require.ErrorIs(t, err, ErrEmailNotFound)

	loginMain := User{Username: "shared@example.com", Password: "MainPass123"}
	require.NoError(t, loginMain.ValidateAndFill())
	assert.Equal(t, mainUser.Id, loginMain.Id)
	loginSite := User{Username: "Shared@Example.com", Password: "SitePass123", SiteId: 11}
	require.NoError(t, loginSite.ValidateAndFill())
	assert.Equal(t, siteUser.Id, loginSite.Id)
	wrongSite := User{Username: "shared@example.com", Password: "MainPass123", SiteId: 11}
	require.ErrorIs(t, wrongSite.ValidateAndFill(), ErrInvalidCredentials)

	require.NoError(t, ResetUserPasswordByEmail("shared@example.com", "MainReset123", 0))
	require.NoError(t, DB.First(mainUser, mainUser.Id).Error)
	require.NoError(t, DB.First(siteUser, siteUser.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("MainReset123", mainUser.Password))
	assert.True(t, common.ValidatePasswordAndHash("SitePass123", siteUser.Password))
	require.NoError(t, ResetUserPasswordByEmail("SHARED@example.com", "SiteReset123", 11))
	require.NoError(t, DB.First(mainUser, mainUser.Id).Error)
	require.NoError(t, DB.First(siteUser, siteUser.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("MainReset123", mainUser.Password))
	assert.True(t, common.ValidatePasswordAndHash("SiteReset123", siteUser.Password))

	require.NoError(t, DB.Model(mainUser).Update("status", common.UserStatusDisabled).Error)
	require.NoError(t, DB.First(siteUser, siteUser.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, siteUser.Status)
	disabledLogin := User{Username: "shared@example.com", Password: "MainReset123"}
	require.ErrorIs(t, disabledLogin.ValidateAndFill(), ErrInvalidCredentials)
	stillEnabled := User{Username: "shared@example.com", Password: "SiteReset123", SiteId: 11}
	require.NoError(t, stillEnabled.ValidateAndFill())
	assert.Equal(t, siteUser.Id, stillEnabled.Id)

	takenName := &User{
		Username: mainUser.Username,
		Password: "OtherPass123",
		Email:    "other-on-site@example.com",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		SiteId:   11,
	}
	err = takenName.Insert(0)
	require.Error(t, err)
	assert.True(t, isAdtyUniqueViolation(err), err.Error())
	exist, err := CheckUserExistOrDeleted(mainUser.Username, "", 11)
	require.NoError(t, err)
	assert.True(t, exist)
}

func TestSiteEmailUniqueIndexRejectsSameSiteDuplicates(t *testing.T) {
	setupSiteEmailDB(t)
	require.NoError(t, DB.Create(&User{Username: "idx-main", Password: "x", Email: "index@example.com", Status: common.UserStatusEnabled}).Error)
	err := DB.Create(&User{Username: "idx-dup", Password: "x", Email: "INDEX@example.com", Status: common.UserStatusEnabled}).Error
	require.Error(t, err)
	assert.True(t, isSiteEmailUniqueViolation(err))
	require.NoError(t, DB.Create(&User{Username: "idx-site", Password: "x", Email: "index@example.com", Status: common.UserStatusEnabled, SiteId: 4}).Error)
	require.NoError(t, DB.Create(&User{Username: "idx-blank-1", Password: "x", Email: "", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Username: "idx-blank-2", Password: "x", Email: "", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, EnsureSiteEmailUniqueIndex(DB))
}

func TestSiteEmailUniqueIndexRejectsExistingDuplicates(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Create(&User{Username: "dup-a", Password: "x", Email: "dup@example.com"}).Error)
	require.NoError(t, db.Exec("INSERT INTO users (username, password, email, site_id, role, status) VALUES ('dup-b', 'x', 'DUP@example.com', 0, 1, 1)").Error)
	err := EnsureSiteEmailUniqueIndex(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate site emails")
}

func TestConcurrentSiteEmailRegistration(t *testing.T) {
	setupSiteEmailDB(t)

	t.Run("same site rejects the second writer", func(t *testing.T) {
		errs := make([]error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		for i := 0; i < 2; i++ {
			i := i
			go func() {
				defer wg.Done()
				user := &User{
					Username: "same-site-race-" + string(rune('a'+i)),
					Password: "Password123",
					Email:    "race-same@example.com",
					Role:     common.RoleCommonUser,
					Status:   common.UserStatusEnabled,
				}
				errs[i] = user.Insert(0)
			}()
		}
		wg.Wait()
		var taken, ok int
		for _, err := range errs {
			if err == nil {
				ok++
				continue
			}
			require.ErrorIs(t, err, ErrEmailAlreadyTaken)
			taken++
		}
		assert.Equal(t, 1, ok)
		assert.Equal(t, 1, taken)
		var count int64
		require.NoError(t, DB.Model(&User{}).Where("LOWER(email) = ? AND site_id = 0", "race-same@example.com").Count(&count).Error)
		assert.EqualValues(t, 1, count)
	})

	t.Run("cross site both succeed", func(t *testing.T) {
		errs := make([]error, 2)
		sites := []int{0, 15}
		var wg sync.WaitGroup
		wg.Add(2)
		for i, siteID := range sites {
			i, siteID := i, siteID
			go func() {
				defer wg.Done()
				user := &User{
					Username: "cross-site-race-" + string(rune('a'+i)),
					Password: "Password123",
					Email:    "race-cross@example.com",
					Role:     common.RoleCommonUser,
					Status:   common.UserStatusEnabled,
					SiteId:   siteID,
				}
				errs[i] = user.Insert(0)
			}()
		}
		wg.Wait()
		for i, err := range errs {
			require.NoError(t, err, "site %d", sites[i])
		}
		var users []User
		require.NoError(t, DB.Where("LOWER(email) = ?", "race-cross@example.com").Order("site_id").Find(&users).Error)
		require.Len(t, users, 2)
		assert.Equal(t, 0, users[0].SiteId)
		assert.Equal(t, 15, users[1].SiteId)
		assert.NotEqual(t, users[0].Id, users[1].Id)
	})
}

func TestSiteEmailCodeKeyIsScoped(t *testing.T) {
	assert.Equal(t, "0:user@example.com", SiteEmailCodeKey(" User@Example.com ", 0))
	assert.Equal(t, "8:user@example.com", SiteEmailCodeKey("user@example.com", 8))
	assert.NotEqual(t, SiteEmailCodeKey("user@example.com", 0), SiteEmailCodeKey("user@example.com", 8))
}
