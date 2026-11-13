package model

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuiltinOAuthSubjectIsUniqueUnderConcurrency(t *testing.T) {
	truncateTables(t)
	require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(DB))

	identities := make([]AuthSessionIdentity, 2)
	for i := range identities {
		user := User{
			Username: fmt.Sprintf("oauth-owner-%d", i), Password: "password",
			Status: common.UserStatusEnabled, AuthVersion: 1,
		}
		require.NoError(t, DB.Create(&user).Error)
		session := UserSession{
			SID: fmt.Sprintf("oauth-session-%d", i), UserID: user.Id, Version: 1,
			UserAuthVersion: 1, Status: UserSessionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}
		require.NoError(t, DB.Create(&session).Error)
		identities[i] = AuthSessionIdentity{
			UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1,
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, len(identities))
	for i := range identities {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = DB.Transaction(func(tx *gorm.DB) error {
				return UpdateUserBindColumnForSessionWithTx(tx, identities[i], "github_id", "gh-shared")
			})
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)
	}
	assert.Equal(t, 1, wins)
	var count int64
	require.NoError(t, DB.Model(&User{}).Where("github_id = ?", "gh-shared").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestWeChatSubjectIsUniqueUnderConcurrency(t *testing.T) {
	truncateTables(t)
	require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(DB))

	identities := make([]AuthSessionIdentity, 2)
	for i := range identities {
		user := User{
			Username: fmt.Sprintf("wechat-owner-%d", i), Password: "password",
			Status: common.UserStatusEnabled, AuthVersion: 1,
		}
		require.NoError(t, DB.Create(&user).Error)
		session := UserSession{
			SID: fmt.Sprintf("wechat-session-%d", i), UserID: user.Id, Version: 1,
			UserAuthVersion: 1, Status: UserSessionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}
		require.NoError(t, DB.Create(&session).Error)
		identities[i] = AuthSessionIdentity{
			UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1,
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, len(identities))
	for i := range identities {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = DB.Transaction(func(tx *gorm.DB) error {
				return UpdateUserBindColumnForSessionWithTx(tx, identities[i], "wechat_id", "wx-shared")
			})
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)
	}
	assert.Equal(t, 1, wins)
	var count int64
	require.NoError(t, DB.Model(&User{}).Where("wechat_id = ?", "wx-shared").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestNewOAuthUserCannotReuseBuiltinSubject(t *testing.T) {
	truncateTables(t)
	require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(DB))

	create := func(name, subject string) error {
		user := &User{
			Username: name, Password: "password", Status: common.UserStatusEnabled, AuthVersion: 1,
		}
		return DB.Transaction(func(tx *gorm.DB) error {
			if err := user.InsertWithTx(tx, 0); err != nil {
				return err
			}
			user.GitHubId = subject
			return AssignBuiltinProviderIDsWithTx(tx, user)
		})
	}
	require.NoError(t, create("oauth-new-1", "gh-new"))
	assert.ErrorIs(t, create("oauth-new-2", "gh-new"), ErrExternalIdentityAlreadyClaimed)

	var owners int64
	require.NoError(t, DB.Model(&User{}).Where("github_id = ?", "gh-new").Count(&owners).Error)
	assert.EqualValues(t, 1, owners)
	var rolledBack int64
	require.NoError(t, DB.Model(&User{}).Where("username = ?", "oauth-new-2").Count(&rolledBack).Error)
	assert.Zero(t, rolledBack)
}

func TestBuiltinOAuthSubjectIndexRejectsExistingDuplicates(t *testing.T) {
	truncateTables(t)
	for _, column := range []string{"github_id", "discord_id", "oidc_id", "linux_do_id"} {
		require.NoError(t, DB.Exec("DROP INDEX IF EXISTS ux_users_"+column+"_subject").Error)
		require.NoError(t, DB.Exec("DROP INDEX IF EXISTS ux_users_"+column+"_site_subject").Error)
	}
	require.NoError(t, DB.Create(&User{Username: "oauth-dupe-1", Password: "password", GitHubId: "gh-dupe"}).Error)
	require.NoError(t, DB.Create(&User{Username: "oauth-dupe-2", Password: "password", GitHubId: "gh-dupe"}).Error)
	err := EnsureBuiltinOAuthSubjectIndexes(DB)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "github_id")
}
