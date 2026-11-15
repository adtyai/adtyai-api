package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 站点运营钱包的扣款入口只有 model.AdtyAfterPrincipalSpentTx。正常结算路径
// （预扣登记过分摊行）走的是 deductAdtyFundBatchTx，它不触发这个 hook，
// 于是「站长该付给平台的本金」在绝大多数用户身上从来没被扣过。
//
// 这个 fixture 造的就是最常见的形状：用户有本金批次，挂在站点下，
// 预扣 → 结算。断言站点运营钱包按本金份额减少。
type operatingSettleFixture struct {
	db       *gorm.DB
	site     *model.AdtySite
	customer *model.User
}

// 站内客户的本金：$2（$1 = 500000 quota）。
const operatingSettlePrincipal = 1_000_000

func newOperatingSettleFixture(t *testing.T) *operatingSettleFixture {
	t.Helper()
	db := adtyTestDB(t)
	// 版本只作证据，不作通过条件（AGENTS.md：不设 PostgreSQL 版本上下限）。
	versionNum, version, err := model.OfficialPostgresServerVersion(db)
	require.NoError(t, err)
	t.Logf("evidence only: server_version_num=%s server_version=%s", versionNum, version)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, model.MigrateAdtyTablesOn(db))
	require.NoError(t, db.AutoMigrate(model.AdtySiteTables()...))
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false

	owner := &model.User{Username: "op-settle-owner", Password: "hashed-password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: "op-settle-owner@example.com"}
	require.NoError(t, db.Create(owner).Error)

	site := &model.AdtySite{OwnerUserId: owner.Id, Subdomain: "op-settle", RootDomain: "sites.example.com",
		Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))

	customer := &model.User{Username: "op-settle-customer", Password: "hashed-password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: "op-settle-customer@example.com",
		Quota: operatingSettlePrincipal, SiteId: site.Id}
	require.NoError(t, db.Create(customer).Error)
	require.NoError(t, db.Create(&model.AdtySiteCustomer{UserId: customer.Id, SiteId: site.Id}).Error)
	// 本金批次：站点充值入账时建出来的那种（source 不是 legacy）。
	require.NoError(t, db.Create(&model.AdtyFundBatch{
		UserId: customer.Id, Kind: "principal", Source: "topup",
		Original: operatingSettlePrincipal, Remaining: operatingSettlePrincipal,
	}).Error)

	return &operatingSettleFixture{db: db, site: site, customer: customer}
}

// fundOperating 给运营钱包垫上本金，模拟站长已经付过平台的那笔钱。
func (f *operatingSettleFixture) fundOperating(t *testing.T, usd string, key string) {
	t.Helper()
	_, err := ApplyAdtySiteWalletChange(f.site.Id, model.AdtySiteWalletOperating, decimal.RequireFromString(usd), AdtySiteWalletChange{
		Type: "topup_credit", RefType: "topup", RefID: key, IdempotencyKey: key,
	})
	require.NoError(t, err)
}

