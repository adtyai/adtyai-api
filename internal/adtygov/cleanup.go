package adtygov

// CleanupCampaign is the official ADTY-24-07 campaign artifact.
// Landing the capability does not execute a mass-delete or C2 cleanup.
type CleanupCampaign struct {
	CapabilityID string   `json:"capability_id"`
	Tools        []string `json:"tools"`
	Status       string   `json:"status"`
	Executed     bool     `json:"executed"`
	MassDeleted  bool     `json:"mass_deleted"`
}

// OfficialCleanupTools names already-present official inventory tools.
func OfficialCleanupTools() []string {
	return []string{
		"web knip (package.json script knip)",
		"go inventory campaign record",
	}
}

// OfficialCleanupCampaign returns the in-repo cleanup-campaign entry.
// Executed and MassDeleted stay false: this commit does not delete leftovers.
func OfficialCleanupCampaign() CleanupCampaign {
	return CleanupCampaign{
		CapabilityID: CapCleanup,
		Tools:        OfficialCleanupTools(),
		Status:       "capability_landed",
		Executed:     false,
		MassDeleted:  false,
	}
}
