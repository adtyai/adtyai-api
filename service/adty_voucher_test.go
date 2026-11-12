package service

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdtyVoucherIssueRedeemAndRejects(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	ResetAdtyVoucherGuesses()
	root := referralUser(t, db, "voucher-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	owner := referralUser(t, db, "voucher-owner")
	customer := referralUser(t, db, "voucher-customer")
	mainUser := referralUser(t, db, "voucher-main")
	other := referralUser(t, db, "voucher-other")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "voucher-site", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(30), AdtySiteWalletChange{
		Type: "seed", Reason: "seed earnings", IdempotencyKey: "voucher-seed-earnings",
	})
	require.NoError(t, err)

	_, err = IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeMainUser, AmountUSD: decimal.NewFromInt(5), Reason: "main"})
	require.ErrorIs(t, err, ErrAdtyVoucherLimit)
	require.NoError(t, SetAdtyVoucherIssuanceLimit(root.Id, decimal.NewFromInt(20)))
	mainIssued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeMainUser, AmountUSD: decimal.NewFromInt(5), Reason: "main"})
	require.NoError(t, err)
	require.Len(t, mainIssued.Code, 26)
	_, err = RedeemAdtyVoucher(customer.Id, mainIssued.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherWrongAccount)
	redeemed, err := RedeemAdtyVoucher(mainUser.Id, mainIssued.Code)
	require.NoError(t, err)
	assert.EqualValues(t, 5*500000, redeemed.Quota)
	replay, err := RedeemAdtyVoucher(mainUser.Id, mainIssued.Code)
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, redeemed.TradeNo, replay.TradeNo)
	_, err = RedeemAdtyVoucher(other.Id, mainIssued.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherUsed)
	var user model.User
	require.NoError(t, db.First(&user, mainUser.Id).Error)
	assert.EqualValues(t, 5*500000, user.Quota)
	var orders int64
	require.NoError(t, db.Model(&model.AdtyTopupOrder{}).Where("trade_no = ? AND provider = ?", redeemed.TradeNo, model.AdtyVoucherProvider).Count(&orders).Error)
	assert.EqualValues(t, 1, orders)

	issued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.RequireFromString("10.50"), Reason: "customer"})
	require.NoError(t, err)
	_, err = IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(100), Reason: "too much"})
	require.ErrorIs(t, err, ErrAdtyVoucherFunds)
	customerRedeem, err := RedeemAdtyVoucher(customer.Id, issued.Code)
	require.NoError(t, err)
	assert.EqualValues(t, 5250000, customerRedeem.Quota)
	var earnings model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earnings).Error)
	assert.True(t, earnings.Balance.Equal(decimal.RequireFromString("19.50")))

	_, err = IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(1), Reason: "root cannot issue customer codes"})
	require.Error(t, err)
	marginCode, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeSiteMargin, SiteId: site.Id, AmountUSD: decimal.NewFromInt(4), Reason: "margin"})
	require.NoError(t, err)
	_, err = RedeemAdtyVoucher(customer.Id, marginCode.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherWrongAccount)
	marginRedeem, err := RedeemAdtyVoucher(owner.Id, marginCode.Code)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyVoucherPurposeSiteMargin, marginRedeem.Purpose)
	var margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	assert.True(t, margin.Balance.Equal(decimal.NewFromInt(4)))
	var ownerRow model.User
	require.NoError(t, db.First(&ownerRow, owner.Id).Error)
	assert.Zero(t, ownerRow.Quota)

	held, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteOwner, SiteId: site.Id, AmountUSD: decimal.NewFromInt(2), Reason: "self"})
	require.NoError(t, err)
	require.NoError(t, CancelAdtyVoucher(owner.Id, held.Id))
	_, err = RedeemAdtyVoucher(owner.Id, held.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherCancelled)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earnings).Error)
	assert.True(t, earnings.Balance.Equal(decimal.RequireFromString("19.50")))
}

func TestAdtyVoucherConcurrentRedeemCreditsOnce(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	ResetAdtyVoucherGuesses()
	root := referralUser(t, db, "voucher-race-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	first := referralUser(t, db, "voucher-race-a")
	second := referralUser(t, db, "voucher-race-b")
	require.NoError(t, SetAdtyVoucherIssuanceLimit(root.Id, decimal.NewFromInt(10)))
	issued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeMainUser, AmountUSD: decimal.NewFromInt(3), Reason: "race"})
	require.NoError(t, err)
	start := make(chan struct{})
	var group sync.WaitGroup
	results := make(chan error, 2)
	for _, user := range []*model.User{first, second} {
		group.Go(func() {
			<-start
			_, err := RedeemAdtyVoucher(user.Id, issued.Code)
			results <- err
		})
	}
	close(start)
	group.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
			continue
		}
		assert.ErrorIs(t, err, ErrAdtyVoucherUsed)
	}
	assert.Equal(t, 1, success)
	var credited int64
	require.NoError(t, db.Model(&model.User{}).Where("id IN ?", []int{first.Id, second.Id}).Select("COALESCE(SUM(quota), 0)").Scan(&credited).Error)
	assert.EqualValues(t, 3*500000, credited)
}
