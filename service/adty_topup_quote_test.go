package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// confirmAdtyCompliance 打开支付合规和 USD 显示，结束时还原。
func confirmAdtyCompliance(t *testing.T) {
	t.Helper()
	payment := operation_setting.GetPaymentSetting()
	oldConfirmed, oldVersion := payment.ComplianceConfirmed, payment.ComplianceTermsVersion
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	general := operation_setting.GetGeneralSetting()
	oldDisplay := general.QuotaDisplayType
	general.QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	oldPrice, oldMin, oldPerUnit := operation_setting.Price, operation_setting.MinTopUp, common.QuotaPerUnit
	operation_setting.Price, operation_setting.MinTopUp, common.QuotaPerUnit = 7.3, 1, 500000
	t.Cleanup(func() {
		payment.ComplianceConfirmed, payment.ComplianceTermsVersion = oldConfirmed, oldVersion
		general.QuotaDisplayType = oldDisplay
		operation_setting.Price, operation_setting.MinTopUp, common.QuotaPerUnit = oldPrice, oldMin, oldPerUnit
	})
}

// setAdtyBonusRates 设好加赠档位，结束时还原。加赠没有内置档位（任务 3.6），测试自己设。
func setAdtyBonusRates(t *testing.T, rates map[int]float64) {
	t.Helper()
	setting := adty_setting.GetTopupSetting()
	old := setting.BonusRates
	setting.BonusRates = rates
	t.Cleanup(func() { setting.BonusRates = old })
}

var testAdtyBonusRates = map[int]float64{10: 1, 30: 1.5, 100: 2.5, 100000: 8}

func TestAdtyBonusForNoTiersReturnsZero(t *testing.T) {
	setAdtyBonusRates(t, map[int]float64{})
	rate, bonus := AdtyBonusFor(100)
	assert.True(t, rate.IsZero())
	assert.True(t, bonus.IsZero())
}

func TestAdtyQuoteWithoutBonusSetting(t *testing.T) {
	confirmAdtyCompliance(t)
	setAdtyBonusRates(t, map[int]float64{})
	quote, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	assert.True(t, quote.BonusUSD.IsZero())
	assert.EqualValues(t, 50_000_000, quote.CreditQuota)
}

func TestAdtyBonusForTiers(t *testing.T) {
	setAdtyBonusRates(t, testAdtyBonusRates)
	cases := []struct {
		face        int64
		rate, bonus string
	}{
		{9, "0", "0"},
		{10, "1", "0.1"},
		{29, "1", "0.29"},
		{30, "1.5", "0.45"},
		{150, "2.5", "3.75"},
		{100000, "8", "8000"},
		{250000, "8", "20000"},
	}
	for _, tc := range cases {
		rate, bonus := AdtyBonusFor(tc.face)
		assert.Equal(t, tc.rate, rate.String(), tc.face)
		assert.Equal(t, tc.bonus, bonus.String(), tc.face)
	}
}

func TestAdtyQuoteUSDT(t *testing.T) {
	confirmAdtyCompliance(t)
	setAdtyBonusRates(t, testAdtyBonusRates)
	SetAdtyMethodBonusForTest(t, "usdt", true, "0.025")
	quote, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	assert.Equal(t, "100", quote.PayAmount.String())
	assert.Equal(t, AdtyCurrencyUSDT, quote.PayCurrency)
	assert.Equal(t, "2.5", quote.BonusUSD.String())
	assert.Equal(t, "0.025", quote.BonusRate.String())
	assert.EqualValues(t, 51_250_000, quote.CreditQuota)
}

