package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func TestCreateAffWithdrawalRejectsMismatchedAddressBeforeFreeze(t *testing.T) {
	_, err := CreateAffWithdrawal(1, AffWithdrawCreateInput{
		AmountQuota: 1000,
		IdemKey:     "idem-bad-addr",
		PayMethod:   AffPayMethodUSDT,
		PayAccount:  "0x0000000000000000000000000000000000000001",
		UsdtChain:   "trc20",
	})
	require.ErrorIs(t, err, setting.ErrUsdtPayoutAddressInvalid)
}
