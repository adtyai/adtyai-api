package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCreateTopupOrderRejectsMissingOrDisabledUsdtChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting.SetBepusdtEnabledChains(`["trc20"]`)
	t.Cleanup(func() {
		setting.SetBepusdtEnabledChains(`["trc20","bep20","polygon","xlayer"]`)
	})

	missing := postTopupOrder(t, `{"amount":10,"payment_method":"usdt"}`)
	require.Equal(t, http.StatusBadRequest, missing.Code)
	require.Contains(t, missing.Body.String(), "invalid_usdt_chain")

	unknown := postTopupOrder(t, `{"amount":10,"payment_method":"usdt","usdt_chain":"erc20"}`)
	require.Equal(t, http.StatusBadRequest, unknown.Code)
	require.Contains(t, unknown.Body.String(), "invalid_usdt_chain")

	disabled := postTopupOrder(t, `{"amount":10,"payment_method":"usdt","usdt_chain":"bep20"}`)
	require.Equal(t, http.StatusBadRequest, disabled.Code)
	require.Contains(t, disabled.Body.String(), "usdt_chain_disabled")
}

func TestUsdtTradeTypeUsesOrderChainNotGlobal(t *testing.T) {
	setting.SetBepusdtTradeType("usdt.trc20")
	got, err := usdtTradeTypeForOrder(&model.TopupOrder{UsdtChain: "polygon"})
	require.NoError(t, err)
	require.Equal(t, "usdt.polygon", got)

	_, err = usdtTradeTypeForOrder(&model.TopupOrder{UsdtChain: "erc20"})
	require.Error(t, err)

	got, err = usdtTradeTypeForOrder(&model.TopupOrder{})
	require.NoError(t, err)
	require.Equal(t, "usdt.trc20", got)
}

func postTopupOrder(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/topup/order", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	CreateTopupOrder(c)
	return w
}
