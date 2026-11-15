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
