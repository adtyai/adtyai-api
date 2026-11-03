package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestSiteCustomerTopupInfoUsesSiteBonus(t *testing.T) {
	f := newOwnerFixture(t)
	confirmRouterCompliance(t)
	setRouterBonusRates(t)
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("bonus_rates", `{"100":8}`).Error)
	service.InvalidateAdtySiteCache()

	customer := f.customer(t, f.site, "site-payer", "payer@example.com")
	token := sessionTokenFor(t, customer)
	main := adtySessionToken(t, f.db, "main-payer", 1)

	info := callAs(t, f.engine, token, http.MethodGet, "/api/adty/topup/info", "")
	require.Equal(t, http.StatusOK, info.Code)
	body := decodeAdtyData[struct {
		Tiers []struct {
			Amount int     `json:"amount"`
			Rate   float64 `json:"rate"`
		} `json:"tiers"`
		BonusRates map[string]float64 `json:"bonus_rates"`
	}](t, info.Body)
	require.Equal(t, 8.0, body.BonusRates["100"])
	var tierRate float64
	for _, tier := range body.Tiers {
		if tier.Amount == 100 {
			tierRate = tier.Rate
		}
	}
	require.Equal(t, 8.0, tierRate)

	quote := callAs(t, f.engine, token, http.MethodPost, "/api/adty/topup/quote", `{"amount":100,"method":"usdt.trc20"}`)
	quoted := decodeAdtyData[struct {
		Quote struct {
			BonusUSD decimal.Decimal `json:"bonus_usd"`
		} `json:"quote"`
	}](t, quote.Body)
	require.Equal(t, "8", quoted.Quote.BonusUSD.String())

	mainInfo := callAs(t, f.engine, main, http.MethodGet, "/api/adty/topup/info", "")
	mainBody := decodeAdtyData[struct {
		BonusRates map[string]float64 `json:"bonus_rates"`
	}](t, mainInfo.Body)
	require.Equal(t, 2.5, mainBody.BonusRates["100"])
}
