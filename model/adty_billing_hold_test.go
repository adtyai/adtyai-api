package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withMemoryBillingHoldDB(t *testing.T, migrate bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	if migrate {
		require.NoError(t, db.AutoMigrate(&AdtyBillingHold{}))
	} else {
		require.False(t, db.Migrator().HasTable(&AdtyBillingHold{}))
	}
	prev := DB
	DB = db
	t.Cleanup(func() { DB = prev })
	return db
}

func TestRecordAdtyBillingHoldMissingTableErrors(t *testing.T) {
	withMemoryBillingHoldDB(t, false)
	err := RecordAdtyBillingHold(51, "hold-missing", 12)
	require.ErrorIs(t, err, ErrAdtyBillingHoldTableMissing)
}

// 差额记录必须还在（`ISS-R4-0084` 只删拦截副作用，不删数据）。
//
// 财务对账要的正是这张表：谁的请求没能被余额全额覆盖、差多少。
// 修前这条同时断言「用户被暂停」，那不是数据属性，是门禁的副作用——
// 门禁删了，这条只留数据断言。
func TestRecordAdtyBillingHoldKeepsTheRowWithoutBlocking(t *testing.T) {
	withMemoryBillingHoldDB(t, true)
	require.NoError(t, RecordAdtyBillingHold(52, "hold-1", 15))

	var row AdtyBillingHold
	require.NoError(t, DB.Where("user_id = ?", 52).First(&row).Error)
	require.EqualValues(t, 15, row.ShortfallQuota, "差额要记下来，财务对账要用")
	require.Equal(t, AdtyBillingHoldOpen, row.Status)

	// 同一个 request 重复记录不新增行（幂等），也不因为「已有开口 row」而报错。
	require.NoError(t, RecordAdtyBillingHold(52, "hold-1", 15))
	var count int64
	require.NoError(t, DB.Model(&AdtyBillingHold{}).Where("user_id = ?", 52).Count(&count).Error)
	require.EqualValues(t, 1, count)

	// 另一个用户没有任何 hold —— 只是数据事实，不再有「是否暂停」这个查询。
	var other int64
	require.NoError(t, DB.Model(&AdtyBillingHold{}).Where("user_id = ?", 53).Count(&other).Error)
	require.Zero(t, other)
}
