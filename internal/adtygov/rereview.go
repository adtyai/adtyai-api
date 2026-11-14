package adtygov

// RereviewEntry is the official ADTY-24-08 post-fix rereview entry point.
// Invoking or recording this entry does not run Phase D and does not mark
// the project converged.
type RereviewEntry struct {
	CapabilityID     string `json:"capability_id"`
	InvokedAs        string `json:"invoked_as"`
	Status           string `json:"status"`
	PhaseDExecuted   bool   `json:"phase_d_executed"`
	ProjectConverged bool   `json:"project_converged"`
	SmokeClaimed     bool   `json:"smoke_claimed"`
	ScaleClaimed     bool   `json:"scale_claimed"`
}

// NewRereviewEntry returns the official rereview invocation record.
func NewRereviewEntry() RereviewEntry {
	return RereviewEntry{
		CapabilityID:     CapRereview,
		InvokedAs:        "adtygov rereview-record",
		Status:           "capability_landed",
		PhaseDExecuted:   false,
		ProjectConverged: false,
		SmokeClaimed:     false,
		ScaleClaimed:     false,
	}
}
