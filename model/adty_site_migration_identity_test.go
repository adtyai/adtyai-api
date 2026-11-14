package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdtySetUsersSiteTxRecordsMappingOnce(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &AdtySiteCustomer{}, &AdtyUserSiteMigration{}))
	user := User{Username: "inherit-map", Password: "x", Group: "default", Status: common.UserStatusEnabled, SiteId: 7, Quota: 400}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&AdtySiteCustomer{UserId: user.Id, SiteId: 7, JoinedAt: 10}).Error)

	var moved []int
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		moved, err = AdtySetUsersSiteTx(tx, 7, 0)
		if err != nil {
			return err
		}
		return ArchiveAdtySiteCustomersOnCloseTx(tx, moved, 7, 100)
	})
	require.NoError(t, err)
	assert.Equal(t, []int{user.Id}, moved)

	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, 7, stored.InheritedFromSiteId)
	assert.Equal(t, 400, stored.Quota)
	assert.Equal(t, user.Id, stored.Id)

	var first AdtyUserSiteMigration
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&first).Error)
	assert.Equal(t, 7, first.SourceSiteId)
	assert.Zero(t, first.TargetSiteId)
	assert.NotZero(t, first.Id)

	var membership AdtySiteCustomer
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&membership).Error)
	assert.Equal(t, 7, membership.SiteId)
	assert.Equal(t, AdtySiteCustomerArchived, membership.Status)
	assert.Equal(t, int64(100), membership.ArchivedAt)

	err = db.Transaction(func(tx *gorm.DB) error {
		if _, err := AdtySetUsersSiteTx(tx, 7, 0); err != nil {
			return err
		}
		if err := RecordAdtyUserSiteMigrationsTx(tx, []int{user.Id}, 7, 0, 200); err != nil {
			return err
		}
		return ArchiveAdtySiteCustomersOnCloseTx(tx, []int{user.Id}, 7, 200)
	})
	require.NoError(t, err)

	var mappings []AdtyUserSiteMigration
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&mappings).Error)
	require.Len(t, mappings, 1)
	assert.Equal(t, first.Id, mappings[0].Id)
	assert.Equal(t, first.CreatedAt, mappings[0].CreatedAt)
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&membership).Error)
	assert.Equal(t, AdtySiteCustomerArchived, membership.Status)
	assert.Equal(t, int64(100), membership.ArchivedAt, "a retry must not rewrite the original archive timestamp")
	assert.Equal(t, 7, membership.SiteId)
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, 7, stored.InheritedFromSiteId)
}

func TestAdtySetUsersSiteTxDoesNotMapTransferToAnotherSite(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &AdtyUserSiteMigration{}))
	user := User{Username: "no-other-site-map", Password: "x", Group: "default", Status: common.UserStatusEnabled, SiteId: 7}
	require.NoError(t, db.Create(&user).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := AdtySetUsersSiteTx(tx, 7, 9)
		return err
	})
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&AdtyUserSiteMigration{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count, "inherit-main-only must not write a mapping onto another subsite")
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, 9, stored.SiteId)
	assert.Zero(t, stored.InheritedFromSiteId)
}
