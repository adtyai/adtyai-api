package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ISS-R4-0015：人工退款只能退「收到过的现金」。
//
// 团队充值单和兑换码订单是额度入账：钱从来没进平台账户，退现金等于凭空付出，
// 而额度本身还能继续花。修前 adtyEnsureCreditedOrder 只看 status，这类单会被
// 接受，shortfall 全额登记成线下打款。
func adtyCashRefundFixture(t *testing.T) (*gorm.DB, User) {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &AdtyManualRefund{}))
	require.NoError(t, MigrateAdtyTables())
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	root := User{Username: "cash-refund-root", Password: "x", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&root).Error)
	return db, root
}

func seedAdtyCashRefundOrder(t *testing.T, db *gorm.DB, root User, tradeNo, provider string, teamID int64, payAmount decimal.Decimal) {
	t.Helper()
	require.NoError(t, db.Create(&TopUp{
		UserId: root.Id, Amount: 100, TradeNo: tradeNo,
		PaymentMethod: "usdt", PaymentProvider: provider, Status: common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, db.Create(&AdtyTopupOrder{
		TradeNo: tradeNo, UserId: root.Id, Provider: provider, Method: "usdt",
		FaceUSD: decimal.NewFromInt(100), CreditQuota: 50_000_000, TeamId: teamID,
		PayAmount: payAmount, PayCurrency: "USDT",
	}).Error)
}

// 团队充值单：没有任何现金进来，必须拒绝，且不是静默记成 shortfall。
func TestAdtyTeamTopUpRefundIsRejectedPG(t *testing.T) {
	db, root := adtyCashRefundFixture(t)
	seedAdtyCashRefundOrder(t, db, root, "TEAM-REFUND-1", "adty_team", 42, decimal.Zero)

	_, err := ApplyAdtyTopUpManualRefund("TEAM-REFUND-1", "probe", root.Id, "team-refund-1", decimal.NewFromInt(100))
	require.Error(t, err, "团队充值单没有收到过现金，不能退现金")
	assert.Contains(t, err.Error(), "cannot be refunded in cash")

	var rows int64
	require.NoError(t, db.Model(&AdtyManualRefund{}).Count(&rows).Error)
	assert.Zero(t, rows, "被拒绝时不能留下退款申请记录")
}

// 兑换码订单：同样是额度入账，钱来自已冻结的额度，不是新收到的现金。
func TestAdtyVoucherOrderRefundIsRejectedPG(t *testing.T) {
	db, root := adtyCashRefundFixture(t)
	seedAdtyCashRefundOrder(t, db, root, "VOUCHER-REFUND-1", AdtyVoucherProvider, 0, decimal.Zero)

	_, err := ApplyAdtyTopUpManualRefund("VOUCHER-REFUND-1", "probe", root.Id, "voucher-refund-1", decimal.NewFromInt(100))
	require.Error(t, err, "兑换码订单不能退现金")
	assert.Contains(t, err.Error(), "cannot be refunded in cash")
}

// 真实现金充值单必须照旧能申请退款——修复不能连带挡住正常业务。
func TestAdtyCashTopUpRefundStillWorksPG(t *testing.T) {
	db, root := adtyCashRefundFixture(t)
	seedAdtyCashRefundOrder(t, db, root, "CASH-REFUND-1", PaymentProviderAdtyBEpusdt, 0, decimal.NewFromInt(100))

	result, err := ApplyAdtyTopUpManualRefund("CASH-REFUND-1", "customer asked", root.Id, "cash-refund-1", decimal.NewFromInt(100))
	require.NoError(t, err, "真实现金充值单必须仍可退款")
	require.NotNil(t, result)
	assert.Equal(t, "CASH-REFUND-1", result.Refund.TradeNo)
}

// 没有收款金额的订单（例如历史脏数据）同样拦住，避免又一条漏网路径。
func TestAdtyZeroPaidOrderRefundIsRejectedPG(t *testing.T) {
	db, root := adtyCashRefundFixture(t)
	seedAdtyCashRefundOrder(t, db, root, "ZERO-PAID-1", PaymentProviderAdtyBEpusdt, 0, decimal.Zero)

	_, err := ApplyAdtyTopUpManualRefund("ZERO-PAID-1", "probe", root.Id, "zero-paid-1", decimal.NewFromInt(100))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be refunded in cash")
}

// ISS-R4-0043：补丁之前就已经 approved 的退款单，仍能从「完成」按钮付出现金。
//
// 申请阶段的门（adtyEnsureCreditedOrder）挡的是新申请；这条测试绕过申请，
// 直接往 adty_manual_refunds 插一行 approved 并把后续步骤都标成已记录，
// 然后调完成接口——模拟「门锁了但屋里还有人」。
func completeAdtyStaleApprovedRefund(t *testing.T, db *gorm.DB, root User, tradeNo, refundID string, extTeam int64, provider string) error {
	t.Helper()
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&AdtyManualRefund{
		RefundId: refundID, TradeNo: tradeNo, UserId: root.Id,
		Status: AdtyManualRefundStatusApproved, RequestUSD: decimal.NewFromInt(100),
		RequestQuota: 50_000_000, Reason: "stale approved row",
		AppliedAt: now, ApproveOperatorId: root.Id, ApprovedAt: now,
		ReclaimedAt: now, BonusAt: now, CommissionAt: now,
		PayoutRecordedAt: now, PayoutStatus: AdtyManualRefundPayoutSuccess,
	}).Error)
	_, err := CompleteAdtyTopUpManualRefund(tradeNo, refundID, root.Id, "complete-"+refundID)
	return err
}

