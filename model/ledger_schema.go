package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	ledgerImmutableTrigger = "trg_ledger_entries_immutable"
	ledgerHourlyUniqueIdx  = "ux_ledger_hourly_rollup"
)

// EnsureLedgerSchema 幂等：分区表 + 禁改删触发器 + 小时汇总唯一键 + ledger_idempotency。
// 仅 PostgreSQL；其他库跳过分区/触发器。
func EnsureLedgerSchema(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if err := db.AutoMigrate(&LedgerIdempotency{}); err != nil {
		return fmt.Errorf("ledger_idempotency: %w", err)
	}
	if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		// 非 PG：仅保证普通表存在（验收以 WSL+PG 为准）
		return db.AutoMigrate(&LedgerEntry{})
	}
	if err := ensureLedgerEntriesPartitioned(db); err != nil {
		return err
	}
	if err := ensureLedgerImmutableTrigger(db); err != nil {
		return err
	}
	if err := ensureLedgerHourlyUniqueIndex(db); err != nil {
		return err
	}
	return nil
}

func ensureLedgerEntriesPartitioned(db *gorm.DB) error {
	var partRel int
	if err := db.Raw(`SELECT COUNT(*) FROM pg_partitioned_table p
		JOIN pg_class c ON c.oid = p.partrelid
		WHERE c.relname = 'ledger_entries'`).Scan(&partRel).Error; err != nil {
		return err
	}
	if partRel > 0 {
		return ensureLedgerPartitionsCover(db, time.Now().UTC())
	}

	hasTable := db.Migrator().HasTable("ledger_entries")
	if !hasTable {
		if err := createEmptyPartitionedLedger(db); err != nil {
			return err
		}
		return ensureLedgerPartitionsCover(db, time.Now().UTC())
	}

	// 已有普通表 → 迁到分区表（可重复：若已是分区则上面已返回）
	return convertPlainLedgerToPartitioned(db)
}

func createEmptyPartitionedLedger(db *gorm.DB) error {
	sql := `
CREATE TABLE IF NOT EXISTS ledger_entries (
  id BIGSERIAL NOT NULL,
  user_id BIGINT NOT NULL,
  account VARCHAR(16) NOT NULL,
  kind VARCHAR(64) NOT NULL,
  delta BIGINT NOT NULL,
  balance_after BIGINT NOT NULL,
  reason TEXT,
  ref_type VARCHAR(64),
  ref_id VARCHAR(128),
  hour_bucket TIMESTAMPTZ,
  operator_id BIGINT DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_user_id ON ledger_entries (user_id);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_hour_bucket ON ledger_entries (hour_bucket);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_created_at ON ledger_entries (created_at);
`
	return db.Exec(sql).Error
}

