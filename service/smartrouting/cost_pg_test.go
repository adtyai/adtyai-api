package smartrouting

import (
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBumpDayCostUsesAtomicAddAndSkipsExcluded(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&model.ChannelCostGuard{}))
	prev := model.DB
	model.DB = db
	const channelID = 910000043
	day := time.Now().Format("2006-01-02")
	t.Cleanup(func() {
		_ = db.Where("channel_id = ?", channelID).Delete(&model.ChannelCostGuard{}).Error
		model.DB = prev
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Where("channel_id = ?", channelID).Delete(&model.ChannelCostGuard{}).Error)
	require.NoError(t, db.Create(&model.ChannelCostGuard{ChannelID: channelID, Day: day, CostCNY: 4, Excluded: true}).Error)

	cfg := DefaultConfig()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bumpDayCost(channelID, 1, cfg)
		}()
	}
	wg.Wait()
	var row model.ChannelCostGuard
	require.NoError(t, db.Where("channel_id = ? AND day = ?", channelID, day).First(&row).Error)
	if row.CostCNY < 35.9 || row.CostCNY > 36.1 || !row.Excluded {
		t.Fatalf("already excluded row cost=%v excluded=%v", row.CostCNY, row.Excluded)
	}
}
