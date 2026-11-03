package authz

// 二开的后台权限项（任务 2.6）。资源名带 adty_ 前缀，避免和上游以后加的资源重名。
// 全部不设 DefaultRoles：普通管理员默认没有权限，由超级管理员逐项开启。
// 不在 init() 里登记：上游测试对权限矩阵做完全相等比较，只列了上游的三类资源。
const (
	ResourceAdtyUser     = "adty_user"
	ResourceAdtyLog      = "adty_log"
	ResourceAdtyModel    = "adty_model"
	ResourceAdtyFinance  = "adty_finance"
	ResourceAdtyReferral = "adty_referral"
	ResourceAdtyPayment  = "adty_payment"
	// ResourceAdtyMarketing 是营销（P12）：查看、编辑（设置、草稿、优惠码）、发送群发。
	ResourceAdtyMarketing = "adty_marketing"
	ResourceAdtySite      = "adty_site"

	ActionGroup        = "group"
	ActionQuota        = "quota"
	ActionDelete       = "delete"
	ActionManualCredit = "manual_credit"
	ActionRefund       = "refund"
	ActionReview       = "review"
	ActionPayout       = "payout"
	ActionAdjust       = "adjust"
	ActionSettings     = "settings"
	ActionSend         = "send"
)

var (
	AdtyUserRead   = Permission{Resource: ResourceAdtyUser, Action: ActionRead}
	AdtyUserWrite  = Permission{Resource: ResourceAdtyUser, Action: ActionWrite}
	AdtyUserGroup  = Permission{Resource: ResourceAdtyUser, Action: ActionGroup}
	AdtyUserQuota  = Permission{Resource: ResourceAdtyUser, Action: ActionQuota}
	AdtyUserDelete = Permission{Resource: ResourceAdtyUser, Action: ActionDelete}

	AdtyLogRead = Permission{Resource: ResourceAdtyLog, Action: ActionRead}

	AdtyModelRead  = Permission{Resource: ResourceAdtyModel, Action: ActionRead}
	AdtyModelWrite = Permission{Resource: ResourceAdtyModel, Action: ActionWrite}

	AdtyFinanceRead         = Permission{Resource: ResourceAdtyFinance, Action: ActionRead}
	AdtyFinanceManualCredit = Permission{Resource: ResourceAdtyFinance, Action: ActionManualCredit}
	AdtyFinanceRefund       = Permission{Resource: ResourceAdtyFinance, Action: ActionRefund}

	AdtyReferralRead     = Permission{Resource: ResourceAdtyReferral, Action: ActionRead}
	AdtyReferralReview   = Permission{Resource: ResourceAdtyReferral, Action: ActionReview}
	AdtyReferralPayout   = Permission{Resource: ResourceAdtyReferral, Action: ActionPayout}
	AdtyReferralAdjust   = Permission{Resource: ResourceAdtyReferral, Action: ActionAdjust}
	AdtyReferralSettings = Permission{Resource: ResourceAdtyReferral, Action: ActionSettings}

	AdtyPaymentRead    = Permission{Resource: ResourceAdtyPayment, Action: ActionRead}
	AdtyPaymentOperate = Permission{Resource: ResourceAdtyPayment, Action: ActionOperate}

	AdtyMarketingRead  = Permission{Resource: ResourceAdtyMarketing, Action: ActionRead}
	AdtyMarketingWrite = Permission{Resource: ResourceAdtyMarketing, Action: ActionWrite}
	AdtyMarketingSend  = Permission{Resource: ResourceAdtyMarketing, Action: ActionSend}

	AdtySiteRead     = Permission{Resource: ResourceAdtySite, Action: ActionRead}
	AdtySiteReview   = Permission{Resource: ResourceAdtySite, Action: ActionReview}
	AdtySiteAdjust   = Permission{Resource: ResourceAdtySite, Action: ActionAdjust}
	AdtySiteSettings = Permission{Resource: ResourceAdtySite, Action: ActionSettings}
)

