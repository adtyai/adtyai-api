package model

import (
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAddChannelDayCostConcurrentPostgres(t *testing.T) {
	if os.Getenv("ADTY_TEST_PG") != "1" {
		t.Skip("set ADTY_TEST_PG=1 and TEST_POSTGRES_DSN to run the postgres path")
	}
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	require.NotEmpty(t, dsn)
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	ip := net.ParseIP(parsed.Hostname())
	require.NotNil(t, ip)
	require.True(t, ip.IsLoopback(), "database tests only permit loopback instances")

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ChannelCostGuard{}))
	prev := DB
	DB = db
	const channelID = 910000042
	const day = "2099-01-02"
	t.Cleanup(func() {
		_ = db.Where("channel_id = ? AND day = ?", channelID, day).Delete(&ChannelCostGuard{}).Error
		DB = prev
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Where("channel_id = ? AND day = ?", channelID, day).Delete(&ChannelCostGuard{}).Error)

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if addErr := AddChannelDayCost(channelID, day, 1); addErr != nil {
				errs <- addErr
			}
		}()
	}
	wg.Wait()
	close(errs)
	for addErr := range errs {
		t.Error(addErr)
	}
	var row ChannelCostGuard
	require.NoError(t, db.Where("channel_id = ? AND day = ?", channelID, day).First(&row).Error)
	if row.CostCNY < 31.9 || row.CostCNY > 32.1 {
		t.Fatalf("concurrent adds lost updates, cost=%v", row.CostCNY)
	}
}
