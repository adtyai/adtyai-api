package setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsdtChainTradeTypesMatchBepusdt(t *testing.T) {
	require.Equal(t, "usdt.trc20", TradeTypeForUsdtChain("trc20"))
	require.Equal(t, "usdt.bep20", TradeTypeForUsdtChain(" BEP20 "))
	require.Equal(t, "usdt.polygon", TradeTypeForUsdtChain("polygon"))
	require.Equal(t, "usdt.xlayer", TradeTypeForUsdtChain("xlayer"))
	require.Equal(t, "", TradeTypeForUsdtChain(""))
	require.Equal(t, "", TradeTypeForUsdtChain("erc20"))
}

func TestEnabledUsdtChainsDefaultAllFour(t *testing.T) {
	restoreUsdtChains(t)
	bepusdtMu.Lock()
	BepusdtEnabledChains = ""
	bepusdtMu.Unlock()
	got := GetBepusdtEnabledChainIDs()
	require.Equal(t, []string{"trc20", "bep20", "polygon", "xlayer"}, got)
	require.Len(t, EnabledUsdtChains(), 4)
}

func TestRequireEnabledUsdtChain(t *testing.T) {
	restoreUsdtChains(t)
	SetBepusdtEnabledChains(`["polygon","trc20","trc20"]`)
	id, err := RequireEnabledUsdtChain(" trc20 ")
	require.NoError(t, err)
	require.Equal(t, "trc20", id)

	_, err = RequireEnabledUsdtChain("")
	require.ErrorIs(t, err, ErrUsdtChainRequired)
	_, err = RequireEnabledUsdtChain("erc20")
	require.ErrorIs(t, err, ErrUsdtChainInvalid)
	_, err = RequireEnabledUsdtChain("bep20")
	require.ErrorIs(t, err, ErrUsdtChainDisabled)

	// Stored order chain still maps after the admin turns it off.
	require.Equal(t, "usdt.bep20", TradeTypeForUsdtChain("bep20"))
}

func TestNormalizeBepusdtEnabledChainsJSON(t *testing.T) {
	restoreUsdtChains(t)
	normalized, err := NormalizeBepusdtEnabledChainsJSON(`["xlayer","trc20","xlayer"]`)
	require.NoError(t, err)
	require.Equal(t, `["trc20","xlayer"]`, normalized)

	normalized, err = NormalizeBepusdtEnabledChainsJSON(`[]`)
	require.NoError(t, err)
	require.Equal(t, `[]`, normalized)
	SetBepusdtEnabledChains(normalized)
	require.Empty(t, GetBepusdtEnabledChainIDs())

	_, err = NormalizeBepusdtEnabledChainsJSON(`["usdt.trc20"]`)
	require.ErrorIs(t, err, ErrBepusdtEnabledChainsInvalid)
	_, err = NormalizeBepusdtEnabledChainsJSON(`not-json`)
	require.ErrorIs(t, err, ErrBepusdtEnabledChainsInvalid)

	SetBepusdtEnabledChains(`not-json`)
	require.Empty(t, GetBepusdtEnabledChainIDs())
}

func TestValidateUsdtPayoutAddress(t *testing.T) {
	tron := "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb"
	evm := "0x0000000000000000000000000000000000000001"

	chain, address, err := ValidateUsdtPayoutAddress(" TRC20 ", " "+tron+" ")
	require.NoError(t, err)
	require.Equal(t, UsdtChainTRC20, chain)
	require.Equal(t, tron, address)

	for _, id := range []string{UsdtChainBEP20, UsdtChainPolygon, UsdtChainXLayer} {
		gotChain, gotAddress, err := ValidateUsdtPayoutAddress(id, evm)
		require.NoError(t, err)
		require.Equal(t, id, gotChain)
		require.Equal(t, evm, gotAddress)
		_, _, err = ValidateUsdtPayoutAddress(id, tron)
		require.ErrorIs(t, err, ErrUsdtPayoutAddressInvalid)
	}

	_, _, err = ValidateUsdtPayoutAddress(UsdtChainTRC20, evm)
	require.ErrorIs(t, err, ErrUsdtPayoutAddressInvalid)
	_, _, err = ValidateUsdtPayoutAddress("", tron)
	require.ErrorIs(t, err, ErrUsdtChainRequired)
	_, _, err = ValidateUsdtPayoutAddress("erc20", tron)
	require.ErrorIs(t, err, ErrUsdtChainInvalid)
	_, _, err = ValidateUsdtPayoutAddress(UsdtChainTRC20, "")
	require.ErrorIs(t, err, ErrUsdtPayoutAddressInvalid)
}

func restoreUsdtChains(t *testing.T) {
	t.Helper()
	bepusdtMu.Lock()
	prev := BepusdtEnabledChains
	bepusdtMu.Unlock()
	t.Cleanup(func() {
		bepusdtMu.Lock()
		BepusdtEnabledChains = prev
		bepusdtMu.Unlock()
	})
}
