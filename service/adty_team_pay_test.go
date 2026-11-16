package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
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

func TestTeamTopupStartsExistingBEpusdtWithoutGift(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status_code":200,"message":"success","data":{"trade_id":"tid-team","order_id":"team","amount":"1","actual_amount":"1.01","token":"TAddr","expiration_time":900,"status":1,"payment_url":"https://pay.example/usdt"}}`))
	}))
	t.Cleanup(server.Close)
	cfg := setting.GetBEpusdtSetting()
	saved := *cfg
	cfg.Enabled, cfg.BaseURL, cfg.Secret, cfg.TradeTypes = true, server.URL, "team-bepusdt-secret", []string{"usdt.trc20"}
	t.Cleanup(func() { *cfg = saved })

	leader := referralUser(t, db, "team-usdt-leader")
	created, err := CreateAdtyTeam(leader.Id, 0, "USDT", decimal.NewFromInt(1))
	require.NoError(t, err)
	order, err := StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "usdt.trc20", "team-usdt-1", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, created.Team.Id, order.TeamId)
	require.Equal(t, model.PaymentProviderAdtyBEpusdt, order.Provider)
	require.Equal(t, "usdt.trc20", order.Method)
	require.True(t, order.BonusUSD.IsZero())
	require.True(t, order.BonusRate.IsZero())
	require.True(t, order.PromoBonusUSD.IsZero())
	require.EqualValues(t, 500000, order.CreditQuota)
	require.Equal(t, "TAddr", order.PayAddress)
	require.Equal(t, "https://pay.example/usdt", order.PayURL)

	replay, err := StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "usdt.trc20", "team-usdt-1", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, order.TradeNo, replay.TradeNo)
}
