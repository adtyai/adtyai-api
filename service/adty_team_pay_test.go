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

type teamStripeProvider struct{ available bool }

func (p teamStripeProvider) Name() string { return model.PaymentProviderStripe }
func (p teamStripeProvider) Methods() []AdtyMethodInfo {
	return []AdtyMethodInfo{{Id: model.PaymentMethodStripe, Available: p.available}}
}
func (p teamStripeProvider) AvailableFor(method string, quote *AdtyQuote) bool {
	return p.available && method == model.PaymentMethodStripe && quote != nil && quote.PayAmount.IsPositive()
}
func (p teamStripeProvider) Cancel(*model.AdtyTopupOrder) error { return nil }

func TestTeamTopupStartsExistingStripeCheckoutWithoutGift(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	previous := AdtyPaymentProviderByName(model.PaymentProviderStripe)
	RegisterAdtyPaymentProvider(teamStripeProvider{available: true})
	t.Cleanup(func() {
		if previous != nil {
			RegisterAdtyPaymentProvider(previous)
			return
		}
		RegisterAdtyPaymentProvider(teamStripeProvider{})
	})
	payment := operation_setting.GetPaymentSetting()
	previousConfirmed, previousVersion := payment.ComplianceConfirmed, payment.ComplianceTermsVersion
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		payment.ComplianceConfirmed = previousConfirmed
		payment.ComplianceTermsVersion = previousVersion
	})
	leader := referralUser(t, db, "team-stripe-leader")
	created, err := CreateAdtyTeam(leader.Id, 0, "Stripe", decimal.NewFromInt(1))
	require.NoError(t, err)
	calls := 0
	order, err := StartAdtyTeamStripeTopup(leader.Id, created.Team.Id, decimal.NewFromInt(1), decimal.RequireFromString("8.00"), "team-stripe-1", func(tradeNo string, money decimal.Decimal) (string, string, error) {
		calls++
		require.True(t, money.Equal(decimal.RequireFromString("8.00")))
		return "https://checkout.stripe.test/" + tradeNo, "cs_team", nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, created.Team.Id, order.TeamId)
	require.Equal(t, model.PaymentProviderStripe, order.Provider)
	require.Equal(t, model.PaymentMethodStripe, order.Method)
	require.True(t, order.BonusUSD.IsZero())
	require.True(t, order.BonusRate.IsZero())
	require.True(t, order.PromoBonusUSD.IsZero())
	require.EqualValues(t, 500000, order.CreditQuota)
	require.Equal(t, "https://checkout.stripe.test/"+order.TradeNo, order.PayURL)
	require.Equal(t, "cs_team", order.GatewayTradeId)

	replay, err := StartAdtyTeamStripeTopup(leader.Id, created.Team.Id, decimal.NewFromInt(1), decimal.RequireFromString("8.00"), "team-stripe-1", func(string, decimal.Decimal) (string, string, error) {
		calls++
		return "", "", nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, order.TradeNo, replay.TradeNo)
}

func TestTeamTopupStartsExistingBEpusdtWithoutGift(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	payment := operation_setting.GetPaymentSetting()
	previousConfirmed, previousVersion := payment.ComplianceConfirmed, payment.ComplianceTermsVersion
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		payment.ComplianceConfirmed = previousConfirmed
		payment.ComplianceTermsVersion = previousVersion
	})
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