var adtyResources = []ResourceDefinition{
	{
		Resource: ResourceAdtyUser,
		LabelKey: "Users",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View users", DescriptionKey: "View user lists, details and balance history."},
			{Action: ActionWrite, LabelKey: "Edit users", DescriptionKey: "Edit profiles, enable or disable, create users, reset 2FA and Passkeys, unbind sign-in methods and change passwords."},
			{Action: ActionGroup, LabelKey: "Change user groups", DescriptionKey: "Move users to another group. The group decides the price."},
			{Action: ActionQuota, LabelKey: "Adjust balances", DescriptionKey: "Add to or deduct from user balances."},
			{Action: ActionDelete, LabelKey: "Delete users", DescriptionKey: "Delete user accounts."},
		},
	},
	{
		Resource: ResourceAdtyLog,
		LabelKey: "Logs and statistics",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View all logs", DescriptionKey: "View every user's logs, tasks and site-wide statistics."},
		},
	},
	{
		Resource: ResourceAdtyModel,
		LabelKey: "Models",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View models", DescriptionKey: "View models, vendors and prefill groups."},
			{Action: ActionWrite, LabelKey: "Edit models", DescriptionKey: "Edit models, vendors and prefill groups."},
		},
	},
	{
		Resource: ResourceAdtyFinance,
		LabelKey: "Top-up finance",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View top-up orders", DescriptionKey: "View orders and recharge reconciliation."},
			{Action: ActionManualCredit, LabelKey: "Complete orders manually", DescriptionKey: "Credit an order after checking the payment."},
			{Action: ActionRefund, LabelKey: "Refund orders", DescriptionKey: "Take back the credited amount of a paid order."},
		},
	},
	{
		Resource: ResourceAdtyReferral,
		LabelKey: "Referrals",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View referrals", DescriptionKey: "View affiliates, commissions and withdrawals."},
			{Action: ActionReview, LabelKey: "Review referrals", DescriptionKey: "Approve or reject partner applications and withdrawals."},
			{Action: ActionPayout, LabelKey: "Mark payouts", DescriptionKey: "Mark withdrawals as paid and upload proof."},
			{Action: ActionAdjust, LabelKey: "Adjust referrals", DescriptionKey: "Adjust commissions, freeze settlement, set rates and disable affiliates."},
			{Action: ActionSettings, LabelKey: "Referral settings", DescriptionKey: "Change referral settings, including the withdrawal exchange rate."},
		},
	},
	{
		Resource: ResourceAdtyPayment,
		LabelKey: "Payment channels",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View payment channels", DescriptionKey: "View payment channel status."},
			{Action: ActionOperate, LabelKey: "Operate payment channels", DescriptionKey: "Enable, disable and test payment channels."},
		},
	},
	{
		Resource: ResourceAdtyMarketing,
		LabelKey: "Marketing",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View marketing", DescriptionKey: "View marketing settings, campaigns, statistics, promo codes and sign-up sources."},
			{Action: ActionWrite, LabelKey: "Edit marketing", DescriptionKey: "Change marketing settings, write campaigns and templates, set automated emails and promo codes."},
			{Action: ActionSend, LabelKey: "Send campaigns", DescriptionKey: "Send, schedule and cancel marketing campaigns."},
		},
	},
	{
		Resource: ResourceAdtySite,
		LabelKey: "Reseller sites",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View reseller sites", DescriptionKey: "View reseller sites, their customers and statistics."},
			{Action: ActionReview, LabelKey: "Review reseller sites", DescriptionKey: "Open sites, approve or reject applications, review flagged customers and reports."},
			{Action: ActionAdjust, LabelKey: "Adjust reseller sites", DescriptionKey: "Change a site's cost factor, markup limit and visibility, pause, resume or close sites."},
			{Action: ActionSettings, LabelKey: "Reseller settings", DescriptionKey: "Change reseller settings and manage site invite codes."},
		},
	},
}

// AdtyRegisterResources 登记二开的权限资源。重复调用不会重复登记。
func AdtyRegisterResources() {
	for _, resource := range adtyResources {
		if isKnownResource(resource.Resource) {
			continue
		}
		RegisterResource(resource)
	}
}

// AdtyClearAdminBaseline 清空所有资源动作的 DefaultRoles，下次 Init 时内置角色就没有默认权限。
// 返回的函数恢复原来的 registry，给测试用。
func AdtyClearAdminBaseline() (restore func()) {
	saved := make([]ResourceDefinition, len(registry))
	for i, resource := range registry {
		saved[i] = resource
		saved[i].Actions = append([]ActionDefinition(nil), resource.Actions...)
	}
	for i := range registry {
		actions := append([]ActionDefinition(nil), registry[i].Actions...)
		for j := range actions {
			actions[j].DefaultRoles = nil
		}
		registry[i].Actions = actions
	}
	return func() {
		registry = saved
	}
}
