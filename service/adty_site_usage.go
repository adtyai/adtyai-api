package service

import (
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// 后台的名额用量（任务 11.3、11.8、11.9）：Resend 发件域名用了多少、失败多少、最近一次因名额满被拒的时间；
// 每个 Turnstile widget 登记了几个域名（上限 10），剩余少于 5 个时提醒。只读本地表，不调 Resend 和 Cloudflare。

// AdtyTurnstileLowSlots 是剩余名额提醒的阈值（方案 11.8：剩余少于 5 个时提醒）。
const AdtyTurnstileLowSlots = 5

// adtySenderQuotaFullAt 是最近一次建发件域名因 Resend 名额满被拒的时间（Unix 秒）。
// 首期单实例部署（执行层决定 10），进程内记录即可；重启后清零。
var adtySenderQuotaFullAt atomic.Int64

// adtyNoteSenderQuotaFull 在 adtyResendUserError 判断为名额满时调用。
func adtyNoteSenderQuotaFull() {
	adtySenderQuotaFullAt.Store(time.Now().Unix())
}

// AdtySenderDomainUsage 是 Resend 发件域名的用量。
type AdtySenderDomainUsage struct {
	InUse       int64 `json:"in_use"`   // pending + verified：占着 Resend 的域名名额
	Verified    int64 `json:"verified"` // 其中已验证、正在用来发信的
	Failed      int64 `json:"failed"`   // Resend 验证失败的（仍占名额，站长删掉重加前一直占着）
	Disabled    int64 `json:"disabled"` // 已停用、已从 Resend 删掉的
	QuotaFullAt int64 `json:"quota_full_at"`
}

// AdtyTurnstileWidgetUsage 是一个 widget 的用量。
type AdtyTurnstileWidgetUsage struct {
	Id      string `json:"id"`
	SiteKey string `json:"site_key"`
	Used    int    `json:"used"`
	Limit   int    `json:"limit"`
}

// AdtyResellerUsage 是后台设置页显示的两项用量。
type AdtyResellerUsage struct {
	Sender            AdtySenderDomainUsage      `json:"sender"`
	Widgets           []AdtyTurnstileWidgetUsage `json:"widgets"`
	WidgetSlotsLeft   int                        `json:"widget_slots_left"`
	WidgetSlotsLow    bool                       `json:"widget_slots_low"`
	UnassignedDomains int64                      `json:"unassigned_domains"` // active 但没登记进任何 widget（名额满时）的自定义域名
}

// GetAdtyResellerUsage 汇总发件域名和 Turnstile widget 的用量。
// 第一个 widget 还要放主站域名和每个 root_domain（11.8），它们算进第一个 widget 的已用数。
func GetAdtyResellerUsage() (*AdtyResellerUsage, error) {
	usage := &AdtyResellerUsage{Widgets: []AdtyTurnstileWidgetUsage{}}
	var counts []struct {
		Status string
		Total  int64
	}
	if err := model.DB.Model(&model.AdtySiteSenderDomain{}).Select("status, COUNT(*) AS total").Group("status").Scan(&counts).Error; err != nil {
		return nil, err
	}
	for _, row := range counts {
		switch row.Status {
		case model.AdtySenderDomainPending:
			usage.Sender.InUse += row.Total
		case model.AdtySenderDomainVerified:
			usage.Sender.InUse += row.Total
			usage.Sender.Verified += row.Total
		case model.AdtySenderDomainFailed:
			usage.Sender.Failed += row.Total
			usage.Sender.InUse += row.Total
		case model.AdtySenderDomainDisabled:
			usage.Sender.Disabled += row.Total
		}
	}
	usage.Sender.QuotaFullAt = adtySenderQuotaFullAt.Load()

	var assigned []struct {
		TurnstileWidget string
		Total           int
	}
	if err := model.DB.Model(&model.AdtySiteDomain{}).Select("turnstile_widget, COUNT(*) AS total").
		Where("status = ? AND turnstile_widget <> ''", model.AdtySiteDomainActive).Group("turnstile_widget").Scan(&assigned).Error; err != nil {
		return nil, err
	}
	byWidget := map[string]int{}
	for _, row := range assigned {
		byWidget[row.TurnstileWidget] = row.Total
	}
	if err := model.DB.Model(&model.AdtySiteDomain{}).Where("status = ? AND turnstile_widget = ''", model.AdtySiteDomainActive).
		Count(&usage.UnassignedDomains).Error; err != nil {
		return nil, err
	}
	cfg := resellerSetting()
	platformHosts := len(cfg.RootDomains)
	if adtyMainHost() != "" {
		platformHosts++
	}
	for i, widget := range cfg.TurnstileWidgets {
		used := byWidget[widget.Id]
		if i == 0 {
			used += platformHosts
		}
		usage.Widgets = append(usage.Widgets, AdtyTurnstileWidgetUsage{Id: widget.Id, SiteKey: widget.SiteKey, Used: used, Limit: adtyTurnstileDomainsPerWidget})
		usage.WidgetSlotsLeft += max(adtyTurnstileDomainsPerWidget-used, 0)
	}
	usage.WidgetSlotsLow = len(cfg.TurnstileWidgets) > 0 && usage.WidgetSlotsLeft < AdtyTurnstileLowSlots
	return usage, nil
}
