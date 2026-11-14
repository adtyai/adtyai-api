package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
	require.NotEmpty(t, held.Code)
	_, err = RedeemAdtyVoucher(customer.Id, held.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherWrongAccount)
	require.NoError(t, CancelAdtyVoucher(owner.Id, held.Id))
	_, err = RedeemAdtyVoucher(owner.Id, held.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherCancelled)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earnings).Error)
	assert.True(t, earnings.Balance.Equal(decimal.RequireFromString("19.50")))
}

func TestAdtyVoucherFreezeMarginAndCloseKeepOriginalLimits(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	ResetAdtyVoucherGuesses()
	root := referralUser(t, db, "voucher-close-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	owner := referralUser(t, db, "voucher-close-owner")
	customer := referralUser(t, db, "voucher-close-customer")
	stranger := referralUser(t, db, "voucher-close-stranger")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "voucher-close", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(40), AdtySiteWalletChange{
		Type: "seed", Reason: "seed", IdempotencyKey: "voucher-close-seed",
	})
	require.NoError(t, err)
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, decimal.NewFromInt(15), AdtySiteWalletChange{
		Type: "seed", Reason: "margin seed", IdempotencyKey: "voucher-close-margin",
	})
	require.NoError(t, err)

	_, err = IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(3), Source: model.AdtyVoucherSourceMargin, Reason: "margin source"})
	require.Error(t, err)
	require.NoError(t, SetAdtyVoucherMarginAuthorization(root.Id, site.Id, true))
	marginIssued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(3), Source: model.AdtyVoucherSourceMargin, Reason: "margin source"})
	require.NoError(t, err)
	require.NoError(t, FreezeAdtyVoucher(owner.Id, marginIssued.Id))
	_, err = RedeemAdtyVoucher(customer.Id, marginIssued.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherFrozen)
	var margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	assert.True(t, margin.Balance.Equal(decimal.NewFromInt(15)), "issuance must freeze margin, not spend it")
	assert.True(t, margin.FrozenUSD.Equal(decimal.NewFromInt(3)))

	held, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(6), Reason: "close"})
	require.NoError(t, err)
	require.NoError(t, db.Model(&site).Updates(map[string]any{"status": model.AdtySiteStatusClosing, "close_execute_at": 1, "close_transfer_to": 0}).Error)
	require.NoError(t, ExecuteAdtySiteClose(site.Id, 10))
	var inherited model.AdtyVoucher
	require.NoError(t, db.First(&inherited, held.Id).Error)
	assert.True(t, inherited.InheritedByMain)
	assert.Equal(t, model.AdtyVoucherStatusPending, inherited.Status)
	_, err = RedeemAdtyVoucher(stranger.Id, held.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherWrongAccount)
	redeemed, err := RedeemAdtyVoucher(customer.Id, held.Code)
	require.NoError(t, err)
	assert.EqualValues(t, 6*500000, redeemed.Quota)
	var earnings model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earnings).Error)
	assert.True(t, earnings.Balance.Equal(decimal.NewFromInt(34)))
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

