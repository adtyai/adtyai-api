package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLedgerSelfAccountsForAffRefundKinds(t *testing.T) {
	require.Equal(t, []string{"aff"}, ledgerSelfAccounts("", "aff_refund_void"))
	require.Equal(t, []string{"aff"}, ledgerSelfAccounts("paid", "aff_refund_claw"))
	require.Equal(t, []string{"paid", "bonus"}, ledgerSelfAccounts("", "aff_refund_claw_paid"))
	require.Equal(t, []string{"bonus"}, ledgerSelfAccounts("bonus", ""))
	require.Equal(t, []string{"paid", "bonus"}, ledgerSelfAccounts("", "topup_paid"))
}
