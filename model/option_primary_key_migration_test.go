package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// legacyOptionsDB 建出问题发生时的表形态：key 没有主键，同一键能有多行。
func legacyOptionsDB(t *testing.T, rows []Option) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Exec("CREATE TABLE options (key text NOT NULL, value text NOT NULL)").Error)
	for _, row := range rows {
		require.NoError(t, db.Exec("INSERT INTO options (key, value) VALUES (?, ?)", row.Key, row.Value).Error)
	}
	return db
}

func captureSysErrors(t *testing.T) *[]string {
	t.Helper()
	messages := make([]string, 0)
	restore := common.SetSysLogSink(func(level, message string) {
		if level == "ERROR" {
			messages = append(messages, message)
		}
	})
	t.Cleanup(restore)
	return &messages
}

func optionsRowCount(t *testing.T, db *gorm.DB, key string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table("options").Where("key = ?", key).Count(&n).Error)
	return n
}

// 同一份数据换个物理顺序也必须选出同一行：不排序时 PostgreSQL 返回的是物理
// 顺序，重建结果会随环境、随 VACUUM FULL 变化（ISS-R4-0113）。
func TestRepairOptionPrimaryKeyPicksTheSameRowWhateverThePhysicalOrder(t *testing.T) {
	first, second := Option{Key: "Alpha", Value: "a"}, Option{Key: "Alpha", Value: "b"}
	kept := make([]string, 0, 2)
	for _, rows := range [][]Option{{first, second}, {second, first}} {
		captureSysErrors(t)
		db := legacyOptionsDB(t, rows)
		require.NoError(t, repairOptionPrimaryKey(db))
		require.EqualValues(t, 1, optionsRowCount(t, db, "Alpha"))
		kept = append(kept, requireOptionValue(t, db, "Alpha"))
	}
	require.Equal(t, []string{"b", "b"}, kept, "行序不同但保留的值必须相同")
}

// 冲突日志必须带值：只有键名时，事后没人能回答「最后选中的是哪个值」。
func TestRepairOptionPrimaryKeyLogsKeptAndDroppedValues(t *testing.T) {
	messages := captureSysErrors(t)
	db := legacyOptionsDB(t, []Option{{Key: "Alpha", Value: "a"}, {Key: "Alpha", Value: "b"}})
	require.NoError(t, repairOptionPrimaryKey(db))
	joined := strings.Join(*messages, "\n")
	require.Contains(t, joined, `options key "Alpha" had conflicting values: kept="b" dropped="a"`)
	require.Contains(t, joined, "kept one value for 1 conflicting keys: Alpha")
}

// 约定 C 说 QuotaPerUnit 只有一个合法值，重建不该靠行序猜：两行都非法时也要
// 落到 500000，并且把换掉的值留下（ISS-R4-0113）。
func TestRepairOptionPrimaryKeyKeepsOnlyLegalQuotaPerUnit(t *testing.T) {
	messages := captureSysErrors(t)
	db := legacyOptionsDB(t, []Option{
		{Key: optionQuotaPerUnitKey, Value: "400000"},
		{Key: optionQuotaPerUnitKey, Value: "400000"},
		{Key: "Other", Value: "kept"},
		{Key: "", Value: "dropped"},
	})
	require.NoError(t, repairOptionPrimaryKey(db))
	require.Equal(t, "500000", requireOptionValue(t, db, optionQuotaPerUnitKey))
	require.Equal(t, "kept", requireOptionValue(t, db, "Other"))
	require.EqualValues(t, 0, optionsRowCount(t, db, ""), "空键的行不落库")
	unique, err := optionsKeyIsUnique(db)
	require.NoError(t, err)
	require.True(t, unique)
	joined := strings.Join(*messages, "\n")
	require.Contains(t, joined, `options key "QuotaPerUnit" rebuilt with the only legal value: dropped="400000" kept="500000"`)
	require.Contains(t, joined, "skipped 1 options rows with empty keys")
}
