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
				}
				for range 2 {
					require.NoError(t, migrateDB())
				}
				require.True(t, db.Migrator().HasTable("adty_notification_reads"))
				if upgrade {
					var historical User
					require.NoError(t, db.First(&historical, 101).Error)
					assert.Equal(t, 123456, historical.Quota)
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
