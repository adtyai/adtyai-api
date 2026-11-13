package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestHostedQuoteKeepsFractionalProductFaceAndOriginalCurrency(t *testing.T) {
	confirmAdtyCompliance(t)
	previousRates := adty_setting.GetTopupSetting().BonusRates
	previousDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	previousUnit := common.QuotaPerUnit
	adty_setting.GetTopupSetting().BonusRates = map[int]float64{1: 10}
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		adty_setting.GetTopupSetting().BonusRates = previousRates
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplay
		common.QuotaPerUnit = previousUnit
	})
	quote, err := AdtyHostedQuoteForSite(nil, "default", decimal.NewFromInt(750000), decimal.RequireFromString("2.49"), "eur", AdtyPromoInput{})
	require.NoError(t, err)
	require.Equal(t, "1.5", quote.FaceUSD.String())
	require.True(t, quote.BonusUSD.IsZero())
	require.EqualValues(t, 750000, quote.CreditQuota)
	require.Equal(t, "2.49", quote.PayAmount.String())
	require.Equal(t, "EUR", quote.PayCurrency)
}
