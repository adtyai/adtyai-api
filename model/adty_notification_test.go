package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationReadDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade=%t", dialect, upgrade), func(t *testing.T) {
				db := isolatedMigrationTestDatabase(t, dialect)
				if upgrade {
					require.NoError(t, db.AutoMigrate(&User{}))
					require.NoError(t, db.Create(&User{Id: 101, Username: "receipt-history", Quota: 123456}).Error)
					require.NoError(t, db.AutoMigrate(&AdtyNotificationRead{}))
					require.NoError(t, MarkAdtyNotificationsRead(101, 0, []string{strings.Repeat("c", 64)}, 1000))
				}
				for range 2 {
					require.NoError(t, migrateDB())
				}
				require.True(t, db.Migrator().HasTable("adty_notification_reads"))
				require.True(t, db.Migrator().HasTable("adty_notification_publications"))
				publication := AdtyNotificationPublication{IssuerSiteId: 7, PublisherId: 101, PublisherName: "original-publisher", Audience: "site", TargetSiteId: 7,
					TargetSiteName: "shop.example.test", Kind: "rules", Title: "Original rules", Content: "Original content", Status: "published", Revision: 1,
					ReadKey: strings.Repeat("d", 64), PublishedAt: 100, UpdatedAt: 100, UpdatedBy: 101}
				require.NoError(t, db.Create(&publication).Error)
				duplicate := publication
				duplicate.Id = 0
				require.Error(t, db.Create(&duplicate).Error, "version keys must be unique across publications")
				for range 2 {
					require.NoError(t, migrateDB())
				}
				var groupEdits sync.WaitGroup
				edits := make(chan error, 2)
				for i := range 2 {
					candidate := publication
					candidate.Content, candidate.ReadKey = fmt.Sprintf("Edit %d", i), strings.Repeat(fmt.Sprint(i+1), 64)
					groupEdits.Go(func() { edits <- UpdateAdtyNotificationPublication(&candidate, 1) })
				}
				groupEdits.Wait()
				close(edits)
				conflicts, successes := 0, 0
				for err := range edits {
					if err == nil {
						successes++
					} else {
						require.ErrorIs(t, err, ErrAdtyNotificationConflict)
						conflicts++
					}
				}
				assert.Equal(t, 1, successes)
				assert.Equal(t, 1, conflicts)
				require.NoError(t, db.First(&publication, publication.Id).Error)
				assert.Equal(t, "original-publisher", publication.PublisherName)
				assert.Equal(t, int64(100), publication.PublishedAt)
				publication.Status = "withdrawn"
				require.NoError(t, UpdateAdtyNotificationPublication(&publication, 2))
				publication.Status = "published"
				require.ErrorIs(t, UpdateAdtyNotificationPublication(&publication, 3), ErrAdtyNotificationConflict, "a withdrawn publication cannot be restored by an editor")
				if upgrade {
					var historical User
					require.NoError(t, db.First(&historical, 101).Error)
					assert.Equal(t, 123456, historical.Quota)
					oldKeys, err := GetAdtyNotificationReadKeys(101, 0, []string{strings.Repeat("c", 64)})
					require.NoError(t, err)
					assert.Equal(t, []string{strings.Repeat("c", 64)}, oldKeys)
					require.NoError(t, db.Where("read_key = ?", strings.Repeat("c", 64)).Delete(&AdtyNotificationRead{}).Error)
				}
				key := strings.Repeat("a", 64)
				later := strings.Repeat("b", 64)
				require.NoError(t, MarkAdtyNotificationsRead(101, 0, []string{key}, 1234))
				require.NoError(t, MarkAdtyNotificationsRead(101, 0, []string{key}, 5678))
				var stored AdtyNotificationRead
				require.NoError(t, db.Where("user_id = ? AND site_id = ?", 101, 0).First(&stored).Error)
				assert.Equal(t, int64(1234), stored.ReadAt, "repeat requests preserve the first read time")
				for _, scope := range [][2]int{{101, 1}, {202, 0}} {
					keys, err := GetAdtyNotificationReadKeys(scope[0], scope[1], []string{key})
					require.NoError(t, err)
					assert.Empty(t, keys, "another user/site cannot inherit a receipt")
				}
				var group sync.WaitGroup
				failures := make(chan error, 8)
				for range 8 {
					group.Go(func() { failures <- MarkAdtyNotificationsRead(101, 0, []string{later}, 9999) })
				}
				group.Wait()
				close(failures)
				for err := range failures {
					require.NoError(t, err)
				}
				keys, err := GetAdtyNotificationReadKeys(101, 0, []string{key, later})
				require.NoError(t, err)
				assert.ElementsMatch(t, []string{key, later}, keys)
				var count int64
				require.NoError(t, db.Model(&AdtyNotificationRead{}).Where("user_id = ? AND site_id = ?", 101, 0).Count(&count).Error)
				assert.Equal(t, int64(2), count)
				require.NoError(t, migrateDB())
				keys, err = GetAdtyNotificationReadKeys(101, 0, []string{key, later})
				require.NoError(t, err)
				assert.Len(t, keys, 2, "startup must retain durable read state")
			})
		}
	}
}

func TestStaffDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade=%t", dialect, upgrade), func(t *testing.T) {
				db := isolatedMigrationTestDatabase(t, dialect)
				if upgrade {
					require.NoError(t, db.AutoMigrate(&User{}))
					require.NoError(t, db.Create(&User{Id: 101, Username: "staff-history", Quota: 654321, AuthVersion: 7}).Error)
					// Exact previous User schema, before the dedicated identity column.
					require.NoError(t, db.Migrator().DropColumn(&User{}, "staff_site_id"))
				}
				for range 2 {
					require.NoError(t, migrateDB())
				}
				require.True(t, db.Migrator().HasTable("adty_site_staff"))
				if upgrade {
					var historical User
					require.NoError(t, db.First(&historical, 101).Error)
					assert.Equal(t, 654321, historical.Quota)
					assert.EqualValues(t, 7, historical.AuthVersion)
					assert.Zero(t, historical.StaffSiteId)
				}
				user := User{Username: "dedicated-staff", SiteId: 9, StaffSiteId: 9}
				require.NoError(t, db.Create(&user).Error)
				staff := AdtySiteStaff{SiteId: 9, UserId: user.Id, OwnerUserId: 2, Role: "support", Status: "enabled", Permissions: `["customers.read"]`, Version: 1}
				require.NoError(t, db.Create(&staff).Error)
				for range 2 {
					require.NoError(t, migrateDB())
				}
				var stored AdtySiteStaff
				require.NoError(t, db.First(&stored, staff.Id).Error)
				assert.Equal(t, `["customers.read"]`, stored.Permissions)
				assert.Equal(t, 2, stored.OwnerUserId)
				require.NoError(t, db.Model(&stored).Update("status", "deleted").Error)
				duplicate := staff
				duplicate.Id, duplicate.SiteId = 0, 99
				assert.Error(t, db.Create(&duplicate).Error, "deleted staff identity cannot be rebound to another site")
				require.NoError(t, db.First(&user, user.Id).Error)
				assert.Equal(t, 9, user.ToBaseUser().StaffSiteId)
			})
		}
	}
}
