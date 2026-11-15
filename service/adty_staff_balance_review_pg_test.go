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

// 审核扣减必须用**逻辑余额**去扣批次，不是 raw quota。
//
// 这条走的是审核入口（`ReviewAdtyStaffBalanceRequest` → `applyApprovedStaffBalanceTx`），
// 不是 `AdjustUserQuota` 那条路。`TestAdjustUsesLogicalBoundsForTransientRawRows`
// 只覆盖后者，证明不了这一处——两条路各写一遍「在哪拿 before」。
//
// 缺陷形态：客户余额 quota=10 / pending=100（逻辑 110），审核扣 50。
// 准入判据用逻辑余额、扣批次用 raw quota=10 → 「批次不够」→ 整笔回滚，
// 管理员报 `fund batches cannot cover the debit: uncovered 40`，且没有绕法。
func TestStaffBalanceReviewDebitsBatchesFromLogicalBalancePG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	const (
		rawQuota = 10
		pending  = 100
		deduct   = 50
	)
	owner := ticketTestUser(t, db, "review-owner", 1)
	site := model.AdtySite{Subdomain: "review-site", RootDomain: "example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("owner_user_id", owner.Id).Error)

	customer := ticketTestUser(t, db, "review-customer", 1)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Updates(map[string]any{
		"quota": rawQuota, "quota_pending": pending, "site_id": site.Id,
	}).Error)

	// **一条批次都不建**，这是缺陷真正暴露的形状。
	//
	// 有批次时 `short = balanceBefore - lotTotal` 为负，补建分支根本不进，
	// 传 raw 还是逻辑都一样——缺陷被掩盖。没有批次时补建基数就是 balanceBefore：
	// 传逻辑余额 110 → 补建 110、扣 50 剩 60；传 raw 10 → 补建 10、
	// 扣 50 报 `uncovered 40`（正是工单里那个数）。
	//
	// 我第一版在这里造了 110 的批次，反向验证时加了 bug 仍然绿——那条测试是假的。

	// AmountUSD 换回 quota：delta 必须正好是 50。
	amount := decimal.NewFromInt(int64(deduct)).Div(common.QuotaPerUnitDecimal())
	request := model.AdtyStaffBalanceRequest{
		SiteId: site.Id, StaffUserId: owner.Id, CustomerUserId: customer.Id,
		Direction: model.AdtyStaffBalanceDecrease, AmountUSD: amount,
		Status: model.AdtyStaffBalancePending,
	}
	require.NoError(t, db.Create(&request).Error)

	require.NoError(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", request.Id, true),
		"钱够就必须能扣；这里报 fund batches cannot cover the debit 就是本单的缺陷")

	var after model.User
	require.NoError(t, db.First(&after, customer.Id).Error)
	assert.EqualValues(t, rawQuota-deduct, after.Quota, "raw quota 被扣到 -40")
	assert.EqualValues(t, pending, after.QuotaPending, "pending 不动")

	logical, err := model.GetUserQuota(customer.Id, false)
	require.NoError(t, err)
	assert.EqualValues(t, rawQuota+pending-deduct, logical, "逻辑余额 60")

	var lots []model.AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", customer.Id).Find(&lots).Error)
	require.Len(t, lots, 1, "补建出唯一那条 legacy 批次")
	assert.EqualValues(t, 110, lots[0].Original, "补建基数是逻辑余额 110，不是 raw 10")
	assert.EqualValues(t, 60, lots[0].Remaining, "110 - 50")

	var reviewed model.AdtyStaffBalanceRequest
	require.NoError(t, db.First(&reviewed, request.Id).Error)
	assert.Equal(t, model.AdtyStaffBalanceApproved, reviewed.Status)
}

// 反向：raw quota 不够但逻辑余额够的账户，**不能**因为「raw 是 0」被拒。
// 这是同一条缺陷的另一副面孔：raw=0 时 AdtyDebitUserQuotaTx 会返回 before>0、
// debited 正常，但传错 before 会让补建基数变成 0，扣不动。
func TestStaffBalanceReviewWorksWhenRawQuotaIsExhaustedPG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	owner := ticketTestUser(t, db, "review2-owner", 1)
	site := model.AdtySite{Subdomain: "review2-site", RootDomain: "example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("owner_user_id", owner.Id).Error)

	customer := ticketTestUser(t, db, "review2-customer", 1)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Updates(map[string]any{
		"quota": 0, "quota_pending": 80, "site_id": site.Id,
	}).Error)
	// 这条账户**没有批次**：额度全在 pending 里，批次为空，靠补建兜住。
	amount := decimal.NewFromInt(30).Div(common.QuotaPerUnitDecimal())
	request := model.AdtyStaffBalanceRequest{
		SiteId: site.Id, StaffUserId: owner.Id, CustomerUserId: customer.Id,
		Direction: model.AdtyStaffBalanceDecrease, AmountUSD: amount,
		Status: model.AdtyStaffBalancePending,
	}
	require.NoError(t, db.Create(&request).Error)

	require.NoError(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", request.Id, true))

	logical, err := model.GetUserQuota(customer.Id, false)
	require.NoError(t, err)
	assert.EqualValues(t, 50, logical, "80 - 30")

	_ = gorm.ErrRecordNotFound
}
