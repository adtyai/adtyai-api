package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestSiteCustomerTopupInfoUsesSiteBonus(t *testing.T) {
	f := newOwnerFixture(t)
	confirmRouterCompliance(t)
	setRouterBonusRates(t)
	customer := f.customer(t, f.site, "site-payer", "payer@example.com")
	token := sessionTokenFor(t, customer)
	main := adtySessionToken(t, f.db, "main-payer", 1)

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("bonus_rates", `{"100":4}`).Error)
	service.InvalidateAdtySiteCache()
	within := siteTopupInfo(t, f.engine, token)
	require.Equal(t, 4.0, within.BonusRates["100"])
	require.Equal(t, "4", siteTopupQuoteBonus(t, f.engine, token))

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("bonus_rates", `{"100":8}`).Error)
	service.InvalidateAdtySiteCache()
	over := siteTopupInfo(t, f.engine, token)
	require.Equal(t, 4.5, over.BonusRates["100"])
	var tierRate float64
	for _, tier := range over.Tiers {
		if tier.Amount == 100 {
			tierRate = tier.Rate
		}
	}
	require.Equal(t, 4.5, tierRate)
	require.Equal(t, "4.5", siteTopupQuoteBonus(t, f.engine, token))

	mainInfo := callAs(t, f.engine, main, http.MethodGet, "/api/adty/topup/info", "")
	mainBody := decodeAdtyData[struct {
		BonusRates map[string]float64 `json:"bonus_rates"`
	}](t, mainInfo.Body)
	require.Equal(t, 2.5, mainBody.BonusRates["100"])
}

type siteTopupInfoBody struct {
	Tiers []struct {
		Amount int     `json:"amount"`
		Rate   float64 `json:"rate"`
	} `json:"tiers"`
	BonusRates map[string]float64 `json:"bonus_rates"`
}

func siteTopupInfo(t *testing.T, engine *gin.Engine, token string) siteTopupInfoBody {
	t.Helper()
	info := callAs(t, engine, token, http.MethodGet, "/api/adty/topup/info", "")
	require.Equal(t, http.StatusOK, info.Code)
	return decodeAdtyData[siteTopupInfoBody](t, info.Body)
}

func siteTopupQuoteBonus(t *testing.T, engine *gin.Engine, token string) string {
	t.Helper()
	quote := callAs(t, engine, token, http.MethodPost, "/api/adty/topup/quote", `{"amount":100,"method":"usdt.trc20"}`)
	quoted := decodeAdtyData[struct {
		Quote struct {
			BonusUSD decimal.Decimal `json:"bonus_usd"`
		} `json:"quote"`
	}](t, quote.Body)
	return quoted.Quote.BonusUSD.String()
}