func TestAdtyQuoteEpay(t *testing.T) {
	confirmAdtyCompliance(t)
	setAdtyBonusRates(t, testAdtyBonusRates)
	SetAdtyMethodBonusForTest(t, AdtyMethodAlipay, true, "0.025")
	quote, err := AdtyQuoteFor("default", 100, AdtyMethodAlipay, AdtyPromoInput{})
	require.NoError(t, err)
	assert.Equal(t, "730.00", quote.PayAmount.StringFixed(2))
	assert.Equal(t, AdtyCurrencyCNY, quote.PayCurrency)
	assert.Equal(t, "7.3", quote.PriceCNYPerUSD.String())
	assert.EqualValues(t, 51_250_000, quote.CreditQuota)
	wx, err := AdtyQuoteFor("default", 100, AdtyMethodWxpay, AdtyPromoInput{})
	require.NoError(t, err)
	assert.Equal(t, "730.00", wx.PayAmount.StringFixed(2))
	assert.Equal(t, AdtyCurrencyCNY, wx.PayCurrency)
}

func TestAdtyQuoteRejections(t *testing.T) {
	confirmAdtyCompliance(t)
	operation_setting.MinTopUp = 5
	_, err := AdtyQuoteFor("default", 4, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.Error(t, err, "below the minimum")
	_, err = AdtyQuoteFor("default", 10, "paypal", AdtyPromoInput{})
	require.Error(t, err, "unknown method")

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	_, err = AdtyQuoteFor("default", 10, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.Error(t, err, "tokens display mode")
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD

	// 2026-11-01 取消了支付合规确认：没确认也能报价。
	operation_setting.GetPaymentSetting().ComplianceConfirmed = false
	_, err = AdtyQuoteFor("default", 10, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err, "compliance confirmation is no longer required")
}

func TestAdtyQuoteUsesConfiguredRates(t *testing.T) {
	confirmAdtyCompliance(t)
	setAdtyBonusRates(t, map[int]float64{10: 3})
	SetAdtyMethodBonusForTest(t, "usdt", true, "0.03")
	quote, err := AdtyQuoteFor("default", 10, AdtyMethodUSDTBEP20, AdtyPromoInput{})
	require.NoError(t, err)
	assert.Equal(t, "0.3", quote.BonusUSD.String())
	assert.Equal(t, "0.03", quote.BonusRate.String())
}

func TestBonusRatesOptionRejectsInvalidKey(t *testing.T) {
	require.NoError(t, adty_setting.ValidateBonusRates(`{"10":1,"30":1.5,"100000":8}`))
	require.Error(t, adty_setting.ValidateBonusRates(`{"10.5":1}`))
	require.Error(t, adty_setting.ValidateBonusRates(`{"010":1}`))
	require.Error(t, adty_setting.ValidateBonusRates(`{"10":101}`))
	require.Error(t, adty_setting.ValidateBonusRates(`[1,2]`))
	require.NoError(t, adty_setting.ValidateBonusRates(`{}`), "an empty table turns the bonus off")
}

func TestEpayTimeoutSettingsValidated(t *testing.T) {
	setting := adty_setting.GetEpaySetting()
	oldChannel, oldTotal := setting.ChannelTimeoutSeconds, setting.TotalTimeoutSeconds
	setting.ChannelTimeoutSeconds, setting.TotalTimeoutSeconds = 5, 12
	t.Cleanup(func() { setting.ChannelTimeoutSeconds, setting.TotalTimeoutSeconds = oldChannel, oldTotal })

	cases := []struct {
		key, value string
		ok         bool
	}{
		{adty_setting.EpaySettingKeyChannelTimeout, "12", true},
		{adty_setting.EpaySettingKeyChannelTimeout, "13", false},
		{adty_setting.EpaySettingKeyChannelTimeout, "0", false},
		{adty_setting.EpaySettingKeyTotalTimeout, "30", true},
		{adty_setting.EpaySettingKeyTotalTimeout, "31", false},
		{adty_setting.EpaySettingKeyTotalTimeout, "4", false},
		{adty_setting.EpaySettingKeyTotalTimeout, "1.5", false},
	}
	for _, tc := range cases {
		err := adty_setting.ValidateEpayTimeout(tc.key, tc.value)
		assert.Equal(t, tc.ok, err == nil, "%s=%s", tc.key, tc.value)
	}
}
