package service

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// Turnstile 和自定义域名（任务 11.8）。免费版每个 widget 最多 10 个域名、每个账号最多 20 个 widget。
// 主站域名和每个 root_domain 登记在 turnstile_widgets 的第一个里（一个域名连带它的子域名一起授权，
// 平台子域名不用逐个登记，部署时在 Cloudflare 后台配好）；每个 active 的自定义域名登记进还有空位的 widget。
// widget 的 PUT 是整体替换，先 GET 再原样带回 mode、name、region 等；PUT 的响应带 secret，不解析、不打日志。

// adtyTurnstileVerifyURL 是 siteverify 的地址，测试里换成假服务器。
var adtyTurnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// adtyTurnstileHTTP 调 siteverify 用，官方示例超时 10 秒。
var adtyTurnstileHTTP = &http.Client{Timeout: 10 * time.Second}

const adtyTurnstileMaxTokenLength = 2048

// adtyTurnstileWidget 是 widget 详情里要原样带回 PUT 的字段。
type adtyTurnstileWidget struct {
	Sitekey        string   `json:"sitekey"`
	Name           string   `json:"name"`
	Domains        []string `json:"domains"`
	Mode           string   `json:"mode"`
	Region         string   `json:"region,omitempty"`
	BotFightMode   *bool    `json:"bot_fight_mode,omitempty"`
	ClearanceLevel string   `json:"clearance_level,omitempty"`
	Offlabel       *bool    `json:"offlabel,omitempty"`
	EphemeralId    *bool    `json:"ephemeral_id,omitempty"`
}

func adtyWidgetPath(account, widgetId string) string {
	return "/accounts/" + url.PathEscape(account) + "/challenges/widgets/" + url.PathEscape(widgetId)
}

func (c *AdtyCloudflareClient) getTurnstileWidget(ctx context.Context, account, widgetId string) (*adtyTurnstileWidget, error) {
	var out adtyTurnstileWidget
	err := c.do(ctx, http.MethodGet, adtyWidgetPath(account, widgetId), nil, &out)
	return &out, err
}

// putTurnstileWidget 整体替换 widget。响应里带 secret，所以 out 传 nil，不解析响应。
func (c *AdtyCloudflareClient) putTurnstileWidget(ctx context.Context, account, widgetId string, widget *adtyTurnstileWidget) error {
	body := map[string]any{"domains": widget.Domains, "mode": widget.Mode, "name": widget.Name}
	if widget.Region != "" {
		body["region"] = widget.Region
	}
	if widget.BotFightMode != nil {
		body["bot_fight_mode"] = *widget.BotFightMode
	}
	if widget.ClearanceLevel != "" {
		body["clearance_level"] = widget.ClearanceLevel
	}
	if widget.Offlabel != nil {
		body["offlabel"] = *widget.Offlabel
	}
	if widget.EphemeralId != nil {
		body["ephemeral_id"] = *widget.EphemeralId
	}
	return c.do(ctx, http.MethodPut, adtyWidgetPath(account, widgetId), body, nil)
}

// adtyTurnstileAssign 把一个自定义域名登记进还有空位的 widget，返回 widget id。名额全满时返回空串
// （这个域名照常能用，人机验证改在平台子域名上做）。已经在某个 widget 里的直接返回它。
func adtyTurnstileAssign(ctx context.Context, hostname string) (string, error) {
	cfg := resellerSetting()
	account := strings.TrimSpace(cfg.CloudflareAccountId)
	if account == "" || len(cfg.TurnstileWidgets) == 0 {
		return "", nil
	}
	for _, configured := range cfg.TurnstileWidgets {
		widget, err := AdtyCloudflare.getTurnstileWidget(ctx, account, configured.Id)
		if err != nil {
			return "", err
		}
		if slices.Contains(widget.Domains, hostname) {
			return configured.Id, nil
		}
		if len(widget.Domains) >= adtyTurnstileDomainsPerWidget {
			continue
		}
		widget.Domains = append(widget.Domains, hostname)
		if err := AdtyCloudflare.putTurnstileWidget(ctx, account, configured.Id, widget); err != nil {
			return "", err
		}
		return configured.Id, nil
	}
	return "", nil
}

