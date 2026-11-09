package service

import (
	"encoding/json"
	"slices"

	"github.com/QuantumNous/new-api/common"
)

// 站长中心的敏感操作要重新验证身份（任务 11.6，按 AGENTS.md 的 OWASP 要求做敏感操作的再认证）：
// 改价、改加赠、送试用额度、删除自定义域名、新建修改删除优惠码、删除发件域名。用上游的二次验证（/api/verify 拿 proof，请求带 X-Security-Proof），
// 和 2FA、Passkey、密码同一套；proof 只能用一次，并绑定操作和目标，不能拿去做别的事。

// VerificationScopeAdtySiteOwnerManage 是站长敏感操作的验证范围。
const VerificationScopeAdtySiteOwnerManage = "site.owner.manage"

// 站长敏感操作的种类。
const (
	AdtySiteOwnerActionPricing       = "pricing"       // target：0 是全站定价，否则是客户 id
	AdtySiteOwnerActionBonus         = "bonus"         // target：站点 id
	AdtySiteOwnerActionTrial         = "trial"         // target：客户 id
	AdtySiteOwnerActionDomain        = "domain.delete" // target：自定义域名 id
	AdtySiteOwnerActionPromo         = "promo"         // target：优惠码 id，新建为 0
	AdtySiteOwnerActionSender        = "sender.delete" // target：站点 id
	AdtySiteOwnerActionStaffCreate   = "staff.create"  // target: site ID
	AdtySiteOwnerActionStaffUpdate   = "staff.update"  // target: membership ID
	AdtySiteOwnerActionStaffPassword = "staff.password"
	AdtySiteOwnerActionStaffDelete   = "staff.delete"
)

var adtySiteOwnerActions = []string{AdtySiteOwnerActionPricing, AdtySiteOwnerActionBonus, AdtySiteOwnerActionTrial, AdtySiteOwnerActionDomain, AdtySiteOwnerActionPromo, AdtySiteOwnerActionSender,
	AdtySiteOwnerActionStaffCreate, AdtySiteOwnerActionStaffUpdate, AdtySiteOwnerActionStaffPassword, AdtySiteOwnerActionStaffDelete}

// AdtySiteOwnerContext 是 proof 绑定的操作：哪种操作、对哪个目标。
type AdtySiteOwnerContext struct {
	Action string `json:"action"`
	Target int    `json:"target"`
}

func adtyIsSiteOwnerScope(scope string) bool {
	return scope == VerificationScopeAdtySiteOwnerManage
}

// adtyBindSiteOwnerOperation 校验并规范化站长操作的上下文（BindVerificationOperation 调用）。
func adtyBindSiteOwnerOperation(scope string, fields map[string]json.RawMessage, raw json.RawMessage) (any, bool) {
	if !adtyIsSiteOwnerScope(scope) {
		return nil, false
	}
	var context AdtySiteOwnerContext
	if len(fields) != 2 || common.Unmarshal(raw, &context) != nil || !slices.Contains(adtySiteOwnerActions, context.Action) || context.Target < 0 {
		return nil, false
	}
	if context.Target == 0 && context.Action != AdtySiteOwnerActionPricing && context.Action != AdtySiteOwnerActionPromo {
		return nil, false
	}
	return context, true
}

// AdtySiteOwnerOperation 构造要验证的操作，controller 交给 middleware.RequireSecurityProof。
func AdtySiteOwnerOperation(action string, target int) VerificationOperation {
	payload, _ := common.Marshal(AdtySiteOwnerContext{Action: action, Target: target})
	return VerificationOperation{Scope: VerificationScopeAdtySiteOwnerManage, Context: payload}
}

func init() {
	// 访问令牌也可以做站长操作，但令牌要有钱包写权限（和站长接口登记的 wallet:write 一致）。
	accessTokenVerificationScopes[VerificationScopeAdtySiteOwnerManage] = "wallet:write"
}
