package model

import (
	"testing"

	"github.com/QuantumNous/new-api/internal/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 没有分摊的那条结算路径（升级前留下的 operation，只有预扣没有 allocation 行）
// 必须报出真实的 backed，否则 `fund_batch_gap` 在最需要观测漂移的账户上恒为 0。
//
// 这条分支原来直接 `return applied`，于是 gap 结构性等于 0 —— 246f168 加那一列
// 的全部意义，被这条 return 抵消掉。
func TestNoShareSettlementReportsBackedCoveragePG(t *testing.T) {
	db := adtyTestDBWithSchema(t)
	pgtest.RecordServerVersion(t, db)

	require.NoError(t, db.Create(&User{Id: 901, Username: "noshare-901", Password: "password1", Quota: 0}).Error)
	// 批次只有 20，余额 50：这就是「钱在账上、批次不全」的漂移形状。
	require.NoError(t, db.Create(&AdtyFundBatch{
		UserId: 901, Kind: "principal", Source: "thin", Original: 20, Remaining: 20,
	}).Error)

	var backed int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		backed, err = SettleAdtyFundAllocationsTx(tx, 901, "op-noshare-1", 50, 50)
		return err
	}))
	assert.EqualValues(t, 20, backed, "无分摊路径也要报补建前的真实覆盖，不能 return applied")

	gap := int64(50) - backed
	assert.EqualValues(t, 30, gap, "这就是应当落进 fund_batch_gap 的漂移额")

	var lots []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ? AND remaining > 0", 901).Find(&lots).Error)
	sum := int64(0)
	for _, lot := range lots {
		sum += lot.Remaining
	}
	assert.Zero(t, sum, "50 全扣完；补建的 30 也正好用掉")
}

// `balanceBefore` 是调用方给的，函数内部没有任何东西能验证它——这是「凭空造钱」
// 的唯一入口，而目前只有调用方的自觉在守。
//
// 这条用例钉的是**不可防、但必须可见**：高报 balanceBefore 会补出额度，
// 而 backed 不含那部分，于是差额一定出现在 gap 里。
//
// 为什么不做成「高报就报错」：这个状态和「余额确实是 50、被本次操作扣到 0」
// 在库里完全一样（quota=0、没有批次），函数这一层不可能区分。能区分的只有
// 调用方自己的事务语义，而那正是它需要遵守的东西。
func TestOverstatedBalanceIsVisibleAsGapNotPreventedPG(t *testing.T) {
	db := adtyTestDBWithSchema(t)
	pgtest.RecordServerVersion(t, db)

	// 库里余额是 0，调用方却声称消费前有 50 —— 这就是被高报的输入。
	require.NoError(t, db.Create(&User{Id: 902, Username: "overstated-902", Password: "password1", Quota: 0}).Error)

	var backed int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		backed, err = SettleAdtyFundAllocationsTx(tx, 902, "op-overstated-1", 50, 50)
		return err
	}))
	assert.Zero(t, backed, "一个真实批次都没有，backed 必须是 0")
	assert.EqualValues(t, 50, int64(50)-backed, "高报出来的额度全部进 gap，一条都不许无声无息")

	var invented AdtyFundBatch
	require.NoError(t, db.Where("user_id = ? AND source = ?", 902, "legacy").First(&invented).Error)
	assert.EqualValues(t, 50, invented.Original, "补建出来的额度就是高报出来的额度")
	assert.Zero(t, invented.Remaining, "本次已全部扣走")
}
