package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 客户归属只由分站服务改（执行层决定 29）：用户改资料、管理员编辑用户都不能改 site_id。
func TestUserUpdateKeepsSiteId(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	user := User{Username: "site_customer", Password: "x", Group: "default", Status: 1, Role: 1, SiteId: 7}
	require.NoError(t, db.Create(&user).Error)

	update := User{Id: user.Id, DisplayName: "renamed", SiteId: 0}
	require.NoError(t, update.Update(false))
	update = User{Id: user.Id, DisplayName: "renamed again", SiteId: 9}
	require.NoError(t, update.Update(false))

	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, 7, stored.SiteId)
	assert.Equal(t, "renamed again", stored.DisplayName)
}
