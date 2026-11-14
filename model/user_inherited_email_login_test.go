package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInheritedMainEmailRequiresUsernameThenPassword(t *testing.T) {
	setupUserUpdateTestState(t)
	mainHash, err := common.HashAccountPassword("main-password-1")
	require.NoError(t, err)
	inheritedHash, err := common.HashAccountPassword("inherited-password-1")
	require.NoError(t, err)

	main := User{
		Username: "inherit-main-user", Password: mainHash, Email: "shared-login@example.com",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser,
	}
	inherited := User{
		Username: "inherit-closed-user", Password: inheritedHash, Email: "SHARED-LOGIN@example.com",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser, InheritedFromSiteId: 9,
	}
	require.NoError(t, DB.Create(&main).Error)
	require.NoError(t, DB.Create(&inherited).Error)
	require.NotEqual(t, main.Id, inherited.Id)

	_, err = GetUniqueUserByEmail("shared-login@example.com", 0)
	require.ErrorIs(t, err, ErrEmailAmbiguous)

	emailOnly := User{Username: "shared-login@example.com", Password: "main-password-1"}
	require.ErrorIs(t, emailOnly.ValidateAndFill(), ErrEmailAmbiguous)
	assert.Equal(t, "shared-login@example.com", emailOnly.Username)

	byMainName := User{Username: "inherit-main-user", Password: "main-password-1"}
	require.NoError(t, byMainName.ValidateAndFill())
	assert.Equal(t, main.Id, byMainName.Id)
	assert.Equal(t, "inherit-main-user", byMainName.Username)
	assert.Zero(t, byMainName.InheritedFromSiteId)

	byInheritedName := User{Username: "Inherit-Closed-User", Password: "inherited-password-1"}
	require.NoError(t, byInheritedName.ValidateAndFill())
	assert.Equal(t, inherited.Id, byInheritedName.Id)
	assert.Equal(t, 9, byInheritedName.InheritedFromSiteId)

	wrongPassword := User{Username: "inherit-main-user", Password: "inherited-password-1"}
	require.ErrorIs(t, wrongPassword.ValidateAndFill(), ErrInvalidCredentials)
	missingName := User{Username: "nobody-here", Password: "main-password-1"}
	require.ErrorIs(t, missingName.ValidateAndFill(), ErrInvalidCredentials)

	require.ErrorIs(t, ResetUserPasswordByEmail("shared-login@example.com", "NewPassword123", 0), ErrEmailAmbiguous)
	var stillMain, stillInherited User
	require.NoError(t, DB.First(&stillMain, main.Id).Error)
	require.NoError(t, DB.First(&stillInherited, inherited.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("main-password-1", stillMain.Password))
	assert.True(t, common.ValidatePasswordAndHash("inherited-password-1", stillInherited.Password))
	assert.Equal(t, main.Id, stillMain.Id)
	assert.Equal(t, inherited.Id, stillInherited.Id)
	assert.Equal(t, "shared-login@example.com", NormalizeEmail(stillMain.Email))
	assert.Equal(t, NormalizeEmail(stillMain.Email), NormalizeEmail(stillInherited.Email))

	_, err = ResolveUserForEmailChallenge("shared-login@example.com", "nobody-here", 0)
	require.ErrorIs(t, err, ErrEmailNotFound)
	_, err = ResolveUserForEmailChallenge("shared-login@example.com", "inherit-closed-user", 0)
	require.NoError(t, err)
	_, err = ResolveUserForEmailChallenge("other@example.com", "inherit-main-user", 0)
	require.ErrorIs(t, err, ErrEmailNotFound)

	_, err = ResetUserPasswordByIdentity("shared-login@example.com", "inherit-closed-user", "InheritedReset123", 0)
	require.NoError(t, err)
	require.NoError(t, DB.First(&stillMain, main.Id).Error)
	require.NoError(t, DB.First(&stillInherited, inherited.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("main-password-1", stillMain.Password), "the other account must keep its password")
	assert.True(t, common.ValidatePasswordAndHash("InheritedReset123", stillInherited.Password))
	assert.Equal(t, "inherit-main-user", stillMain.Username)
	assert.Equal(t, "inherit-closed-user", stillInherited.Username)
	assert.NotEqual(t, stillMain.Id, stillInherited.Id)

	relogin := User{Username: "inherit-closed-user", Password: "InheritedReset123"}
	require.NoError(t, relogin.ValidateAndFill())
	assert.Equal(t, inherited.Id, relogin.Id)
}

func TestPasswordResetCodeKeyBindsUserAndMigration(t *testing.T) {
	main := &User{Id: 11, Username: "Main-User", Email: "Shared@example.com", SiteId: 0}
	inherited := &User{Id: 12, Username: "closed-user", Email: "shared@example.com", SiteId: 0, InheritedFromSiteId: 9}
	assert.Equal(t, "0:shared@example.com:11:main-user:0", PasswordResetCodeKey(main))
	assert.Equal(t, "0:shared@example.com:12:closed-user:9", PasswordResetCodeKey(inherited))
	assert.NotEqual(t, PasswordResetCodeKey(main), PasswordResetCodeKey(inherited))
	assert.True(t, len(PasswordResetCodeKey(main)) > len(SiteEmailCodeKey(main.Email, 0)))
}
