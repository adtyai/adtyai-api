package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTeamTopupOrderCreditsPrincipalWithoutGift(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leader := referralUser(t, db, "team-topup-leader")
	created, err := CreateAdtyTeam(leader.Id, 0, "Topup", decimal.NewFromInt(1))
	require.NoError(t, err)
	order, err := CreateAdtyTeamTopupOrder(leader.Id, created.Team.Id, decimal.NewFromInt(1), "team-pay-1")
	require.NoError(t, err)
	require.True(t, order.BonusUSD.IsZero())
	require.True(t, order.BonusRate.IsZero())
	require.True(t, order.PromoBonusUSD.IsZero())
	require.EqualValues(t, 500000, order.CreditQuota)

	replay, err := CreateAdtyTeamTopupOrder(leader.Id, created.Team.Id, decimal.NewFromInt(1), "team-pay-1")
	require.NoError(t, err)
	require.Equal(t, order.TradeNo, replay.TradeNo)

	err = db.Transaction(func(tx *gorm.DB) error {
		_, _, _, _, err := model.CompleteAdtyTopUpTx(tx, order.TradeNo, model.AdtyPaymentFacts{
			Provider: model.PaymentProviderAdtyEpay, Source: model.AdtyPaymentSourceManual,
		})
		return err
	})
	require.NoError(t, err)
	var team model.AdtyTeam
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	require.EqualValues(t, 500000, team.SharedQuota)
	var user model.User
	require.NoError(t, db.First(&user, leader.Id).Error)
	require.Equal(t, 0, user.Quota)

	err = db.Transaction(func(tx *gorm.DB) error {
		already, _, _, _, err := model.CompleteAdtyTopUpTx(tx, order.TradeNo, model.AdtyPaymentFacts{
			Provider: model.PaymentProviderAdtyEpay, Source: model.AdtyPaymentSourceManual,
		})
		require.True(t, already)
		return err
	})
	require.NoError(t, err)
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	require.EqualValues(t, 500000, team.SharedQuota)
	_ = common.QuotaPerUnit
}