func convertPlainLedgerToPartitioned(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// 再次确认非分区（并发保护）
		var partRel int
		if err := tx.Raw(`SELECT COUNT(*) FROM pg_partitioned_table p
			JOIN pg_class c ON c.oid = p.partrelid WHERE c.relname = 'ledger_entries'`).Scan(&partRel).Error; err != nil {
			return err
		}
		if partRel > 0 {
			return nil
		}

		if err := tx.Exec(`ALTER TABLE ledger_entries RENAME TO ledger_entries_legacy`).Error; err != nil {
			return fmt.Errorf("rename legacy: %w", err)
		}
		// 旧索引/约束随表改名；新建分区父表
		if err := tx.Exec(`
CREATE TABLE ledger_entries (
  id BIGSERIAL NOT NULL,
  user_id BIGINT NOT NULL,
  account VARCHAR(16) NOT NULL,
  kind VARCHAR(64) NOT NULL,
  delta BIGINT NOT NULL,
  balance_after BIGINT NOT NULL,
  reason TEXT,
  ref_type VARCHAR(64),
  ref_id VARCHAR(128),
  hour_bucket TIMESTAMPTZ,
  operator_id BIGINT DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at)`).Error; err != nil {
			return fmt.Errorf("create partitioned: %w", err)
		}

		var minAt, maxAt *time.Time
		_ = tx.Raw(`SELECT MIN(created_at), MAX(created_at) FROM ledger_entries_legacy`).Row().Scan(&minAt, &maxAt)
		now := time.Now().UTC()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
		to := from.AddDate(0, 3, 0)
		if minAt != nil {
			from = time.Date(minAt.UTC().Year(), minAt.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
		}
		if maxAt != nil {
			end := time.Date(maxAt.UTC().Year(), maxAt.UTC().Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 2, 0)
			if end.After(to) {
				to = end
			}
		}
		for m := from; m.Before(to); m = m.AddDate(0, 1, 0) {
			if err := createMonthPartitionTx(tx, m); err != nil {
				return err
			}
		}

		if err := tx.Exec(`
INSERT INTO ledger_entries (id, user_id, account, kind, delta, balance_after, reason, ref_type, ref_id, hour_bucket, operator_id, created_at)
SELECT id, user_id, account, kind, delta, balance_after, reason, ref_type, ref_id, hour_bucket, operator_id, created_at
FROM ledger_entries_legacy`).Error; err != nil {
			return fmt.Errorf("copy data: %w", err)
		}

		if err := tx.Exec(`
SELECT setval(pg_get_serial_sequence('ledger_entries', 'id'),
  COALESCE((SELECT MAX(id) FROM ledger_entries), 1), true)`).Error; err != nil {
			return fmt.Errorf("setval: %w", err)
		}

		if err := tx.Exec(`DROP TABLE ledger_entries_legacy`).Error; err != nil {
			return fmt.Errorf("drop legacy: %w", err)
		}

		_ = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_ledger_entries_user_id ON ledger_entries (user_id)`)
		_ = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_ledger_entries_hour_bucket ON ledger_entries (hour_bucket)`)
		_ = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_ledger_entries_created_at ON ledger_entries (created_at)`)
		return nil
	})
}

func monthPartitionName(monthStart time.Time) string {
	m := monthStart.UTC()
	return fmt.Sprintf("ledger_entries_y%04dm%02d", m.Year(), int(m.Month()))
}

func createMonthPartitionTx(tx *gorm.DB, monthStart time.Time) error {
	m := time.Date(monthStart.UTC().Year(), monthStart.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	name := monthPartitionName(m)
	next := m.AddDate(0, 1, 0)
	// IF NOT EXISTS for partitions: check pg_class
	var exists int
	if err := tx.Raw(`SELECT COUNT(*) FROM pg_class WHERE relname = ?`, name).Scan(&exists).Error; err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	sql := fmt.Sprintf(
		`CREATE TABLE %s PARTITION OF ledger_entries FOR VALUES FROM ('%s') TO ('%s')`,
		name,
		m.Format("2006-01-02"),
		next.Format("2006-01-02"),
	)
	return tx.Exec(sql).Error
}

// EnsureLedgerMonthPartition 确保某月分区存在（导出给预建任务）。
func EnsureLedgerMonthPartition(db *gorm.DB, monthStart time.Time) error {
	if db == nil {
		db = DB
	}
	return createMonthPartitionTx(db, monthStart)
}

func ensureLedgerPartitionsCover(db *gorm.DB, now time.Time) error {
	cur := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	// 当前月 + 下月（预建；距月末 ≤7 天也会再跑 partition_precreate）
	for _, m := range []time.Time{cur.AddDate(0, -1, 0), cur, cur.AddDate(0, 1, 0)} {
		if err := createMonthPartitionTx(db, m); err != nil {
			return err
		}
	}
	return nil
}

func ensureLedgerImmutableTrigger(db *gorm.DB) error {
	if err := db.Exec(`
CREATE OR REPLACE FUNCTION ledger_entries_immutable() RETURNS trigger AS $fn$
BEGIN
  RAISE EXCEPTION 'ledger_entries is append-only: % not allowed', TG_OP;
END;
$fn$ LANGUAGE plpgsql`).Error; err != nil {
		return err
	}
	var n int
	if err := db.Raw(`SELECT COUNT(*) FROM pg_trigger WHERE tgname = ? AND NOT tgisinternal`, ledgerImmutableTrigger).Scan(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		if err := db.Exec(fmt.Sprintf(`
CREATE TRIGGER %s
  BEFORE UPDATE OR DELETE ON ledger_entries
  FOR EACH ROW EXECUTE FUNCTION ledger_entries_immutable()`, ledgerImmutableTrigger)).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureLedgerHourlyUniqueIndex(db *gorm.DB) error {
	// 仅对 hour_bucket IS NOT NULL 的汇总行强制唯一
	return db.Exec(fmt.Sprintf(`
CREATE UNIQUE INDEX IF NOT EXISTS %s
  ON ledger_entries (user_id, account, kind, hour_bucket, created_at)
  WHERE hour_bucket IS NOT NULL`, ledgerHourlyUniqueIdx)).Error
}

// DisableLedgerImmutableTrigger 验收用：临时关闭禁删触发器以便删除汇总行测补跑。
func DisableLedgerImmutableTrigger(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	return db.Exec(fmt.Sprintf(`ALTER TABLE ledger_entries DISABLE TRIGGER %s`, ledgerImmutableTrigger)).Error
}

// EnableLedgerImmutableTrigger 验收用：重新启用。
func EnableLedgerImmutableTrigger(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	return db.Exec(fmt.Sprintf(`ALTER TABLE ledger_entries ENABLE TRIGGER %s`, ledgerImmutableTrigger)).Error
}

// IsLedgerPartitioned 验收查询。
func IsLedgerPartitioned(db *gorm.DB) (bool, error) {
	if db == nil {
		db = DB
	}
	var n int
	err := db.Raw(`SELECT COUNT(*) FROM pg_partitioned_table p
		JOIN pg_class c ON c.oid = p.partrelid WHERE c.relname = 'ledger_entries'`).Scan(&n).Error
	return n > 0, err
}

// LedgerSchemaOK 快速自检。
func LedgerSchemaOK(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	ok, err := IsLedgerPartitioned(db)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("ledger_entries not partitioned")
	}
	if !db.Migrator().HasTable(&LedgerIdempotency{}) {
		return fmt.Errorf("ledger_idempotency missing")
	}
	var trig int
	if err := db.Raw(`SELECT COUNT(*) FROM pg_trigger WHERE tgname = ? AND NOT tgisinternal`, ledgerImmutableTrigger).Scan(&trig).Error; err != nil {
		return err
	}
	if trig == 0 {
		return fmt.Errorf("immutable trigger missing")
	}
	return nil
}

// TruncateToHourUTC 整点（UTC）。
func TruncateToHourUTC(t time.Time) time.Time {
	return t.UTC().Truncate(time.Hour)
}

// HourBucketRunKey 格式化为 ops run_key。
func HourBucketRunKey(hb time.Time) string {
	return TruncateToHourUTC(hb).Format(time.RFC3339)
}

// ParseHourBucketRunKey 解析 run_key。
func ParseHourBucketRunKey(key string) (time.Time, error) {
	key = strings.TrimSpace(key)
	t, err := time.Parse(time.RFC3339, key)
	if err != nil {
		return time.Time{}, err
	}
	return TruncateToHourUTC(t), nil
}
