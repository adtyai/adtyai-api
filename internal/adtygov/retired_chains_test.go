package adtygov

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOfficialRetiredChainInventoryIsLandedNotClaimedClosed(t *testing.T) {
	inv := OfficialRetiredChainInventory()
	require.Equal(t, CapRetiredChain, inv.CapabilityID)
	require.Equal(t, OfficialRetiredChainInventoryID, inv.InventoryID)
	require.Equal(t, "ADTY-24-06", inv.InventoryID)
	require.False(t, inv.ClaimedClosed, "ADTY-24-06 must not be claimed closed")
	require.False(t, inv.SmokeClaimed, "smoke must not be claimed passed")
	require.False(t, inv.ScaleClaimed, "scale must not be claimed passed")
	require.False(t, inv.CampaignExecuted)
	require.False(t, inv.MassDeleted)

	require.Equal(t, OfficialRetiredObjectIDs, officialRetiredIDs(inv.Objects))
	require.Len(t, inv.Objects, 12)

	allowed := map[string]struct{}{
		RetiredStatusItemizedC2Close:          {},
		RetiredStatusPreviouslyRemoved:        {},
		RetiredStatusOfficiallyRetainedCompat: {},
		RetiredStatusLeftoverNotInThisIssue:   {},
	}
	byID := map[string]RetiredObject{}
	for _, obj := range inv.Objects {
		require.NotEmpty(t, obj.ID)
		require.NotEmpty(t, obj.Title)
		require.NotEmpty(t, obj.Status)
		_, ok := allowed[obj.Status]
		require.True(t, ok, "unexpected status %q for %s", obj.Status, obj.ID)
		require.NotEmpty(t, obj.Notes)
		byID[obj.ID] = obj
	}

	require.Equal(t, RetiredStatusItemizedC2Close, byID["RETIRE-checkin"].Status)
	require.Contains(t, byID["RETIRE-checkin"].RelatedIssues, "ISS-R1-0062")
	require.Contains(t, byID["RETIRE-checkin"].KeepList, "checkin_quota")
	require.Contains(t, byID["RETIRE-checkin"].KeepList, "/api/user/checkin")

	require.Equal(t, RetiredStatusItemizedC2Close, byID["RETIRE-playground"].Status)
	require.Contains(t, byID["RETIRE-playground"].RelatedIssues, "ISS-R1-0064")
	require.Contains(t, byID["RETIRE-playground"].KeepList, "OfficialPlaygroundResidualCallSites")

	require.Equal(t, RetiredStatusItemizedC2Close, byID["RETIRE-desktop-electron"].Status)
	require.Contains(t, byID["RETIRE-desktop-electron"].RelatedIssues, "ISS-R1-0063")
	require.Contains(t, byID["RETIRE-desktop-electron"].Leftover, "window.electron")
	require.Contains(t, byID["RETIRE-desktop-electron"].Leftover, "database-step.tsx")

	require.Equal(t, RetiredStatusItemizedC2Close, byID["RETIRE-site-self-apply"].Status)
	require.Contains(t, byID["RETIRE-site-self-apply"].RelatedIssues, "ISS-R1-0065")
	require.Contains(t, byID["RETIRE-site-self-apply"].KeepList, "/api/adty/site/apply")
	require.Contains(t, byID["RETIRE-site-self-apply"].KeepList, "CreateAdtyAdminSite")

	for _, id := range []string{
		"RETIRE-subscription",
		"RETIRE-upstream-redemption",
		"RETIRE-upstream-invite-reward",
		"RETIRE-marketing",
		"RETIRE-model-deployment",
		"RETIRE-wallet-topup-card",
		"RETIRE-topup-discount-editor",
	} {
		require.Equal(t, RetiredStatusPreviouslyRemoved, byID[id].Status, id)
	}
	require.Equal(t, RetiredStatusOfficiallyRetainedCompat, byID["RETIRE-discount-editor"].Status)

	require.True(t, HasLandedEntry(CapRetiredChain), "ADTY-24-06 inventory must exist as a landed official entry")
	require.False(t, ClaimedClosed(CapRetiredChain))
	require.False(t, ClaimedClosed(CapSmoke))
	require.False(t, ClaimedClosed(CapScale))
	require.False(t, HasLandedEntry(CapSmoke))
	require.False(t, HasLandedEntry(CapScale))

	campaign := OfficialCleanupCampaign()
	require.False(t, campaign.Executed)
	require.False(t, campaign.MassDeleted)

	entry := NewRereviewEntry()
	require.False(t, entry.PhaseDExecuted)
	require.False(t, entry.ProjectConverged)
	require.False(t, entry.SmokeClaimed)
	require.False(t, entry.ScaleClaimed)
}

func officialRetiredIDs(objects []RetiredObject) []string {
	ids := make([]string, 0, len(objects))
	for _, obj := range objects {
		ids = append(ids, obj.ID)
	}
	return ids
}