func TestAdtyVoucherSourceFundsFreezeSpendAndRelease(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	ResetAdtyVoucherGuesses()
	root := referralUser(t, db, "voucher-hold-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	owner := referralUser(t, db, "voucher-hold-owner")
	customer := referralUser(t, db, "voucher-hold-customer")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "voucher-hold", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(30), AdtySiteWalletChange{
		Type: "seed", Reason: "seed earnings", IdempotencyKey: "voucher-hold-earnings",
	})
	require.NoError(t, err)
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, decimal.NewFromInt(15), AdtySiteWalletChange{
		Type: "seed", Reason: "seed margin", IdempotencyKey: "voucher-hold-margin",
	})
	require.NoError(t, err)

	issued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.RequireFromString("10.50"), Reason: "hold"})
	require.NoError(t, err)
	earning := mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletEarnings)
	require.True(t, earning.Balance.Equal(decimal.NewFromInt(30)), "issuance must not spend the source wallet")
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("10.50")))
	availableAfterIssue := earning.Balance.Sub(earning.FrozenUSD)
	require.True(t, availableAfterIssue.Equal(decimal.RequireFromString("19.50")))
	assertAdtyVoucherLedger(t, db, site.Id, "voucher_freeze", model.AdtySiteWalletEarnings, "0", "10.50")

	redeemed, err := RedeemAdtyVoucher(customer.Id, issued.Code)
	require.NoError(t, err)
	require.False(t, redeemed.Replayed)
	earning = mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletEarnings)
	require.True(t, earning.Balance.Equal(decimal.RequireFromString("19.50")))
	require.True(t, earning.FrozenUSD.IsZero())
	require.True(t, earning.Balance.Sub(earning.FrozenUSD).Equal(availableAfterIssue), "redeem must convert the freeze and not change available twice")
	assertAdtyVoucherLedger(t, db, site.Id, "voucher_spend", model.AdtySiteWalletEarnings, "-10.50", "-10.50")

	replay, err := RedeemAdtyVoucher(customer.Id, issued.Code)
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.Equal(t, redeemed.TradeNo, replay.TradeNo)
	var voucher model.AdtyVoucher
	require.NoError(t, db.First(&voucher, issued.Id).Error)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return spendAdtyVoucherSourceTx(tx, &voucher)
	}))
	earningAgain := mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletEarnings)
	require.True(t, earningAgain.Balance.Equal(earning.Balance))
	require.True(t, earningAgain.FrozenUSD.IsZero())
	require.EqualValues(t, 1, countAdtyVoucherLedgers(t, db, site.Id, "voucher_spend", model.AdtySiteWalletEarnings))

	held, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteOwner, SiteId: site.Id, AmountUSD: decimal.NewFromInt(2), Reason: "self"})
	require.NoError(t, err)
	earning = mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletEarnings)
	require.True(t, earning.Balance.Equal(decimal.RequireFromString("19.50")))
	require.True(t, earning.FrozenUSD.Equal(decimal.NewFromInt(2)))
	require.NoError(t, CancelAdtyVoucher(owner.Id, held.Id))
	earning = mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletEarnings)
	require.True(t, earning.Balance.Equal(decimal.RequireFromString("19.50")), "cancel must release the freeze without crediting balance")
	require.True(t, earning.FrozenUSD.IsZero())
	assertAdtyVoucherLedger(t, db, site.Id, "voucher_release", model.AdtySiteWalletEarnings, "0", "-2")

	require.Error(t, CancelAdtyVoucher(owner.Id, held.Id))
	var cancelled model.AdtyVoucher
	require.NoError(t, db.First(&cancelled, held.Id).Error)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return releaseAdtyVoucherSourceTx(tx, &cancelled)
	}))
	earning = mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletEarnings)
	require.True(t, earning.Balance.Equal(decimal.RequireFromString("19.50")))
	require.True(t, earning.FrozenUSD.IsZero())
	require.EqualValues(t, 1, countAdtyVoucherLedgers(t, db, site.Id, "voucher_release", model.AdtySiteWalletEarnings))

	require.NoError(t, SetAdtyVoucherMarginAuthorization(root.Id, site.Id, true))
	marginIssued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(3), Source: model.AdtyVoucherSourceMargin, Reason: "margin hold"})
	require.NoError(t, err)
	margin := mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletMargin)
	require.True(t, margin.Balance.Equal(decimal.NewFromInt(15)), "issuance must not spend margin")
	require.True(t, margin.FrozenUSD.Equal(decimal.NewFromInt(3)))
	assertAdtyVoucherLedger(t, db, site.Id, "voucher_freeze", model.AdtySiteWalletMargin, "0", "3")
	marginRedeem, err := RedeemAdtyVoucher(customer.Id, marginIssued.Code)
	require.NoError(t, err)
	require.False(t, marginRedeem.Replayed)
	margin = mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletMargin)
	require.True(t, margin.Balance.Equal(decimal.NewFromInt(12)))
	require.True(t, margin.FrozenUSD.IsZero())
	assertAdtyVoucherLedger(t, db, site.Id, "voucher_spend", model.AdtySiteWalletMargin, "-3", "-3")
}

