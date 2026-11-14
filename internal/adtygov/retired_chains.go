package adtygov

// OfficialRetiredChainInventoryID is the in-repo ADTY-24-06 umbrella
// retirement-chain inventory. Landing this inventory records current
// source-tree status. It does not claim ADTY-24-06, 24-09, or 24-10
// closed, and it does not claim smoke or scale passed.
const OfficialRetiredChainInventoryID = "ADTY-24-06"

// Official statuses for one retired object after verifying the current tree.
const (
	RetiredStatusItemizedC2Close          = "itemized_c2_close"
	RetiredStatusPreviouslyRemoved        = "previously_removed"
	RetiredStatusOfficiallyRetainedCompat = "officially_retained_compat"
	RetiredStatusLeftoverNotInThisIssue   = "leftover_not_in_this_issue"
)

// OfficialRetiredObjectIDs is the official A.2 / G.16 retired-object set
// already used in the external review list. This package does not import
// that external file.
var OfficialRetiredObjectIDs = []string{
	"RETIRE-checkin",
	"RETIRE-playground",
	"RETIRE-subscription",
	"RETIRE-upstream-redemption",
	"RETIRE-upstream-invite-reward",
	"RETIRE-discount-editor",
	"RETIRE-marketing",
	"RETIRE-model-deployment",
	"RETIRE-desktop-electron",
	"RETIRE-site-self-apply",
	"RETIRE-wallet-topup-card",
	"RETIRE-topup-discount-editor",
}

// RetiredObject is one official retired feature as verified in the current tree.
type RetiredObject struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	RelatedIssues  []string `json:"related_issues,omitempty"`
	KeepList       string   `json:"keep_list,omitempty"`
	Leftover       string   `json:"leftover,omitempty"`
	VerifiedAbsent []string `json:"verified_absent,omitempty"`
	Notes          string   `json:"notes"`
}

// RetiredChainInventory is the official ADTY-24-06 umbrella artifact.
type RetiredChainInventory struct {
	CapabilityID     string          `json:"capability_id"`
	InventoryID      string          `json:"inventory_id"`
	Objects          []RetiredObject `json:"objects"`
	ClaimedClosed    bool            `json:"claimed_closed"`
	SmokeClaimed     bool            `json:"smoke_claimed"`
	ScaleClaimed     bool            `json:"scale_claimed"`
	CampaignExecuted bool            `json:"campaign_executed"`
	MassDeleted      bool            `json:"mass_deleted"`
}

// OfficialRetiredChainInventory returns the current-tree ADTY-24-06 inventory.
// ClaimedClosed, smoke, scale, and the cleanup campaign remain unclaimed.
func OfficialRetiredChainInventory() RetiredChainInventory {
	return RetiredChainInventory{
		CapabilityID:     CapRetiredChain,
		InventoryID:      OfficialRetiredChainInventoryID,
		Objects:          OfficialRetiredObjects(),
		ClaimedClosed:    false,
		SmokeClaimed:     false,
		ScaleClaimed:     false,
		CampaignExecuted: false,
		MassDeleted:      false,
	}
}

