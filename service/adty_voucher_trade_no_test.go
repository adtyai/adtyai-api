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

func TestAdtyVoucherQuotaUsesOfficialDecimalHelpers(t *testing.T) {
	old := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = old })

	quota, err := adtyVoucherQuota(decimal.RequireFromString("10.50"))
	require.NoError(t, err)
	official, err := common.WalletQuotaFromDecimalStrict(decimal.RequireFromString("10.50").Mul(common.QuotaPerUnitDecimal()))
	require.NoError(t, err)
	assert.EqualValues(t, official, quota)
	assert.EqualValues(t, 5_250_000, quota)

	_, err = adtyVoucherQuota(decimal.RequireFromString("10.501"))
	require.Error(t, err)
	_, err = adtyVoucherQuota(decimal.Zero)
	require.Error(t, err)
}

func TestAdtyVoucherRedeemTradeNoUniqueAndExactCredit(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	ResetAdtyVoucherGuesses()
	t.Cleanup(func() {
		adtyVoucherTradeNoTestHook = nil
		ResetAdtyVoucherGuesses()
	})

	root := referralUser(t, db, "vch-unique-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	user := referralUser(t, db, "vch-unique-user")
	other := referralUser(t, db, "vch-unique-other")
	require.NoError(t, SetAdtyVoucherIssuanceLimit(root.Id, decimal.NewFromInt(40)))

	amount := decimal.RequireFromString("10.50")
	wantQuota, err := common.WalletQuotaFromDecimalStrict(amount.Mul(common.QuotaPerUnitDecimal()))
	require.NoError(t, err)
	assert.EqualValues(t, 5_250_000, wantQuota)

	issued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeMainUser, AmountUSD: amount, Reason: "exact"})
	require.NoError(t, err)
	assert.True(t, issued.AmountUSD.Equal(amount))
	assert.EqualValues(t, wantQuota, issued.Quota)

	var pending model.AdtyVoucher
	require.NoError(t, db.First(&pending, issued.Id).Error)
	assert.Equal(t, model.AdtyVoucherStatusPending, pending.Status)
	assert.Empty(t, pending.TradeNo)

	secondPending, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeMainUser, AmountUSD: amount, Reason: "pending empty"})
	require.NoError(t, err)
	var emptyCount int64
	require.NoError(t, db.Model(&model.AdtyVoucher{}).Where("trade_no = ? OR trade_no IS NULL", "").Count(&emptyCount).Error)
	assert.GreaterOrEqual(t, emptyCount, int64(2), "empty trade_no on pending vouchers is not blocked by the unique index")

	redeemed, err := RedeemAdtyVoucher(user.Id, issued.Code)
	require.NoError(t, err)
	require.False(t, redeemed.Replayed)
	assert.True(t, redeemed.AmountUSD.Equal(amount))
	assert.EqualValues(t, wantQuota, redeemed.Quota)
	require.NotEmpty(t, redeemed.TradeNo)

	var official model.AdtyTopupOrder
	require.NoError(t, db.Where("trade_no = ? AND provider = ?", redeemed.TradeNo, model.AdtyVoucherProvider).First(&official).Error)
	assert.True(t, official.FaceUSD.Equal(amount))
	assert.True(t, official.PayAmount.Equal(amount))
	assert.True(t, official.PaidAmount.Equal(amount))
	assert.EqualValues(t, wantQuota, official.CreditQuota)
	assert.Equal(t, "USD", official.PayCurrency)
	assert.Equal(t, "voucher", official.PaidSource)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", redeemed.TradeNo).First(&topUp).Error)
	assert.EqualValues(t, wantQuota, topUp.Amount, "compatibility Amount is the exact credited quota, not USD IntPart")
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)

	var credited model.User
	require.NoError(t, db.First(&credited, user.Id).Error)
	assert.EqualValues(t, wantQuota, credited.Quota, "wallet must use official quota, not IntPart/float")
	assert.NotEqualValues(t, amount.IntPart(), credited.Quota)
	assert.NotEqualValues(t, amount.IntPart()*common.FixedQuotaPerUnit, credited.Quota)

	var ledgers int64
	require.NoError(t, db.Model(&model.AdtyBalanceLedger{}).Where("user_id = ? AND type = ? AND ref_id = ?", user.Id, model.AdtyLedgerTypeVoucher, redeemed.TradeNo).Count(&ledgers).Error)
	assert.EqualValues(t, 1, ledgers)

	replay, err := RedeemAdtyVoucher(user.Id, issued.Code)
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, redeemed.TradeNo, replay.TradeNo)
	assert.True(t, replay.AmountUSD.Equal(amount))
	assert.EqualValues(t, wantQuota, replay.Quota)
	require.NoError(t, db.First(&credited, user.Id).Error)
	assert.EqualValues(t, wantQuota, credited.Quota)
	var officialCount, topUpCount int64
	require.NoError(t, db.Model(&model.AdtyTopupOrder{}).Where("trade_no = ?", redeemed.TradeNo).Count(&officialCount).Error)
	require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", redeemed.TradeNo).Count(&topUpCount).Error)
	assert.EqualValues(t, 1, officialCount)
	assert.EqualValues(t, 1, topUpCount)
	_, err = RedeemAdtyVoucher(other.Id, issued.Code)
	require.ErrorIs(t, err, ErrAdtyVoucherUsed)

	adtyVoucherTradeNoTestHook = func(hash string, attempt int) string {
		if attempt == 0 {
			return redeemed.TradeNo
		}
		return ""
	}
	second, err := RedeemAdtyVoucher(user.Id, secondPending.Code)
	require.NoError(t, err)
	require.False(t, second.Replayed)
	assert.NotEqual(t, redeemed.TradeNo, second.TradeNo)
	assert.EqualValues(t, wantQuota, second.Quota)
	require.NoError(t, db.First(&credited, user.Id).Error)
	assert.EqualValues(t, int64(wantQuota)*2, credited.Quota)

	var firstTradeRows int64
	require.NoError(t, db.Model(&model.AdtyVoucher{}).Where("trade_no = ?", redeemed.TradeNo).Count(&firstTradeRows).Error)
	assert.EqualValues(t, 1, firstTradeRows)

	third, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeMainUser, AmountUSD: decimal.NewFromInt(1), Reason: "blocked trade"})
	require.NoError(t, err)
	beforeBlocked, err := func() (int64, error) {
		var row model.User
		if err := db.First(&row, user.Id).Error; err != nil {
			return 0, err
		}
		return int64(row.Quota), nil
	}()
	require.NoError(t, err)
	adtyVoucherTradeNoTestHook = func(hash string, attempt int) string {
		return redeemed.TradeNo
	}
	_, err = RedeemAdtyVoucher(user.Id, third.Code)
	require.Error(t, err)
	var blocked model.AdtyVoucher
	require.NoError(t, db.First(&blocked, third.Id).Error)
	assert.Equal(t, model.AdtyVoucherStatusPending, blocked.Status)
	assert.Empty(t, blocked.TradeNo)
	require.NoError(t, db.First(&credited, user.Id).Error)
	assert.EqualValues(t, beforeBlocked, credited.Quota)
	require.NoError(t, db.Model(&model.AdtyVoucher{}).Where("trade_no = ?", redeemed.TradeNo).Count(&firstTradeRows).Error)
	assert.EqualValues(t, 1, firstTradeRows)
}

