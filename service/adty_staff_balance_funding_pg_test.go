package service

import (
	"testing"

	"github.com/QuantumNous/new-api/internal/pgtest"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 员工给客户加款必须先有出资（`ISS-R4-0075`）。
//
// 修前 increase 分支只做「加客户额度 + 建一条无资金来源的批次」，全函数零出资
// 调用——于是一个员工可以让平台凭空给客户加钱，批次表还把这笔钱记成「有背书」。
func TestStaffBalanceIncreaseIsRejectedWithoutSiteFundsPG(t *testing.T) {
	db, owner, _, request := newStaffBalanceFundingFixture(t, "no-funds")
	pgtest.RecordServerVersion(t, db)

	var before model.User
	require.NoError(t, db.First(&before, request.CustomerUserId).Error)

	err := ReviewAdtyStaffBalanceRequest(owner.Id, "owner", request.Id, true)
	require.Error(t, err, "站点三个钱包都是 0 时，加款必须被拒")
	assert.ErrorIs(t, err, ErrAdtySiteWalletInsufficient)

	var after model.User
	require.NoError(t, db.First(&after, request.CustomerUserId).Error)
	assert.EqualValues(t, before.Quota, after.Quota, "被拒时客户余额一分不动")

	var reviewed model.AdtyStaffBalanceRequest
	require.NoError(t, db.First(&reviewed, request.Id).Error)
	assert.Equal(t, model.AdtyStaffBalancePending, reviewed.Status, "整笔事务回滚，请求仍待审")
}

// 运营钱包够 → 加款成功，而且钱确实从站点钱包扣走了。
func TestStaffBalanceIncreaseIsFundedBySiteWalletPG(t *testing.T) {
	db, owner, site, request := newStaffBalanceFundingFixture(t, "funded")
	pgtest.RecordServerVersion(t, db)

	const amountUSD = 100
	require.NoError(t, fundStaffBalanceWallet(t, site.Id, model.AdtySiteWalletOperating, amountUSD, "funded-seed"))

	err := ReviewAdtyStaffBalanceRequest(owner.Id, "owner", request.Id, true)
	require.NoError(t, err)

	var after model.User
	require.NoError(t, db.First(&after, request.CustomerUserId).Error)
	assert.EqualValues(t, int64(amountUSD)*500_000, after.Quota, "客户拿到 $100 对应额度")

	var operating model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&operating).Error)
	assert.True(t, operating.Balance.IsZero(), "运营钱包被扣到 0：$100 全出了")

	var reviewed model.AdtyStaffBalanceRequest
	require.NoError(t, db.First(&reviewed, request.Id).Error)
	assert.Equal(t, model.AdtyStaffBalanceApproved, reviewed.Status)
}

// 运营钱包不够 → 整笔拒绝，客户余额和钱包都不变（事务回滚，不是部分扣款）。
func TestStaffBalanceIncreaseRollsBackWhenSiteFundsAreShortPG(t *testing.T) {
	db, owner, site, request := newStaffBalanceFundingFixture(t, "short")
	pgtest.RecordServerVersion(t, db)

	require.NoError(t, fundStaffBalanceWallet(t, site.Id, model.AdtySiteWalletOperating, 50, "short-seed"))

	var before model.User
	require.NoError(t, db.First(&before, request.CustomerUserId).Error)

	err := ReviewAdtyStaffBalanceRequest(owner.Id, "owner", request.Id, true)
	require.Error(t, err, "只有 $50 却要加 $100，必须拒")

	var after model.User
	require.NoError(t, db.First(&after, request.CustomerUserId).Error)
	assert.EqualValues(t, before.Quota, after.Quota, "客户余额不变")

	var operating model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&operating).Error)
	assert.True(t, operating.Balance.Equal(decimal.NewFromInt(50)), "运营钱包也没有被部分扣款")
}

// —— 夹具 ——

// newStaffBalanceFundingFixture 造一个站点 + 员工 + 客户 + 一笔待审的 $100 加款请求，
// **不给站点钱包放钱**（放钱的用例自己调 fundStaffBalanceWallet）。
func newStaffBalanceFundingFixture(t *testing.T, tag string) (*gorm.DB, *model.User, model.AdtySite, model.AdtyStaffBalanceRequest) {
	t.Helper()
	db := announcementTestDB(t)
	owner := ticketTestUser(t, db, "fund-"+tag+"-owner", 1)
	staff := ticketTestUser(t, db, "fund-"+tag+"-staff", 1)
	customer := ticketTestUser(t, db, "fund-"+tag+"-customer", 1)
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "fund-" + tag,
		Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("site_id", site.Id).Error)
	require.NoError(t, db.Create(&model.AdtySiteStaff{
		SiteId: site.Id, OwnerUserId: owner.Id, UserId: staff.Id, Status: "enabled", Role: "support",
	}).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))

	request := model.AdtyStaffBalanceRequest{
		SiteId: site.Id, StaffUserId: staff.Id, CustomerUserId: customer.Id,
		Direction: model.AdtyStaffBalanceIncrease, AmountUSD: decimal.NewFromInt(100),
		Status: model.AdtyStaffBalancePending,
	}
	require.NoError(t, db.Create(&request).Error)
	return db, owner, site, request
}

func fundStaffBalanceWallet(t *testing.T, siteID int, kind string, amount int64, key string) error {
	t.Helper()
	_, err := ApplyAdtySiteWalletChange(siteID, kind, decimal.NewFromInt(amount), AdtySiteWalletChange{
		Type: "seed", RefType: "test", RefID: key, IdempotencyKey: key,
	})
	return err
}
