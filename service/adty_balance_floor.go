package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// 余额下限：请求前余额不够就拒绝；结算补扣最多扣到 0，不产生负余额。
// 没扣到的差额记入 adty_billing_holds，并暂停该用户新的付费请求，直到财务核查。
// 差额同时记在消费日志的 admin_info.shortfall_quota。不从其他账户或站长保证金补扣。

// AdtyBillingShortfallContextKey 存放上一次扣费产生、还没写进消费日志的差额。
const AdtyBillingShortfallContextKey = "adty_billing_shortfall"

const adtyShortfallAdminKey = "shortfall_quota"

// adtyNoteBillingShortfall 在扣费之后调用：把这次扣费新增的差额（RelayInfo.BillingShortfall 的增量）
// 记进 gin 上下文，交给紧接着的 RecordConsumeLog 钩子写日志。
func adtyNoteBillingShortfall(c *gin.Context, relayInfo *relaycommon.RelayInfo, before int) {
	if c == nil || relayInfo == nil {
		return
	}
	if added := relayInfo.BillingShortfall - before; added > 0 {
		c.Set(AdtyBillingShortfallContextKey, c.GetInt(AdtyBillingShortfallContextKey)+added)
		_ = model.RecordAdtyBillingHold(relayInfo.UserId, relayInfo.RequestId, added)
	}
}

// adtyTakeBillingShortfall 由消费日志钩子调用：取出差额写进 admin_info 并清零，一笔差额只记一条日志。
func adtyTakeBillingShortfall(c *gin.Context, params *model.RecordConsumeLogParams) {
	if c == nil || params == nil {
		return
	}
	shortfall := c.GetInt(AdtyBillingShortfallContextKey)
	if shortfall <= 0 {
		return
	}
	c.Set(AdtyBillingShortfallContextKey, 0)
	if params.Other == nil {
		params.Other = model.NewLogOther()
	}
	params.Other.SetAdmin(adtyShortfallAdminKey, shortfall)
}

// adtyLoggedShortfall 读日志 admin_info 里的差额（消费日志和任务差额日志都用这个键）。
func adtyLoggedShortfall(other *model.LogOther) int64 {
	adminInfo, _ := other.Snapshot()["admin_info"].(map[string]any)
	switch value := adminInfo[adtyShortfallAdminKey].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(common.QuotaFromFloat(value))
	}
	return 0
}
