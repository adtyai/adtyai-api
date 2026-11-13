package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestTeamTopupStartsEpayWithoutGift(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	previousPrice := operation_setting.Price
	operation_setting.Price = 7.2
	t.Cleanup(func() { operation_setting.Price = previousPrice })
	leader := referralUser(t, db, "team-epay-leader")
	created, err := CreateAdtyTeam(leader.Id, 0, "Pay", decimal.NewFromInt(1))
	require.NoError(t, err)
	seedEpayChannel(t, db, model.AdtyEpayChannel{
		Name: "team-pay", GatewayURL: "https://pay.example.com", MerchantId: "m1",
		OrderMode: model.AdtyEpayOrderModeSubmit,
	}, alipay("submit"))

	order, err := StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "alipay", "team-epay-1", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, created.Team.Id, order.TeamId)
	require.True(t, order.BonusUSD.IsZero())
	require.True(t, order.BonusRate.IsZero())
	require.True(t, order.PromoBonusUSD.IsZero())
	require.EqualValues(t, 500000, order.CreditQuota)
	require.Equal(t, "CNY", order.PayCurrency)
	require.True(t, order.PayAmount.Equal(decimal.RequireFromString("7.20")))
	require.Contains(t, order.PayURL, "https://pay.example.com/submit.php")
	require.Equal(t, model.PaymentProviderAdtyEpay, order.Provider)
	require.Equal(t, "alipay", order.Method)

	replay, err := StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "alipay", "team-epay-1", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, order.TradeNo, replay.TradeNo)
}
