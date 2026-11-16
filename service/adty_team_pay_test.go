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

	order, err := StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "alipay", 0, "team-epay-1", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, created.Team.Id, order.TeamId)
	require.True(t, order.BonusUSD.IsZero())
	require.True(t, order.BonusRate.IsZero())
	require.EqualValues(t, 500000, order.CreditQuota)
	require.Equal(t, "CNY", order.PayCurrency)
	require.True(t, order.PayAmount.Equal(decimal.RequireFromString("7.20")))
	require.Contains(t, order.PayURL, "https://pay.example.com/submit.php")
	require.Equal(t, model.PaymentProviderAdtyEpay, order.Provider)
	require.Equal(t, "alipay", order.Method)

	replay, err := StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "alipay", 0, "team-epay-1", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, order.TradeNo, replay.TradeNo)
}

// 负责人选了哪个易支付通道就只用哪个：这个通道下不了单就提示换一个支付方式，不会换到别的通道；
// 订单记下单时的显示名称。
func TestTeamTopupUsesOnlyTheChosenEpayChannel(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	previousPrice := operation_setting.Price
	operation_setting.Price = 7.2
	t.Cleanup(func() { operation_setting.Price = previousPrice })
	leader := referralUser(t, db, "team-chosen-leader")
	created, err := CreateAdtyTeam(leader.Id, 0, "Chosen", decimal.NewFromInt(1))
	require.NoError(t, err)
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":-1,"msg":"busy"}`))
	}))
	t.Cleanup(broken.Close)
	brokenChannel := seedEpayChannel(t, db, model.AdtyEpayChannel{
		Name: "team-broken", GatewayURL: broken.URL, MerchantId: "m-broken", Priority: 9,
	}, alipay("qrcode"))
	chosen := seedEpayChannel(t, db, model.AdtyEpayChannel{
		Name: "team-chosen", GatewayURL: "https://pay.example.com", MerchantId: "m-chosen",
		OrderMode: model.AdtyEpayOrderModeSubmit, Priority: 1,
	}, alipay("submit"))
	require.NoError(t, db.Model(&model.AdtyEpayChannelMethod{}).Where("channel_id = ?", chosen.Id).Update("display_name", "支付宝2").Error)

	_, err = StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "alipay", brokenChannel.Id, "team-chosen-1", "127.0.0.1")
	require.ErrorIs(t, err, ErrAdtyEpayChosenChannelUnavailable)
	var orders []model.AdtyTopupOrder
	require.NoError(t, db.Where("team_id = ?", created.Team.Id).Find(&orders).Error)
	for _, order := range orders {
		require.Equal(t, brokenChannel.Id, order.ChannelId, "a chosen channel never falls back to another one")
	}

	order, err := StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "alipay", chosen.Id, "team-chosen-2", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, chosen.Id, order.ChannelId)
	require.Equal(t, "支付宝2", order.MethodLabel)
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
	order, err := StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "usdt.trc20", 0, "team-usdt-1", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, created.Team.Id, order.TeamId)
	require.Equal(t, model.PaymentProviderAdtyBEpusdt, order.Provider)
	require.Equal(t, "usdt.trc20", order.Method)
	require.True(t, order.BonusUSD.IsZero())
	require.True(t, order.BonusRate.IsZero())
	require.EqualValues(t, 500000, order.CreditQuota)
	require.Equal(t, "TAddr", order.PayAddress)
	require.Equal(t, "https://pay.example/usdt", order.PayURL)

	replay, err := StartAdtyTeamTopupPayment(leader.Id, created.Team.Id, decimal.NewFromInt(1), "usdt.trc20", 0, "team-usdt-1", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, order.TradeNo, replay.TradeNo)
}
