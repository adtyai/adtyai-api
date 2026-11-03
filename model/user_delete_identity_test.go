package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSoftDeleteReleasesUsernameEmailAndWeChat(t *testing.T) {
	user := &User{
		Username: "alice-delete", Password: "hash", Email: "alice-delete@example.com",
		WeChatId: "wx-alice-delete", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		AuthVersion: 1,
	}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() {
		_ = DB.Unscoped().Delete(&User{}, user.Id).Error
	})

	_, err := user.Delete()
	require.NoError(t, err)

	var stored User
	require.NoError(t, DB.Unscoped().First(&stored, user.Id).Error)
	assert.NotNil(t, stored.DeletedAt)
	assert.NotEqual(t, "alice-delete", stored.Username)
	assert.Empty(t, stored.Email)
	assert.Empty(t, stored.WeChatId)
	assert.False(t, IsWeChatIdAlreadyTaken("wx-alice-delete"))
	assert.False(t, IsEmailAlreadyTaken("alice-delete@example.com"))

	var stillNamed int64
	require.NoError(t, DB.Unscoped().Model(&User{}).Where("username = ?", "alice-delete").Count(&stillNamed).Error)
	assert.Zero(t, stillNamed)
}

func TestOpenSiteOwnerCannotBeDeleted(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AdtySite{}))
	owner := &User{
		Username: "site-owner-delete", Password: "hash", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, DB.Create(owner).Error)
	site := &AdtySite{OwnerUserId: owner.Id, Subdomain: "owner-delete", Status: AdtySiteStatusActive}
	require.NoError(t, DB.Create(site).Error)
	t.Cleanup(func() {
		_ = DB.Unscoped().Delete(&AdtySite{}, site.Id).Error
		_ = DB.Unscoped().Delete(&User{}, owner.Id).Error
	})

	_, err := owner.Delete()
	require.ErrorIs(t, err, ErrSiteOwnerCannotDelete)
	_, err = owner.HardDelete()
	require.ErrorIs(t, err, ErrSiteOwnerCannotDelete)
	var stillThere int64
	require.NoError(t, DB.Model(&User{}).Where("id = ?", owner.Id).Count(&stillThere).Error)
	assert.EqualValues(t, 1, stillThere)

	require.NoError(t, DB.Model(site).Update("status", AdtySiteStatusClosed).Error)
	_, err = owner.Delete()
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", owner.Id).Count(&stillThere).Error)
	assert.Zero(t, stillThere)
}
