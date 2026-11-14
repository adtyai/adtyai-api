package adtygov

// OfficialPlaygroundResidualInventoryID is the in-repo G.16.4 mark for
// retired Playground leftovers. This inventory does not register a user
// feature and does not close ADTY-24-06, 24-09, or 24-10.
const OfficialPlaygroundResidualInventoryID = "RETIRE-playground-residuals"

// ResidualRetention is why an official keep-list Playground leftover remains.
type ResidualRetention string

const (
	RetainBillingRelayFlag ResidualRetention = "billing_relay_flag"
	RetainHostPathClass    ResidualRetention = "host_path_classification"
	RetainPluginReserved   ResidualRetention = "plugin_reserved_paths"
	RetainRetiredNotFound  ResidualRetention = "retired_json_404"
	RetainHistoricalConfig ResidualRetention = "historical_config_discard"
	RetainLegacyI18n       ResidualRetention = "leftover_i18n_keys"
)

// ResidualCallSite is one officially marked Playground leftover. UserFeature
// is always false: these sites are compatibility or rejection, not a product.
type ResidualCallSite struct {
	Path        string            `json:"path"`
	Symbol      string            `json:"symbol"`
	Retention   ResidualRetention `json:"retention"`
	Reason      string            `json:"reason"`
	UserFeature bool              `json:"user_feature"`
}

// OfficialPlaygroundResidualCallSites lists the residual call sites that
// remain after the Playground user entry was removed, and why each is kept.
func OfficialPlaygroundResidualCallSites() []ResidualCallSite {
	return []ResidualCallSite{
		{
			Path:      "relay/common/relay_info.go",
			Symbol:    "RelayInfo.IsPlayground",
			Retention: RetainBillingRelayFlag,
			Reason:    "Official keep-list internal compatibility flag still read by billing and task relay. Not a registered user feature.",
		},
		{
			Path:      "relay/common/relay_info.go",
			Symbol:    "GenRelayInfo /pg rewrite",
			Retention: RetainBillingRelayFlag,
			Reason:    "If a leftover /pg request reached GenRelayInfo, the official flag is set and the path is rewritten to /v1. No live /pg relay route is registered.",
		},
		{
			Path:      "service/quota.go",
			Symbol:    "IsPlayground branches",
			Retention: RetainBillingRelayFlag,
			Reason:    "Official keep-list billing compatibility that reads IsPlayground. No new billing policy is introduced.",
		},
		{
			Path:      "service/billing_session.go",
			Symbol:    "IsPlayground branches",
			Retention: RetainBillingRelayFlag,
			Reason:    "Official keep-list billing-session compatibility that reads IsPlayground.",
		},
		{
			Path:      "service/midjourney.go",
			Symbol:    "IsPlayground branch",
			Retention: RetainBillingRelayFlag,
			Reason:    "Official keep-list task-relay compatibility that reads IsPlayground.",
		},
		{
			Path:      "middleware/adty_site_context.go",
			Symbol:    "adtyAPIPathPrefixes /pg/",
			Retention: RetainHostPathClass,
			Reason:    "Official keep-list classification of the retired /pg namespace for closed-site and custom-domain handling. Not a registered user route.",
		},
		{
			Path:      "middleware/adty_site_context.go",
			Symbol:    "adtyIsRelayPath /pg/",
			Retention: RetainHostPathClass,
			Reason:    "Official keep-list host/path classification for the retired /pg namespace.",
		},
		{
			Path:      "relay/constant/relay_mode.go",
			Symbol:    "Path2RelayMode /pg/chat/completions",
			Retention: RetainHostPathClass,
			Reason:    "Keeps official path classification if a leftover /pg/chat/completions request reached relay mode selection. Does not register a user route.",
		},
		{
			Path:      "pkg/jsplugin/routing.go",
			Symbol:    "reserved /pg and /playground",
			Retention: RetainPluginReserved,
			Reason:    "Official keep-list plugin reserved paths. Plugins cannot claim the /pg namespace.",
		},
		{
			Path:      "router/web-router.go",
			Symbol:    "rejectRetiredFeaturePath",
			Retention: RetainRetiredNotFound,
			Reason:    "Official JSON 404 for /pg and subpaths. Beats the plugin dispatcher and is not a live relay route.",
		},
		{
			Path:      "router/main.go",
			Symbol:    "rejectRetiredFeaturePath wiring",
			Retention: RetainRetiredNotFound,
			Reason:    "Wires the official retired-path 404 into the root router.",
		},
		{
			Path:      "web/rsbuild.config.ts",
			Symbol:    "devProxy /pg",
			Retention: RetainRetiredNotFound,
			Reason:    "Development proxy forwards /pg to the backend official JSON 404. Not a product route.",
		},
		{
			Path:      "web/src/features/system-settings/maintenance/config.ts",
			Symbol:    "chat.playground discard",
			Retention: RetainHistoricalConfig,
			Reason:    "Historical chat.playground config is discarded on read and cannot reappear as a toggle.",
		},
		{
			Path:      "i18n/keys.go",
			Symbol:    "MsgDistributorInvalidPlayground",
			Retention: RetainLegacyI18n,
			Reason:    "Official keep-list leftover i18n key. Not a registered user feature.",
		},
		{
			Path:      "web/src/i18n/locales",
			Symbol:    "retired playground UI keys",
			Retention: RetainLegacyI18n,
			Reason:    "Official keep-list leftover locale keys for the retired Playground UI. Not a registered user feature.",
		},
	}
}
