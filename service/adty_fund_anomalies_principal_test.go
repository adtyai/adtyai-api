package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// ISS-R4-0097: the fund reconciliation principal must use the logical wallet
// (quota + quota_pending), the same definition model.userWalletBalance uses.
func TestDetectAdtyFundAnomaliesUsesLogicalPrincipal(t *testing.T) {
	t.Run("pending debit is not a gap", func(t *testing.T) {
		db := prepareSiteDB(t)
		_, site := openActiveSite(t, db, "g134-logical")
		customer := siteCustomer(t, db, site, "g134-logical-buyer")

		// The whole charge is still in quota_pending: raw 1,000,000 has not been
		// reduced yet, so the logical balance is 0 and matches the operating wallet.
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).
			Updates(map[string]any{"quota": 1_000_000, "quota_pending": -1_000_000}).Error)

		items, err := detectExistingAdtyFundAnomalies(site.Id)
		require.NoError(t, err)
		for _, item := range items {
			require.NotEqual(t, AdtyAnomalyBalanceLedgerGap, item.Kind,
				"a pending debit must not be reported as a fund gap: %s", item.Evidence)
		}
	})

	t.Run("real gap is still reported", func(t *testing.T) {
		db := prepareSiteDB(t)
		_, site := openActiveSite(t, db, "g134-real-gap")
		customer := siteCustomer(t, db, site, "g134-real-gap-buyer")

		// Nothing pending: the full raw balance is unexplained by the operating wallet.
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).
			Updates(map[string]any{"quota": 1_000_000, "quota_pending": 0}).Error)

		items, err := detectExistingAdtyFundAnomalies(site.Id)
		require.NoError(t, err)
		var gap *adtyFundAnomalyItem
		for i := range items {
			if items[i].Kind == AdtyAnomalyBalanceLedgerGap {
				gap = &items[i]
			}
		}
		require.NotNil(t, gap, "an operating wallet short of the customer principal is a real gap")
		require.Equal(t, "-2", gap.Difference)
		require.Contains(t, gap.Evidence, `"customer_principal":"2"`)
	})
}