func TestAdtyVoucherCloseRevokesOwnerAndMarginCodes(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	ResetAdtyVoucherGuesses()
	root := referralUser(t, db, "voucher-revoke-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	owner := referralUser(t, db, "voucher-revoke-owner")
	customer := referralUser(t, db, "voucher-revoke-customer")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "voucher-revoke", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(40), AdtySiteWalletChange{
		Type: "seed", Reason: "seed", IdempotencyKey: "voucher-revoke-seed",
	})
	require.NoError(t, err)
	require.NoError(t, SetAdtyVoucherIssuanceLimit(root.Id, decimal.NewFromInt(20)))

	selfIssued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteOwner, SiteId: site.Id, AmountUSD: decimal.NewFromInt(5), Reason: "self pending"})
	require.NoError(t, err)
	frozenSelf, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteOwner, SiteId: site.Id, AmountUSD: decimal.NewFromInt(2), Reason: "self frozen"})
	require.NoError(t, err)
	require.NoError(t, FreezeAdtyVoucher(owner.Id, frozenSelf.Id))
	marginIssued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeSiteMargin, SiteId: site.Id, AmountUSD: decimal.NewFromInt(4), Reason: "margin pending"})
	require.NoError(t, err)
	frozenMargin, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeSiteMargin, SiteId: site.Id, AmountUSD: decimal.NewFromInt(3), Reason: "margin frozen"})
	require.NoError(t, err)
	require.NoError(t, FreezeAdtyVoucher(root.Id, frozenMargin.Id))
	customerIssued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(6), Reason: "customer keep"})
	require.NoError(t, err)

	earning := mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletEarnings)
	require.True(t, earning.Balance.Equal(decimal.NewFromInt(40)))
	require.True(t, earning.FrozenUSD.Equal(decimal.NewFromInt(13)))
	var liability model.AdtyVoucherLiability
	require.NoError(t, db.Where("scope = ?", "main").First(&liability).Error)
	require.True(t, liability.FrozenUSD.Equal(decimal.NewFromInt(7)))
	require.True(t, liability.SpentUSD.IsZero())

	require.NoError(t, db.Model(&site).Updates(map[string]any{"status": model.AdtySiteStatusClosing, "close_execute_at": 1, "close_transfer_to": 0}).Error)
	require.NoError(t, ExecuteAdtySiteClose(site.Id, 10))

	var selfRow, frozenSelfRow, marginRow, frozenMarginRow, customerRow model.AdtyVoucher
	require.NoError(t, db.First(&selfRow, selfIssued.Id).Error)
	require.NoError(t, db.First(&frozenSelfRow, frozenSelf.Id).Error)
	require.NoError(t, db.First(&marginRow, marginIssued.Id).Error)
	require.NoError(t, db.First(&frozenMarginRow, frozenMargin.Id).Error)
	require.NoError(t, db.First(&customerRow, customerIssued.Id).Error)
	assert.Equal(t, model.AdtyVoucherStatusCancelled, selfRow.Status)
	assert.Equal(t, model.AdtyVoucherStatusCancelled, frozenSelfRow.Status)
	assert.Equal(t, model.AdtyVoucherStatusCancelled, marginRow.Status)
	assert.Equal(t, model.AdtyVoucherStatusCancelled, frozenMarginRow.Status)
	assert.False(t, selfRow.InheritedByMain)
	assert.False(t, frozenSelfRow.InheritedByMain)
	assert.False(t, marginRow.InheritedByMain)
	assert.False(t, frozenMarginRow.InheritedByMain)
	assert.Equal(t, model.AdtyVoucherStatusPending, customerRow.Status)
	assert.True(t, customerRow.InheritedByMain)

	earning = mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletEarnings)
	require.True(t, earning.Balance.Equal(decimal.NewFromInt(40)), "close must release owner holds without spending earnings")
	require.True(t, earning.FrozenUSD.Equal(decimal.NewFromInt(6)), "customer code must keep its earnings hold")
	require.True(t, earning.Balance.Sub(earning.FrozenUSD).Equal(decimal.NewFromInt(34)))
	require.EqualValues(t, 2, countAdtyVoucherLedgers(t, db, site.Id, "voucher_release", model.AdtySiteWalletEarnings))
	require.EqualValues(t, 0, countAdtyVoucherLedgers(t, db, site.Id, "voucher_spend", model.AdtySiteWalletEarnings))
	require.NoError(t, db.Where("scope = ?", "main").First(&liability).Error)
	require.True(t, liability.FrozenUSD.IsZero(), "close must release platform margin holds")
	require.True(t, liability.SpentUSD.IsZero())

	_, err = RedeemAdtyVoucher(owner.Id, selfIssued.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherCancelled)
	_, err = RedeemAdtyVoucher(owner.Id, frozenSelf.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherCancelled)
	_, err = RedeemAdtyVoucher(owner.Id, marginIssued.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherCancelled)
	_, err = RedeemAdtyVoucher(owner.Id, frozenMargin.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherCancelled)

	redeemed, err := RedeemAdtyVoucher(customer.Id, customerIssued.Code)
	require.NoError(t, err)
	assert.EqualValues(t, 6*500000, redeemed.Quota)
	earning = mustAdtyVoucherWallet(t, db, site.Id, model.AdtySiteWalletEarnings)
	require.True(t, earning.Balance.Equal(decimal.NewFromInt(34)))
	require.True(t, earning.FrozenUSD.IsZero())
}

