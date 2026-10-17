package system_setting

// ServerAddress is the public API base URL used for links/callbacks when
// CustomCallbackAddress is empty. Default is empty so a fresh install never
// silently points notify_url at a brand production domain (audit P2-1).
// Production MUST set ServerAddress or CustomCallbackAddress via options.
var ServerAddress = ""
var TaskPublicAddress = ""
var WorkerUrl = ""
var WorkerValidKey = ""
var WorkerAllowHttpImageRequestEnabled = false

func EnableWorker() bool {
	return WorkerUrl != ""
}