func TestAdtyVoucherConcurrentRedeemOnPostgresCreditsOnce(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	ResetAdtyVoucherGuesses()
	t.Cleanup(ResetAdtyVoucherGuesses)

	root := referralUser(t, db, "vch-pg-race-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	first := referralUser(t, db, "vch-pg-race-a")
	second := referralUser(t, db, "vch-pg-race-b")
	require.NoError(t, SetAdtyVoucherIssuanceLimit(root.Id, decimal.NewFromInt(10)))
	issued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: root.Id, Purpose: model.AdtyVoucherPurposeMainUser, AmountUSD: decimal.RequireFromString("3.00"), Reason: "race"})
	require.NoError(t, err)

	type redeemResult struct {
		redeemed *AdtyVoucherRedeemed
		err      error
	}
	start := make(chan struct{})
	var group sync.WaitGroup
	results := make(chan redeemResult, 2)
	for _, user := range []*model.User{first, second} {
		group.Go(func() {
			<-start
			redeemed, err := RedeemAdtyVoucher(user.Id, issued.Code)
			results <- redeemResult{redeemed: redeemed, err: err}
		})
	}
	close(start)
	group.Wait()
	close(results)

	var success *AdtyVoucherRedeemed
	used := 0
	for result := range results {
		if result.err == nil {
			require.Nil(t, success, "only one redeem may succeed")
			success = result.redeemed
			continue
		}
		assert.ErrorIs(t, result.err, ErrAdtyVoucherUsed)
		used++
	}
	require.NotNil(t, success)
	assert.False(t, success.Replayed)
	assert.Equal(t, 1, used)

	wantQuota, err := common.WalletQuotaFromDecimalStrict(decimal.RequireFromString("3.00").Mul(common.QuotaPerUnitDecimal()))
	require.NoError(t, err)
	var credited int64
	require.NoError(t, db.Model(&model.User{}).Where("id IN ?", []int{first.Id, second.Id}).Select("COALESCE(SUM(quota), 0)").Scan(&credited).Error)
	assert.EqualValues(t, wantQuota, credited)
	var tradeRows, official, ledgers int64
	require.NoError(t, db.Model(&model.AdtyVoucher{}).Where("trade_no = ?", success.TradeNo).Count(&tradeRows).Error)
	require.NoError(t, db.Model(&model.AdtyTopupOrder{}).Where("trade_no = ?", success.TradeNo).Count(&official).Error)
	require.NoError(t, db.Model(&model.AdtyBalanceLedger{}).Where("type = ? AND ref_id = ?", model.AdtyLedgerTypeVoucher, success.TradeNo).Count(&ledgers).Error)
	assert.EqualValues(t, 1, tradeRows)
	assert.EqualValues(t, 1, official)
	assert.EqualValues(t, 1, ledgers)
}
