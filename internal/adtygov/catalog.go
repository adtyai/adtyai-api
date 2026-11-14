package adtygov

// Official H.24 process capability IDs landed in this package.
const (
	CapInventory     = "ADTY-24-01"
	CapRuleMapping   = "ADTY-24-02"
	CapGlobalReview  = "ADTY-24-03"
	CapIssueManifest = "ADTY-24-04"
	CapRCAFix        = "ADTY-24-05"
	CapRetiredChain  = "ADTY-24-06"
	CapCleanup       = "ADTY-24-07"
	CapRereview      = "ADTY-24-08"
	CapSmoke         = "ADTY-24-09"
	CapScale         = "ADTY-24-10"
)

// LandedCapabilities are the official process tools this package provides.
// Landing them does not mean a full review, cleanup campaign, Phase D/E,
// or a claimed-closed retirement already ran. ADTY-24-06 is landed as an
// inventory; it is not claimed closed.
var LandedCapabilities = []string{
	CapInventory,
	CapRuleMapping,
	CapGlobalReview,
	CapIssueManifest,
	CapRCAFix,
	CapRetiredChain,
	CapCleanup,
	CapRereview,
}

// NotClaimedCapabilities must not be reported closed by this H.24 landing.
var NotClaimedCapabilities = []string{
	CapRetiredChain,
	CapSmoke,
	CapScale,
}

// EntryPoint is a concrete in-repo artifact for one official H.24 ID.
type EntryPoint struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Command       string `json:"command,omitempty"`
	Artifact      string `json:"artifact,omitempty"`
	Landed        bool   `json:"landed"`
	ClaimedClosed bool   `json:"claimed_closed"`
}

// OfficialEntryPoints returns the landed H.24 capabilities and explicitly
// lists 24-06, 24-09, and 24-10 as not claimed closed. ADTY-24-06 is landed
// because the retirement-chain inventory now lives in the repo.
func OfficialEntryPoints() []EntryPoint {
	return []EntryPoint{
		{ID: CapInventory, Title: "全仓源码资产盘点", Command: "adtygov enumerate-units", Artifact: "internal/adtygov.EnumerateUnits", Landed: true},
		{ID: CapRuleMapping, Title: "全仓业务规则映射", Command: "adtygov map-rules", Artifact: "internal/adtygov.OfficialRuleMappings", Landed: true},
		{ID: CapGlobalReview, Title: "全局代码审查", Command: "adtygov review-record", Artifact: "internal/adtygov.NewReviewRun", Landed: true},
		{ID: CapIssueManifest, Title: "全项目问题清单管理", Command: "adtygov validate-issues", Artifact: "internal/adtygov.ValidateIssueManifest", Landed: true},
		{ID: CapRCAFix, Title: "缺陷根因分析与修复", Command: "adtygov record-rca", Artifact: "internal/adtygov.ValidateRCA", Landed: true},
		{ID: CapRetiredChain, Title: "退役功能完整调用链清理", Command: "adtygov retired-chains", Artifact: "internal/adtygov.OfficialRetiredChainInventory", Landed: true, ClaimedClosed: false},
		{ID: CapCleanup, Title: "重复代码与无效依赖清理", Command: "adtygov cleanup-campaign", Artifact: "internal/adtygov.OfficialCleanupCampaign", Landed: true},
		{ID: CapRereview, Title: "修复后全项目重新审查", Command: "adtygov rereview-record", Artifact: "internal/adtygov.NewRereviewEntry", Landed: true},
		{ID: CapSmoke, Title: "全局审查清零后的完整冒烟测试", ClaimedClosed: false, Landed: false},
		{ID: CapScale, Title: "大规模用户/分站/团队测试及最终验收", ClaimedClosed: false, Landed: false},
	}
}

// HasLandedEntry reports whether an official in-repo entry point exists.
func HasLandedEntry(id string) bool {
	for _, item := range OfficialEntryPoints() {
		if item.ID == id {
			return item.Landed && item.Artifact != ""
		}
	}
	return false
}

// ClaimedClosed reports whether this landing claims an H.24 ID is closed.
// 24-06, 24-09, and 24-10 are never claimed closed here.
func ClaimedClosed(id string) bool {
	for _, item := range OfficialEntryPoints() {
		if item.ID == id {
			return item.ClaimedClosed
		}
	}
	return false
}
