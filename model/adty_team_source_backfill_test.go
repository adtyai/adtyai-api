package model

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func loadAdtyTeam(t *testing.T, db *gorm.DB, id int64) AdtyTeam {
	t.Helper()
	var row AdtyTeam
	require.NoError(t, db.Where("id = ?", id).Take(&row).Error)
	return row
}

func TestBackfillAdtyTeamSourceSiteID(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyTeam{}, &AdtyTeamMember{}, &AdtyTeamRegistrationLink{}, &AdtyUserSiteMigration{}))

	known := AdtyTeam{SiteId: 0, LeaderUserId: 1, Name: "known-historical", Discount: decimal.NewFromInt(1), Status: AdtyTeamStatusActive}
	require.NoError(t, db.Create(&known).Error)
	require.NoError(t, db.Create(&AdtyTeamRegistrationLink{
		SiteId: 7, CodeHash: "hash-known", CreatedBy: 1, UsedBy: 1, UsedAt: time.Now().Unix(), TeamId: known.Id,
	}).Error)

	alreadySet := AdtyTeam{SiteId: 0, SourceSiteId: 11, LeaderUserId: 2, Name: "already-set", Discount: decimal.NewFromInt(1), Status: AdtyTeamStatusActive}
	require.NoError(t, db.Create(&alreadySet).Error)
	require.NoError(t, db.Create(&AdtyTeamRegistrationLink{
		SiteId: 7, CodeHash: "hash-already", CreatedBy: 2, UsedBy: 2, UsedAt: time.Now().Unix(), TeamId: alreadySet.Id,
	}).Error)

	nativeMain := AdtyTeam{SiteId: 0, LeaderUserId: 3, Name: "native-main", Discount: decimal.NewFromInt(1), Status: AdtyTeamStatusActive}
	require.NoError(t, db.Create(&nativeMain).Error)
	require.NoError(t, db.Create(&AdtyTeamRegistrationLink{
		SiteId: 0, CodeHash: "hash-native-main", CreatedBy: 3, UsedBy: 3, UsedAt: time.Now().Unix(), TeamId: nativeMain.Id,
	}).Error)

	// A native main-site team whose leader was later inherited must not be guessed
	// from the user row or from a user migration mapping.
	inheritedLeader := User{Username: "legacy-main-leader", Password: "x", Group: "default", Status: 1, SiteId: 0, InheritedFromSiteId: 7}
	require.NoError(t, db.Create(&inheritedLeader).Error)
	legacyMain := AdtyTeam{SiteId: 0, LeaderUserId: inheritedLeader.Id, Name: "legacy-main", Discount: decimal.NewFromInt(1), Status: AdtyTeamStatusActive}
	require.NoError(t, db.Create(&legacyMain).Error)
	require.NoError(t, RecordAdtyUserSiteMigrationsTx(db, []int{inheritedLeader.Id}, 7, 0, 100))

	stillOnSite := AdtyTeam{SiteId: 9, LeaderUserId: 4, Name: "still-on-site", Discount: decimal.NewFromInt(1), Status: AdtyTeamStatusActive}
	require.NoError(t, db.Create(&stillOnSite).Error)
	require.NoError(t, db.Create(&AdtyTeamRegistrationLink{
		SiteId: 9, CodeHash: "hash-still", CreatedBy: 4, UsedBy: 4, UsedAt: time.Now().Unix(), TeamId: stillOnSite.Id,
	}).Error)

	ambiguous := AdtyTeam{SiteId: 0, LeaderUserId: 5, Name: "ambiguous", Discount: decimal.NewFromInt(1), Status: AdtyTeamStatusActive}
	require.NoError(t, db.Create(&ambiguous).Error)
	require.NoError(t, db.Create(&AdtyTeamRegistrationLink{
		SiteId: 7, CodeHash: "hash-amb-a", CreatedBy: 5, UsedBy: 5, UsedAt: time.Now().Unix(), TeamId: ambiguous.Id,
	}).Error)
	require.NoError(t, db.Create(&AdtyTeamRegistrationLink{
		SiteId: 11, CodeHash: "hash-amb-b", CreatedBy: 5, UsedBy: 5, UsedAt: time.Now().Unix(), TeamId: ambiguous.Id,
	}).Error)

	unusedLinkTeam := AdtyTeam{SiteId: 0, LeaderUserId: 6, Name: "unused-link", Discount: decimal.NewFromInt(1), Status: AdtyTeamStatusActive}
	require.NoError(t, db.Create(&unusedLinkTeam).Error)
	require.NoError(t, db.Create(&AdtyTeamRegistrationLink{
		SiteId: 7, CodeHash: "hash-unused", CreatedBy: 6,
	}).Error)

	require.NoError(t, BackfillAdtyTeamSourceSiteID(db))

	stored := loadAdtyTeam(t, db, known.Id)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, 7, stored.SourceSiteId)

	stored = loadAdtyTeam(t, db, alreadySet.Id)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, 11, stored.SourceSiteId, "an already recorded source must not be overwritten")

	stored = loadAdtyTeam(t, db, nativeMain.Id)
	assert.Zero(t, stored.SourceSiteId, "a native main-site team must keep source 0")

	stored = loadAdtyTeam(t, db, legacyMain.Id)
	assert.Zero(t, stored.SourceSiteId, "must not infer source from the leader's inherited_from_site_id or user mapping")

	stored = loadAdtyTeam(t, db, stillOnSite.Id)
	assert.Equal(t, 9, stored.SiteId)
	assert.Zero(t, stored.SourceSiteId, "must not backfill a team that is still on a subsite")

	stored = loadAdtyTeam(t, db, ambiguous.Id)
	assert.Zero(t, stored.SourceSiteId, "must not guess when recorded sources disagree")

	stored = loadAdtyTeam(t, db, unusedLinkTeam.Id)
	assert.Zero(t, stored.SourceSiteId, "an unused registration link does not record a team origin")

	require.NoError(t, BackfillAdtyTeamSourceSiteID(db))
	stored = loadAdtyTeam(t, db, known.Id)
	assert.Equal(t, 7, stored.SourceSiteId)
	stored = loadAdtyTeam(t, db, alreadySet.Id)
	assert.Equal(t, 11, stored.SourceSiteId)
}

func TestMigrateAdtySiteTablesBackfillsKnownTeamSources(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyTeam{}, &AdtyTeamRegistrationLink{}))
	require.NoError(t, db.AutoMigrate(AdtySiteTables()...))

	historical := AdtyTeam{SiteId: 0, LeaderUserId: 21, Name: "migrate-known", Discount: decimal.NewFromInt(1), Status: AdtyTeamStatusActive}
	require.NoError(t, db.Create(&historical).Error)
	require.NoError(t, db.Create(&AdtyTeamRegistrationLink{
		SiteId: 5, CodeHash: "hash-migrate-known", CreatedBy: 21, UsedBy: 21, UsedAt: time.Now().Unix(), TeamId: historical.Id,
	}).Error)
	nativeMain := AdtyTeam{SiteId: 0, LeaderUserId: 22, Name: "migrate-native", Discount: decimal.NewFromInt(1), Status: AdtyTeamStatusActive}
	require.NoError(t, db.Create(&nativeMain).Error)

	require.NoError(t, migrateAdtySiteTables())

	stored := loadAdtyTeam(t, db, historical.Id)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, 5, stored.SourceSiteId)

	stored = loadAdtyTeam(t, db, nativeMain.Id)
	assert.Zero(t, stored.SourceSiteId)
}