func TestStaleApprovedTeamRefundCannotCompletePG(t *testing.T) {
	db, root := adtyCashRefundFixture(t)
	seedAdtyCashRefundOrder(t, db, root, "STALE-TEAM-1", "adty_team", 42, decimal.Zero)
	// 让申请阶段的门失效，只测完成阶段：这一步模拟补丁之前留下的行。
	require.NoError(t, db.Model(&AdtyTopupOrder{}).Where("trade_no = ?", "STALE-TEAM-1").
		Update("team_id", 42).Error)

	err := completeAdtyStaleApprovedRefund(t, db, root, "STALE-TEAM-1", "stale-team-refund", 42, "adty_team")
	require.Error(t, err, "补丁前遗留的 approved 团队单不能完成退款")
	assert.Contains(t, err.Error(), "cannot be refunded in cash")
	assertAdtyNothingMoved(t, db, root, "STALE-TEAM-1", "stale-team-refund")
}

func TestStaleApprovedVoucherRefundCannotCompletePG(t *testing.T) {
	db, root := adtyCashRefundFixture(t)
	seedAdtyCashRefundOrder(t, db, root, "STALE-VOUCHER-1", AdtyVoucherProvider, 0, decimal.Zero)

	err := completeAdtyStaleApprovedRefund(t, db, root, "STALE-VOUCHER-1", "stale-voucher-refund", 0, AdtyVoucherProvider)
	require.Error(t, err, "补丁前遗留的 approved 兑换码单不能完成退款")
	assert.Contains(t, err.Error(), "cannot be refunded in cash")
	assertAdtyNothingMoved(t, db, root, "STALE-VOUCHER-1", "stale-voucher-refund")
}

// assertAdtyNothingMoved 证明拒绝路径「一分钱没动」，不是只断言返回 error。
func assertAdtyNothingMoved(t *testing.T, db *gorm.DB, root User, tradeNo, refundID string) {
	t.Helper()
	var user User
	require.NoError(t, db.First(&user, root.Id).Error)
	assert.EqualValues(t, root.Quota, user.Quota, "用户余额不能被扣减")

	sums, err := SumAdtyFundBatchRemainingByKind(root.Id)
	require.NoError(t, err)
	assert.Zero(t, sums.Principal+sums.Bonus, "资金批次不能被改动")

	var ext AdtyTopupOrder
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&ext).Error)
	assert.True(t, ext.RefundQuota == 0, "订单退款额度必须仍是 0")
	assert.True(t, ext.RefundAmount.IsZero(), "订单退款金额必须仍是 0")
	assert.NotEqual(t, TopUpStatusRefunded, loadTopUpStatus(t, db, tradeNo), "订单不能被标成已退款")

	var row AdtyManualRefund
	require.NoError(t, db.Where("refund_id = ?", refundID).First(&row).Error)
	assert.Equal(t, AdtyManualRefundStatusApproved, row.Status, "退款单必须停在 approved，不能被推进")
	assert.Zero(t, row.CompletedAt)
}

func loadTopUpStatus(t *testing.T, db *gorm.DB, tradeNo string) string {
	t.Helper()
	var row TopUp
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&row).Error)
	return row.Status
}

// ISS-R4-0043 第 1 条：幂等重放必须原样通过。
//
// 现金背书检查放在 `row.Status == Completed` 的重放分支之后，所以一条历史
// 已完成的退款单再次调用完成接口时，应当按幂等返回原结果，而不是被新校验
// 判成「没有现金背书」——已完成的历史记录不能重算。
func TestCompletedRefundReplaysDespiteCashBackingCheckPG(t *testing.T) {
	db, root := adtyCashRefundFixture(t)
	seedAdtyCashRefundOrder(t, db, root, "REPLAY-TEAM-1", "adty_team", 42, decimal.Zero)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&AdtyManualRefund{
		RefundId: "replay-team-refund", TradeNo: "REPLAY-TEAM-1", UserId: root.Id,
		Status: AdtyManualRefundStatusCompleted, RequestUSD: decimal.NewFromInt(100),
		RequestQuota: 50_000_000, Reason: "already completed before the patch",
		AppliedAt: now, ApprovedAt: now, ReclaimedAt: now, BonusAt: now,
		CommissionAt: now, PayoutRecordedAt: now, PayoutStatus: AdtyManualRefundPayoutSuccess,
		CompletedAt: now, CompleteIdemKey: "complete-replay-team-refund",
	}).Error)

	before := root.Quota
	_, err := CompleteAdtyTopUpManualRefund("REPLAY-TEAM-1", "replay-team-refund", root.Id, "complete-replay-team-refund")
	require.NoError(t, err, "已完成的单重放必须成功，不能被新校验挡住")

	var user User
	require.NoError(t, db.First(&user, root.Id).Error)
	assert.EqualValues(t, before, user.Quota, "重放不能改动余额")

	var row AdtyManualRefund
	require.NoError(t, db.Where("refund_id = ?", "replay-team-refund").First(&row).Error)
	assert.Equal(t, AdtyManualRefundStatusCompleted, row.Status)
	assert.EqualValues(t, now, row.CompletedAt, "重放不能改写完成时间")
}