// OfficialRetiredObjects enumerates the official retired objects and the
// status verified against the current source tree. This is a governance
// record, not a second cleanup pass.
func OfficialRetiredObjects() []RetiredObject {
	return []RetiredObject{
		{
			ID:            "RETIRE-checkin",
			Title:         "签到奖励入口",
			Status:        RetiredStatusItemizedC2Close,
			RelatedIssues: []string{"ISS-R1-0062"},
			KeepList:      "Historical checkins, checkin_quota, and refund-source protection stay. GET/POST /api/user/checkin remain official 403 rejection entries. Leftover i18n keys and historical option rows stay for upgrade compatibility. Historical funds are not rewritten or deleted.",
			VerifiedAbsent: []string{
				"web/src/features/profile/components/checkin-calendar-card.tsx",
				"web/src/features/system-settings/general/checkin-settings-section.tsx",
			},
			Notes: "Live check-in credit path was retired in ISS-R1-0062. This inventory does not re-implement or amend that fix.",
		},
		{
			ID:            "RETIRE-playground",
			Title:         "旧 Playground 用户入口及专属中转",
			Status:        RetiredStatusItemizedC2Close,
			RelatedIssues: []string{"ISS-R1-0064"},
			KeepList:      "Official keep-list compatibility remains and is marked in OfficialPlaygroundResidualCallSites (inventory id RETIRE-playground-residuals): RelayInfo.IsPlayground billing/relay branches, /pg host and path classification, plugin reserved /pg and /playground, rejectRetiredFeaturePath JSON 404, historical chat.playground discard, leftover i18n keys, and the dev /pg proxy that forwards to the official 404. These sites are not a user feature.",
			VerifiedAbsent: []string{
				"web/src/features/playground/",
				"web/src/routes/_authenticated/playground/",
				"controller/playground.go",
				"relaykit/dto/playground.go",
			},
			Notes: "ISS-R1-0064 confirmed there is no user-reachable Playground entry and marked residual call sites. This inventory does not drop that keep-list or amend that fix.",
		},
		{
			ID:       "RETIRE-subscription",
			Title:    "订阅套餐系统",
			Status:   RetiredStatusPreviouslyRemoved,
			KeepList: "Existing subscription_* / user_subscriptions tables are not dropped; historical subscription rows stay as financial facts. Official OpenAI-compatible subscription surfaces that remain are not this retired product.",
			VerifiedAbsent: []string{
				"controller/subscription.go",
				"model/subscription.go",
				"web/src/features/subscriptions/",
				"web/src/routes/_authenticated/subscriptions/",
			},
			Notes: "Already absent per docs/adty/deleted-paths.txt and deleted-features.md (任务 1.2 / 1.3). No new cleanup pass.",
		},
		{
			ID:       "RETIRE-upstream-redemption",
			Title:    "上游旧兑换码实现",
			Status:   RetiredStatusPreviouslyRemoved,
			KeepList: "A historical redemptions table, if already present, is not auto-deleted. The official ADTY voucher (AdtyVoucher) is a current capability and is not this retired upstream path.",
			VerifiedAbsent: []string{
				"controller/redemption.go",
				"model/redemption.go",
				"web/src/features/redemption-codes/",
				"web/src/features/wallet/hooks/use-redemption.ts",
			},
			Notes: "Already absent per docs/adty/deleted-paths.txt and deleted-features.md (任务 1.4). No new cleanup pass.",
		},
		{
			ID:       "RETIRE-upstream-invite-reward",
			Title:    "上游邀请奖励",
			Status:   RetiredStatusPreviouslyRemoved,
			KeepList: "Historical invite or ledger rows stay. Official service/referral*.go and current site-owner referral are G.9 / A.13 capabilities, not the retired upstream affiliate-reward product. Retired self-serve site invite-code opening is tracked under RETIRE-site-self-apply, not here.",
			VerifiedAbsent: []string{
				"web/src/features/wallet/components/affiliate-rewards-card.tsx",
				"web/src/features/wallet/hooks/use-affiliate.ts",
				"web/src/features/wallet/lib/affiliate.ts",
			},
			Notes: "Already absent per docs/adty/deleted-paths.txt and deleted-features.md (任务 1.5). No new cleanup pass.",
		},
		{
			ID:       "RETIRE-discount-editor",
			Title:    "旧上游付费折扣编辑器及不符合规则的历史优惠入口",
			Status:   RetiredStatusOfficiallyRetainedCompat,
			KeepList: "The official keep list in deleted-features.md retains payment_setting.amount_discount because upstream code still reads the key; official quotes do not use it. Historical promo rows are not deleted. New promo writes remain rejected with ErrAdtyPromoRetired.",
			VerifiedAbsent: []string{
				"web/src/features/system-settings/integrations/amount-discount-dialog.tsx",
				"web/src/features/system-settings/integrations/amount-discount-visual-editor.tsx",
			},
			Notes: "The visual editor was previously removed (任务 4.3). This inventory records the retained compatibility key and does not start a new cleanup pass.",
		},
		{
			ID:       "RETIRE-marketing",
			Title:    "独立营销联系人/广播/营销自动化",
			Status:   RetiredStatusPreviouslyRemoved,
			KeepList: "Historical promo rows, audit event names, and old tables are not deleted. New databases do not create the retired marketing-only tables.",
			VerifiedAbsent: []string{
				"controller/adty_marketing.go",
				"model/adty_marketing_campaign.go",
				"service/adty_marketing_campaign.go",
				"setting/adty_setting/marketing.go",
				"web/src/features/adty-marketing/",
			},
			Notes: "Already absent per docs/adty/deleted-paths.txt and deleted-features.md. No new cleanup pass.",
		},
		{
			ID:       "RETIRE-model-deployment",
			Title:    "模型部署管理",
			Status:   RetiredStatusPreviouslyRemoved,
			KeepList: "Historical model_deployment.* persisted values and historical audit rows are not deleted.",
			VerifiedAbsent: []string{
				"controller/deployment.go",
				"pkg/ionet/",
				"web/src/features/models/hooks/use-model-deployment-settings.ts",
				"web/src/features/system-settings/integrations/ionet-deployment-settings-section.tsx",
			},
			Notes: "Already absent per docs/adty/deleted-paths.txt and deleted-features.md. No new cleanup pass.",
		},
		{
			ID:            "RETIRE-desktop-electron",
			Title:         "旧桌面客户端产品入口及构建发布链路",
			Status:        RetiredStatusItemizedC2Close,
			RelatedIssues: []string{"ISS-R1-0063"},
			KeepList:      "LICENSE, NOTICE, and THIRD-PARTY-LICENSES stay, with factual inventory wording after the electron/ tree was removed. The previously deleted upstream workflow .github/workflows/electron-build.yml stays on the official deleted-paths list.",
			Leftover:      "web/src/features/setup/components/database-step.tsx still reads a window.electron hook. That leftover is already recorded as a next-round finding from ISS-R1-0063 and is not fixed in this umbrella inventory.",
			VerifiedAbsent: []string{
				"electron/",
				".github/workflows/electron-build.yml",
			},
			Notes: "ISS-R1-0063 removed the leftover electron/ build and publish tree. This inventory does not amend that fix or treat the recorded window.electron hook as in-scope work.",
		},
		{
			ID:            "RETIRE-site-self-apply",
			Title:         "旧分站自助申请入口",
			Status:        RetiredStatusItemizedC2Close,
			RelatedIssues: []string{"ISS-R1-0065"},
			KeepList:      "POST /api/adty/site/apply stays the official 403 rejection entry (no site created, no invite consumed). Historical application rows and invite-code table/rows stay. Root ReviewAdtySiteApplication of already-existing pending applications stays. Official Root CreateAdtyAdminSite / OpenAdtySite(..., AdtySiteOpenedByAdmin) stays. Users cannot self-open through the old entry.",
			VerifiedAbsent: []string{
				"web/src/features/referral/components/site-apply-card.tsx",
				"web/src/features/adty-site-admin/components/invite-codes-panel.tsx",
			},
			Notes: "Self-serve and invite-code opening were hard-disabled in ISS-R1-0065. This inventory does not reopen or amend that fix, and does not change official Root create parameters.",
		},
		{
			ID:     "RETIRE-wallet-topup-card",
			Title:  "旧钱包充值卡",
			Status: RetiredStatusPreviouslyRemoved,
			VerifiedAbsent: []string{
				"web/src/features/wallet/components/recharge-form-card.tsx",
				"web/src/features/wallet/components/creem-products-section.tsx",
				"web/src/features/wallet/hooks/",
			},
			Notes: "Already absent per docs/adty/deleted-paths.txt (任务 3.4, replaced by features/wallet/adty/). Official payment routes were not retouched here. No new cleanup pass.",
		},
		{
			ID:       "RETIRE-topup-discount-editor",
			Title:    "旧充值折扣编辑器",
			Status:   RetiredStatusPreviouslyRemoved,
			KeepList: "The retained payment_setting.amount_discount compatibility key is recorded under RETIRE-discount-editor. Official quotes use the later bonus-rate editor, not this retired visual editor.",
			VerifiedAbsent: []string{
				"web/src/features/system-settings/integrations/amount-discount-dialog.tsx",
				"web/src/features/system-settings/integrations/amount-discount-visual-editor.tsx",
			},
			Notes: "Already absent per docs/adty/deleted-paths.txt and deleted-features.md (任务 4.3). No new cleanup pass.",
		},
	}
}
