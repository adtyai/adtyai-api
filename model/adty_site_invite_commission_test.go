package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestSiteInviteCommissionRefundPartsSplitsHoldConvertedWithdrawnAndUncovered(t *testing.T) {
	row := AdtySiteInviteCommission{
		AmountUSD:       decimal.NewFromInt(10),
		EarningsHoldUSD: decimal.NewFromInt(4),
		ConvertedUSD:    decimal.NewFromInt(3),
		WithdrawnUSD:    decimal.NewFromInt(2),
		Status:          AdtySiteCommissionAvailable,
	}
	release, converted, withdrawn, pending, thisClaw := SiteInviteCommissionRefundParts(row, decimal.NewFromInt(2), 100, 100)
	require.True(t, thisClaw.Equal(decimal.NewFromInt(10)))
	require.True(t, release.Equal(decimal.NewFromInt(4)))
	require.True(t, converted.Equal(decimal.NewFromInt(3)))
	require.True(t, withdrawn.Equal(decimal.NewFromInt(2)))
	require.True(t, pending.Equal(decimal.NewFromInt(1)))

	preview := BuildSiteInviteCommissionRefundPreview(row, decimal.NewFromInt(2), 100, 100)
	require.Equal(t, "10", preview.IssuedUSD)
	require.Equal(t, "4", preview.ExpectedReleaseUSD)
	require.Equal(t, "3", preview.ExpectedConvertedClawbackUSD)
	require.Equal(t, "2", preview.ExpectedWithdrawnRecoveryUSD)
	require.Equal(t, "1", preview.ExpectedPendingCoverageCancelUSD)
	require.Equal(t, "10", preview.ExpectedClawbackUSD)
	require.Equal(t, "inviter", preview.WithdrawnRecoveryLiable)

	got := map[string]string{}
	for _, state := range preview.States {
		got[state.State] = state.AmountUSD
		require.NotEqual(t, AdtySiteCommissionCovered, state.State)
		require.NotEqual(t, AdtySiteCommissionAvailable, state.State)
		require.NotEqual(t, AdtySiteCommissionClawedBack, state.State)
	}
	require.Equal(t, "4", got[AdtySiteCommissionStateOnHold])
	require.Equal(t, "1", got[AdtySiteCommissionStatePendingCoverage])
	require.Equal(t, "3", got[AdtySiteCommissionStateConverted])
	require.Equal(t, "2", got[AdtySiteCommissionStateWithdrawnReleased])
	require.NotContains(t, got, AdtySiteCommissionStateWithdrawalRequested)
}

func TestSiteInviteCommissionOfficialStatesSplitHoldAndPendingWithdrawal(t *testing.T) {
	row := AdtySiteInviteCommission{
		AmountUSD:       decimal.NewFromInt(5),
		EarningsHoldUSD: decimal.NewFromInt(3),
		MarginHoldUSD:   decimal.NewFromInt(2),
		FrozenUSD:       decimal.NewFromInt(2),
		Status:          AdtySiteCommissionCovered,
	}
	got := map[string]string{}
	for _, state := range SiteInviteCommissionOfficialStates(row, decimal.Zero) {
		got[state.State] = state.AmountUSD
	}
	require.Equal(t, "3", got[AdtySiteCommissionStateOnHold])
	require.Equal(t, "2", got[AdtySiteCommissionStateWithdrawalRequested])
	require.NotContains(t, got, AdtySiteCommissionCovered)
	require.NotContains(t, got, "covered")
}

func TestLoadAdtyRefundPageFactsIncludesCommissionStatesAndExpectedParts(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	buyer := seedAdtyTopUpUser(t, db, "facts-comm-"+strconv.FormatInt(time.Now().UnixNano()%1_000_000, 10), 0)
	trade := "FACTS-COMM-" + strconv.Itoa(buyer.Id)
	require.NoError(t, db.Create(&TopUp{TradeNo: trade, UserId: buyer.Id, Status: common.TopUpStatusSuccess}).Error)
	require.NoError(t, db.Create(&AdtyTopupOrder{
		TradeNo: trade, UserId: buyer.Id, SiteId: 8, FaceUSD: decimal.NewFromInt(100),
		PaidAmount: decimal.NewFromInt(700), PaidCurrency: "CNY", PayCurrency: "CNY",
		PriceSnapshot: decimal.NewFromInt(7), CreditQuota: 100 * 500000,
	}).Error)
	require.NoError(t, db.Create(&AdtySiteInviteCommission{
		SiteId: 8, InviterId: buyer.Id, InviteeId: buyer.Id, TradeNo: trade,
		FaceUSD: decimal.NewFromInt(100), Rate: decimal.RequireFromString("0.10"),
		AmountUSD: decimal.NewFromInt(10), EarningsHoldUSD: decimal.NewFromInt(4),
		ConvertedUSD: decimal.NewFromInt(3), WithdrawnUSD: decimal.NewFromInt(2),
		Status: AdtySiteCommissionAvailable,
	}).Error)

	facts, err := LoadAdtyRefundPageFacts(trade, decimal.NewFromInt(50))
	require.NoError(t, err)
	require.NotNil(t, facts.Commission)
	require.Equal(t, "10", facts.Commission.IssuedUSD)
	require.Equal(t, "2", facts.Commission.ExpectedReleaseUSD)
	require.Equal(t, "1.5", facts.Commission.ExpectedConvertedClawbackUSD)
	require.Equal(t, "1", facts.Commission.ExpectedWithdrawnRecoveryUSD)
	require.Equal(t, "0.5", facts.Commission.ExpectedPendingCoverageCancelUSD)
	got := map[string]string{}
	for _, state := range facts.Commission.States {
		got[state.State] = state.AmountUSD
	}
	require.Equal(t, "4", got[AdtySiteCommissionStateOnHold])
	require.Equal(t, "1", got[AdtySiteCommissionStatePendingCoverage])
	require.Equal(t, "3", got[AdtySiteCommissionStateConverted])
	require.Equal(t, "2", got[AdtySiteCommissionStateWithdrawnReleased])
}