// adtyTurnstileRemoveDomain 从 widget 的 domains 里去掉一个域名。
func adtyTurnstileRemoveDomain(ctx context.Context, widgetId, hostname string) error {
	account := strings.TrimSpace(resellerSetting().CloudflareAccountId)
	if account == "" {
		return nil
	}
	widget, err := AdtyCloudflare.getTurnstileWidget(ctx, account, widgetId)
	if err != nil {
		return err
	}
	if !slices.Contains(widget.Domains, hostname) {
		return nil
	}
	widget.Domains = slices.DeleteFunc(widget.Domains, func(domain string) bool { return domain == hostname })
	return AdtyCloudflare.putTurnstileWidget(ctx, account, widgetId, widget)
}

// AdtyTurnstileChoice 是当前请求用哪个 widget：SiteKey 给 /api/status，Secret 给 TurnstileCheck。
// 两个都为空时用上游的 TurnstileSiteKey、TurnstileSecretKey（主站）。
type AdtyTurnstileChoice struct {
	SiteKey string
	Secret  string
}

// AdtyTurnstileFor 按当前站点和请求的域名选 widget：自定义域名用它登记进的 widget；平台子域名用
// turnstile_widgets 的第一个；主站（site 为 nil）用上游的设置。
func AdtyTurnstileFor(site *model.AdtySite, requestHost string) AdtyTurnstileChoice {
	if site == nil {
		return AdtyTurnstileChoice{}
	}
	cfg := resellerSetting()
	if len(cfg.TurnstileWidgets) == 0 {
		return AdtyTurnstileChoice{}
	}
	widgetId := cfg.TurnstileWidgets[0].Id
	host := strings.ToLower(requestHost)
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	if !strings.EqualFold(host, AdtySiteDisplayHost(site)) {
		var domain model.AdtySiteDomain
		if model.DB.Select("turnstile_widget").Where("hostname = ? AND site_id = ? AND status = ?", host, site.Id, model.AdtySiteDomainActive).
			Limit(1).Find(&domain).RowsAffected > 0 && domain.TurnstileWidget != "" {
			widgetId = domain.TurnstileWidget
		}
	}
	for _, widget := range cfg.TurnstileWidgets {
		if widget.Id == widgetId {
			return AdtyTurnstileChoice{SiteKey: widget.SiteKey, Secret: cfg.TurnstileWidgetSecret[widget.Id]}
		}
	}
	return AdtyTurnstileChoice{}
}

// AdtyTurnstileVerify 调 siteverify。通过还要求返回的 hostname 是 expectHost（不看大小写）；
// token 超过 2048 字符直接拒，不发请求。网络错误返回 err，调用方按失败处理。
func AdtyTurnstileVerify(ctx context.Context, secret, token, remoteIP, expectHost string) (bool, error) {
	return AdtyTurnstileVerifyAt(ctx, adtyTurnstileVerifyURL, secret, token, remoteIP, expectHost)
}

// AdtyTurnstileVerifyAt 同 AdtyTurnstileVerify，siteverify 的地址由调用方给（middleware 的测试替换它）。
func AdtyTurnstileVerifyAt(ctx context.Context, verifyURL, secret, token, remoteIP, expectHost string) (bool, error) {
	if token == "" || len(token) > adtyTurnstileMaxTokenLength {
		return false, nil
	}
	form := url.Values{"secret": {secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := adtyTurnstileHTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var result struct {
		Success  bool   `json:"success"`
		Hostname string `json:"hostname"`
	}
	if err := common.DecodeJson(resp.Body, &result); err != nil {
		return false, err
	}
	if !result.Success {
		return false, nil
	}
	host := strings.ToLower(expectHost)
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	return host == "" || strings.EqualFold(result.Hostname, host), nil
}