func mustAdtyVoucherWallet(t *testing.T, db *gorm.DB, siteId int, kind string) model.AdtySiteWallet {
	t.Helper()
	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", siteId, kind).First(&wallet).Error)
	return wallet
}

func assertAdtyVoucherLedger(t *testing.T, db *gorm.DB, siteId int, typ, kind, delta, hold string) {
	t.Helper()
	var entry model.AdtySiteWalletLedger
	require.NoError(t, db.Where("site_id = ? AND type = ? AND wallet_kind = ?", siteId, typ, kind).First(&entry).Error)
	require.True(t, entry.Delta.Equal(decimal.RequireFromString(delta)), "delta=%s typ=%s kind=%s", entry.Delta, typ, kind)
	require.True(t, entry.HoldDelta.Equal(decimal.RequireFromString(hold)), "hold=%s typ=%s kind=%s", entry.HoldDelta, typ, kind)
}

func countAdtyVoucherLedgers(t *testing.T, db *gorm.DB, siteId int, typ, kind string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ? AND type = ? AND wallet_kind = ?", siteId, typ, kind).Count(&n).Error)
	return n
}

func TestAdtyVoucherGuessLimitSharedAcrossRedisClients(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	previousRedis, previousClient := common.RedisEnabled, common.RDB
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	require.NoError(t, client.Ping(context.Background()).Err())
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled = previousRedis
		common.RDB = previousClient
		ResetAdtyVoucherGuesses()
	})
	ResetAdtyVoucherGuesses()

	now := time.Now().Unix()
	userA := 42
	// A timestamp at the window edge is already expired and must not count.
	noteAdtyVoucherGuess(userA, now-adtyVoucherGuessWindow)
	require.True(t, allowAdtyVoucherGuess(userA, now))

	// Two logical clients share one Redis budget for the same user.
	for i := 0; i < adtyVoucherGuessMax; i++ {
		require.True(t, allowAdtyVoucherGuess(userA, now), "shared allow before note %d", i)
		noteAdtyVoucherGuess(userA, now)
	}
	require.False(t, allowAdtyVoucherGuess(userA, now), "9th failed guess in the window must be denied")
	require.True(t, allowAdtyVoucherGuess(userA, now+adtyVoucherGuessWindow), "guesses older than 15 minutes must drop out")
	require.True(t, allowAdtyVoucherGuess(userA+1, now), "a different user has a separate budget")

	root := referralUser(t, db, "voucher-guess-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	mainUser := referralUser(t, db, "voucher-guess-main")
	require.NoError(t, SetAdtyVoucherIssuanceLimit(root.Id, decimal.NewFromInt(10)))
	issued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeMainUser, AmountUSD: decimal.NewFromInt(1), Reason: "guess-success"})
	require.NoError(t, err)
	redeemed, err := RedeemAdtyVoucher(mainUser.Id, issued.Code)
	require.NoError(t, err)
	require.False(t, redeemed.Replayed)
	replay, err := RedeemAdtyVoucher(mainUser.Id, issued.Code)
	require.NoError(t, err)
	require.True(t, replay.Replayed)

	// Successful redeem and replay must not consume a failed-guess slot.
	guessNow := time.Now().Unix()
	for i := 0; i < adtyVoucherGuessMax; i++ {
		_, err = RedeemAdtyVoucher(mainUser.Id, "NOTAREALVOUCHERCODE1")
		require.ErrorIs(t, err, ErrAdtyVoucherInvalid, "failed guess %d", i)
	}
	_, err = RedeemAdtyVoucher(mainUser.Id, "NOTAREALVOUCHERCODE1")
	require.ErrorIs(t, err, ErrAdtyVoucherRate)
	require.False(t, allowAdtyVoucherGuess(mainUser.Id, guessNow))

	// Redis marked enabled with a missing client fails closed and does not
	// fall back to the in-memory counter.
	common.RDB = nil
	require.False(t, allowAdtyVoucherGuess(99, guessNow))
	_, err = RedeemAdtyVoucher(99, "NOTAREALVOUCHERCODE1")
	require.ErrorIs(t, err, ErrAdtyVoucherRate)
	voucherGuess.mu.Lock()
	_, remembered := voucherGuess.fails[99]
	voucherGuess.mu.Unlock()
	require.False(t, remembered, "fail-closed Redis must not record an in-memory guess")
}