func (f *operatingSettleFixture) operating(t *testing.T) decimal.Decimal {
	t.Helper()
	var wallet model.AdtySiteWallet
	require.NoError(t, f.db.Where("site_id = ? AND kind = ?", f.site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	return wallet.Balance
}

func (f *operatingSettleFixture) quota(t *testing.T) int {
	t.Helper()
	var user model.User
	require.NoError(t, f.db.First(&user, f.customer.Id).Error)
	return user.Quota
}

func (f *operatingSettleFixture) remaining(t *testing.T) int64 {
	t.Helper()
	var sum struct{ Remaining int64 }
	require.NoError(t, f.db.Model(&model.AdtyFundBatch{}).Where("user_id = ?", f.customer.Id).
		Select("coalesce(sum(remaining),0) as remaining").Scan(&sum).Error)
	return sum.Remaining
}

func (f *operatingSettleFixture) consumeLedgers(t *testing.T) []model.AdtySiteWalletLedger {
	t.Helper()
	var rows []model.AdtySiteWalletLedger
	require.NoError(t, f.db.Where("site_id = ? AND wallet_kind = ? AND type = ?",
		f.site.Id, model.AdtySiteWalletOperating, "principal_consume").Find(&rows).Error)
	return rows
}

func (f *operatingSettleFixture) reserve(t *testing.T, key string, quota int) {
	t.Helper()
	_, err := model.ReserveBillingOperation(model.BillingOperation{
		OperationKey: key, UserId: f.customer.Id, Kind: "request",
		SiteId: f.site.Id, SiteMarkup: 2, SiteCostFactor: 0.8,
	}, quota)
	require.NoError(t, err)
}

func (f *operatingSettleFixture) settle(t *testing.T, key string, actual int) {
	t.Helper()
	require.NoError(t, model.QueueBillingSettlement(key, actual, false))
	_, _, err := model.ApplyBillingSettlement(key)
	require.NoError(t, err)
}

// 正常结算（有分摊行）必须按本金份额扣站点运营钱包。
//
// 0.6 USD = 300000 quota / 500000。修前运营钱包停在 1（一分没动），
// 而用户的本金批次已经从 1,000,000 扣到 700,000。
func TestUsageSettlementDebitsSiteOperatingWalletPG(t *testing.T) {
	f := newOperatingSettleFixture(t)
	f.fundOperating(t, "1", "op-settle-fund-1")

	f.reserve(t, "request:op-settle-1", 300_000)
	// 预扣登记了分摊行 —— 这决定了结算走 deductAdtyFundBatchTx 而不是 hook 那条。
	var shares int64
	require.NoError(t, f.db.Model(&model.AdtyFundAllocation{}).
		Where("user_id = ? AND operation_id = ?", f.customer.Id, "request:op-settle-1").Count(&shares).Error)
	require.EqualValues(t, 1, shares, "本金批次足够，预扣必须登记分摊行（这就是「正常路径」）")

	f.settle(t, "request:op-settle-1", 300_000)

	assert.Equal(t, 700_000, f.quota(t), "用户余额扣掉 300000")
	assert.EqualValues(t, 700_000, f.remaining(t), "本金批次同步扣掉 300000")
	assert.Equal(t, "0.4", f.operating(t).String(),
		"站点运营钱包必须按本金份额减少 300000/500000 = 0.6 USD；停在 1 就是「站长该付的钱没扣」")

	ledgers := f.consumeLedgers(t)
	require.Len(t, ledgers, 1, "一次结算只记一条本金消费")
	assert.Equal(t, "-0.6", ledgers[0].Delta.String())
	assert.Equal(t, "request:op-settle-1", ledgers[0].RefId, "流水要指回这笔 operation")
}

// 结算重放不能重复扣：ApplyBillingSettlement 本身按 state 短路，
// 这里同时钉住运营钱包那条流水不会出现第二条。
func TestSettlementReplayDoesNotDebitOperatingWalletTwicePG(t *testing.T) {
	f := newOperatingSettleFixture(t)
	f.fundOperating(t, "1", "op-settle-fund-replay")

	f.reserve(t, "request:op-replay-1", 300_000)
	f.settle(t, "request:op-replay-1", 300_000)
	before := f.operating(t)

	_, _, err := model.ApplyBillingSettlement("request:op-replay-1")
	require.NoError(t, err)
	assert.True(t, before.Equal(f.operating(t)), "重放不得再扣一次运营钱包")
	require.Len(t, f.consumeLedgers(t), 1)
}

// 退款（实际消耗 0）不能扣运营钱包：本金批次一分没动，就没有「平台成本」发生。
func TestRefundSettlementDoesNotDebitOperatingWalletPG(t *testing.T) {
	f := newOperatingSettleFixture(t)
	f.fundOperating(t, "1", "op-settle-fund-refund")

	f.reserve(t, "request:op-refund-1", 300_000)
	require.NoError(t, model.QueueBillingSettlement("request:op-refund-1", 0, true))
	_, _, err := model.ApplyBillingSettlement("request:op-refund-1")
	require.NoError(t, err)

	assert.Equal(t, operatingSettlePrincipal, f.quota(t), "全额退款，用户余额复原")
	assert.EqualValues(t, operatingSettlePrincipal, f.remaining(t), "批次从未被扣")
	assert.Equal(t, "1", f.operating(t).String(), "退款不是消费，运营钱包不能动")
	assert.Empty(t, f.consumeLedgers(t))
}

// 只消费一部分（预扣 300000，实际 120000）：运营钱包只按实际消耗的本金份额扣。
func TestPartialSettlementDebitsOnlyConsumedPrincipalPG(t *testing.T) {
	f := newOperatingSettleFixture(t)
	f.fundOperating(t, "1", "op-settle-fund-partial")

	f.reserve(t, "request:op-partial-1", 300_000)
	f.settle(t, "request:op-partial-1", 120_000)

	assert.Equal(t, 880_000, f.quota(t))
	assert.EqualValues(t, 880_000, f.remaining(t))
	assert.Equal(t, "0.76", f.operating(t).String(), "0.24 USD = 120000 quota")
}

// 异步任务的差额结算（kind=task 的 adjust operation）只承担**差额**：
// base 是 task.Quota，提交时那个 request operation 已经结过一次、批次早就扣过。
//
// 这条用例钉两件事：
//   - 运营钱包按差额扣（0.2 的首扣 + 0.1 的补扣），不是按「新的总额再扣一次」；
//   - 批次只再动 50000，不会被扣成「总额」并顺手补建一条 legacy 批次
//     （那条凭空出现的 legacy 正好会把 fund_batch_gap 的漂移抹平）。
func TestTaskAdjustmentDebitsOnlyTheIncrementPG(t *testing.T) {
	f := newOperatingSettleFixture(t)
	require.NoError(t, f.db.AutoMigrate(&model.Task{}, &model.Channel{}, &model.Midjourney{}, &model.Log{}))
	f.fundOperating(t, "1", "op-settle-fund-task")

	task := &model.Task{
		TaskID: "op-settle-task", UserId: f.customer.Id, Group: "default",
		Quota: 100_000, Status: model.TaskStatusInProgress,
		Properties: model.Properties{OriginModelName: "op-settle-model"},
	}
	require.NoError(t, f.db.Create(task).Error)

	// 提交时的 request operation：预扣 100000，落库时绑定到 task 上（InsertWithContext 的写法）。
	initialKey := "request:op-settle-task-submit"
	require.NoError(t, f.db.Create(&model.BillingOperation{
		OperationKey: initialKey, UserId: f.customer.Id, Kind: "request", ReferenceId: int64(task.ID),
		State: model.BillingSettlePending, ActualQuota: 100_000, ReservedQuota: 100_000,
		SiteId: f.site.Id, SiteMarkup: 2, SiteCostFactor: 0.8,
	}).Error)
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		return model.AllocateAdtyFundAllocationsTx(tx, f.customer.Id, initialKey, 100_000, operatingSettlePrincipal)
	}))
	require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", f.customer.Id).Update("quota", 900_000).Error)
	_, _, err := model.ApplyBillingSettlement(initialKey)
	require.NoError(t, err)
	require.Equal(t, 900_000, f.quota(t))
	require.EqualValues(t, 900_000, f.remaining(t))
	require.Equal(t, "0.8", f.operating(t).String(), "首扣 100000 quota = 0.2 USD")

	// 完成后按 150000 重算差额（RecalculateTaskQuota 的核心两步）。
	key, err := model.QueueTaskBillingOperation(task, 150_000, false)
	require.NoError(t, err)
	op, _, err := model.ApplyBillingSettlement(key)
	require.NoError(t, err)
	task.Quota = op.AppliedQuota

	assert.EqualValues(t, 100_000, op.ReservedQuota, "差额结算的 base 是 task 的旧额度（已被首扣结过）")
	assert.Equal(t, 850_000, f.quota(t), "用户只再付 50000")
	assert.EqualValues(t, 850_000, f.remaining(t), "批次也只再扣 50000")
	assert.Equal(t, "0.7", f.operating(t).String(), "运营钱包只按差额扣 0.1 USD，不是按新总额再扣一次")

	var lots []model.AdtyFundBatch
	require.NoError(t, f.db.Where("user_id = ?", f.customer.Id).Find(&lots).Error)
	require.Len(t, lots, 1, "差额结算不许补建 legacy 批次")
	assert.EqualValues(t, 0, op.FundBatchGap, "账平：差额确实被批次背书")

	ledgers := f.consumeLedgers(t)
	require.Len(t, ledgers, 2)
	assert.Equal(t, "-0.2", ledgers[0].Delta.String())
	assert.Equal(t, "-0.1", ledgers[1].Delta.String())
}

// 运营钱包余额不够也必须能把账结完。
//
// 结算发生在上游已经跑完之后：这里报「余额不足」让整笔回滚，用户就白用了，
// 恢复任务还会一直重试同一笔。余额变负正是对账要的信号
// （ReconcileOperatingWallet 拿它和站内客户本金比），不是拒绝计费的理由。
func TestSettlementWithEmptyOperatingWalletStillBillsPG(t *testing.T) {
	f := newOperatingSettleFixture(t)

	f.reserve(t, "request:op-unfunded-1", 300_000)
	f.settle(t, "request:op-unfunded-1", 300_000)

	assert.Equal(t, 700_000, f.quota(t), "钱包不够也照常扣用户余额")
	assert.EqualValues(t, 700_000, f.remaining(t))
	assert.Equal(t, "-0.6", f.operating(t).String(), "透支要如实记下来（站长欠平台 0.6 USD）")
	require.Len(t, f.consumeLedgers(t), 1)
}
