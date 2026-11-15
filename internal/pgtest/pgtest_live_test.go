package pgtest_test

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/internal/pgtest"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 在真库上验一遍：主版本判据放行 17.x，补丁号只记录。
func TestRecordServerVersionOnLivePostgres(t *testing.T) {
	dsn := os.Getenv("ADTY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("ADTY_TEST_PG_DSN 未设置")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, closeErr := db.DB(); closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	versionNum := pgtest.RecordServerVersion(t, db)
	if versionNum == "" {
		t.Fatal("版本号必须被记录")
	}
	// 17.12 / 17.13 都应当到这里；16 或 18 会在 RecordServerVersion 里 Fatal。
	t.Logf("live server_version_num=%s (major check passed)", versionNum)
}
