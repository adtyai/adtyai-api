package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"html"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 站点品牌、系统邮件的发件人和站长发件域名（任务 11.3）。
// 链接和站名一律从站点表取（AdtySiteForUser、AdtySiteForRequest、AdtySiteBaseURL，见 adty_site_resolve.go），
// 不读 Host 头之外的请求数据（执行层决定 28）。

// AdtySiteBrand 是一个站点对外显示的品牌。主站返回平台的站名和地址。
type AdtySiteBrand struct {
	Name           string `json:"name"`
	LogoURL        string `json:"logo_url"`
	Tagline        string `json:"tagline"`
	Notice         string `json:"notice"`
	SupportContact string `json:"support_contact"`
	BaseURL        string `json:"base_url"`
}

// HTMLName 是写进邮件 HTML 的站名。站长填的是纯文本，进 HTML 前要转义。
func (b AdtySiteBrand) HTMLName() string { return html.EscapeString(b.Name) }

// AdtySiteBrandInput 是站长提交的品牌字段（11.6 的 PUT /api/adty/site/self 用）。
type AdtySiteBrandInput struct {
	Name           string `json:"name"`
	LogoURL        string `json:"logo_url"`
	Tagline        string `json:"tagline"`
	Notice         string `json:"notice"`
	SupportContact string `json:"support_contact"`
}

// 品牌字段的字数上限（按字符数，不是字节）。
const (
	adtyBrandNameMax    = 40
	adtyBrandTaglineMax = 80
	adtyBrandNoticeMax  = 500
	adtyBrandSupportMax = 200
	// AdtySiteAssetPrefix 是站长上传的 Logo 的地址前缀（文件在 /data/uploads/site-assets）。
	AdtySiteAssetPrefix = "/uploads/site-assets/"
)

var ErrAdtySiteBrandInvalid = common.NewMessage("Site name up to 40 characters, tagline up to 80, notice up to 500 and support contact up to 200; the logo must be an uploaded image")

// adtyPlainText 去掉控制字符和首尾空白；HTML 原样保留为文本，前端按纯文本渲染，进邮件时转义。
func adtyPlainText(value string, keepNewlines bool) string {
	value = strings.Map(func(r rune) rune {
		if r == '\n' && keepNewlines {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(value)
}

// NormalizeAdtySiteBrand 校验并整理站长提交的品牌：只是模板字段，不接受 HTML 以外的格式，也不渲染 HTML。
func NormalizeAdtySiteBrand(input AdtySiteBrandInput) (AdtySiteBrandInput, error) {
	out := AdtySiteBrandInput{
		Name:           adtyPlainText(input.Name, false),
		Tagline:        adtyPlainText(input.Tagline, false),
		Notice:         adtyPlainText(input.Notice, true),
		SupportContact: adtyPlainText(input.SupportContact, false),
		LogoURL:        strings.TrimSpace(input.LogoURL),
	}
	if utf8.RuneCountInString(out.Name) > adtyBrandNameMax || utf8.RuneCountInString(out.Tagline) > adtyBrandTaglineMax ||
		utf8.RuneCountInString(out.Notice) > adtyBrandNoticeMax || utf8.RuneCountInString(out.SupportContact) > adtyBrandSupportMax {
		return out, ErrAdtySiteBrandInvalid
	}
	if out.LogoURL != "" {
		name, ok := strings.CutPrefix(out.LogoURL, AdtySiteAssetPrefix)
		if !ok || name == "" || strings.ContainsAny(name, "/\\?#:") || strings.Contains(name, "..") {
			return out, ErrAdtySiteBrandInvalid
		}
	}
	return out, nil
}

// AdtyBrandFor 是站点的品牌；nil 返回主站品牌。没填站名的分站用子域名。
func AdtyBrandFor(site *model.AdtySite) AdtySiteBrand {
	if site == nil {
		return AdtySiteBrand{Name: common.SystemName, BaseURL: AdtySiteBaseURL(nil)}
	}
	var brand AdtySiteBrandInput
	if strings.TrimSpace(site.Brand) != "" {
		_ = common.UnmarshalJsonStr(site.Brand, &brand)
	}
	name := strings.TrimSpace(brand.Name)
	if name == "" {
		name = site.Subdomain
	}
	return AdtySiteBrand{
		Name: name, LogoURL: strings.TrimSpace(brand.LogoURL), Tagline: brand.Tagline, Notice: brand.Notice,
		SupportContact: brand.SupportContact, BaseURL: AdtySiteBaseURL(site),
	}
}

// adtyPlatformSender 是全局的系统邮件发件地址：Resend 用 ResendFrom，SMTP 用 SMTPFrom。
func adtyPlatformSender() (string, string) {
	common.OptionMapRWMutex.RLock()
	provider, resendFrom := common.OptionMap["EmailProvider"], common.OptionMap["ResendFrom"]
	common.OptionMapRWMutex.RUnlock()
	if provider == "resend" || (provider == "" && resendFrom != "" && common.SMTPFrom == "") {
		if parsed, err := mail.ParseAddress(strings.TrimSpace(resendFrom)); err == nil {
			return parsed.Address, provider
		}
	}
	from := common.SMTPFrom
	if from == "" {
		from = common.SMTPAccount
	}
	return from, provider
}

// adtyFormatSender 写成「站名 <地址>」。站名只有字母、数字、空格和常见标点时不加引号（和平台的 ResendFrom 写法一致），
// 否则交给 net/mail 按 RFC 5322 加引号或编码。
func adtyFormatSender(name, address string) string {
	plain := name != ""
	for _, r := range name {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" -_!#$%&'*+/=?^`{|}~", r)) {
			plain = false
			break
		}
	}
	if plain && strings.TrimSpace(name) == name {
		return name + " <" + address + ">"
	}
	return (&mail.Address{Name: name, Address: address}).String()
}

// AdtySenderFor 是系统邮件的发件人：站点开了站长发件、发件域名在 Resend 验证通过，并且全局用 Resend 发信时，
// 用「站名 <noreply@站长发件域名>」；否则用「站名 <全局发件地址>」。SMTP 只能用全局发件地址，只换显示名。
func AdtySenderFor(site *model.AdtySite) string {
	name := AdtyBrandFor(site).Name
	platform, provider := adtyPlatformSender()
	if site != nil && resellerSetting().SenderDomainEnabled && provider != "smtp" {
		var domain model.AdtySiteSenderDomain
		if model.DB.Where("site_id = ? AND status = ?", site.Id, model.AdtySenderDomainVerified).Limit(1).Find(&domain).RowsAffected > 0 {
			return adtyFormatSender(name, "noreply@"+domain.Domain)
		}
	}
	return adtyFormatSender(name, platform)
}

// AdtySendSiteEmail 用站点的发件人发一封系统邮件；site 为 nil 时和上游 SendEmail 一样。
func AdtySendSiteEmail(site *model.AdtySite, subject, receiver, content string) error {
	if site == nil {
		return common.SendEmail(subject, receiver, content)
	}
	return common.SendEmailFrom(AdtySenderFor(site), subject, receiver, content)
}

var (
	ErrAdtySenderDomainOff       = common.NewMessage("Sender domains are not available")
	ErrAdtySenderDomainInvalid   = common.NewMessage("Enter a domain you own, such as mail.example.com; our own domains cannot be used")
	ErrAdtySenderDomainQuotaFull = common.NewMessage("No sender domain slots are left. Please contact the platform")
	ErrAdtySenderDomainFailed    = common.NewMessage("The email provider could not complete the request; try again later")
)

// normalizeSenderDomain 校验发件域名：合法的 DNS 名，至少两段，不能是我们的根域名、主站域名下的。
func normalizeSenderDomain(raw string) (string, error) {
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if strings.Count(domain, ".") < 1 || len(domain) > 253 {
		return "", ErrAdtySenderDomainInvalid
	}
	for _, label := range strings.Split(domain, ".") {
		if !adty_setting.ResellerLabelPattern.MatchString(label) {
			return "", ErrAdtySenderDomainInvalid
		}
	}
	blocked := append([]string{}, resellerSetting().RootDomains...)
	if main := adtyMainHost(); main != "" && main != "localhost" {
		blocked = append(blocked, adtyRegistrableDomain(main))
	}
	blocked = append(blocked, strings.TrimPrefix(common.AdtyBrandURL, "https://"))
	for _, own := range blocked {
		if adtyHostUnder(domain, strings.ToLower(strings.TrimSpace(own))) {
			return "", ErrAdtySenderDomainInvalid
		}
	}
	return domain, nil
}

// adtyResendUserError 把 Resend 的错误换成给人看的提示：名额满了提示联系平台。原始错误只记状态码和名称。
func adtyResendUserError(err error) error {
	var apiErr *AdtyResendError
	if errors.As(err, &apiErr) {
		common.SysError(common.LogText("resend domain request failed: status=%d name=%s", apiErr.StatusCode, apiErr.Name))
		message := strings.ToLower(apiErr.Message)
		if apiErr.StatusCode == http.StatusForbidden || strings.Contains(message, "limit") || strings.Contains(message, "quota") {
			adtyNoteSenderQuotaFull() // 后台用量页显示最近一次名额满（11.9）
			return ErrAdtySenderDomainQuotaFull
		}
		return ErrAdtySenderDomainFailed
	}
	if errors.Is(err, ErrAdtyResendNotConfigured) {
		return ErrAdtySenderDomainOff
	}
	common.SysError(common.LogText("resend domain request failed: %s", err.Error()))
	return ErrAdtySenderDomainFailed
}

// deleteSenderDomainIfPresent 从 Resend 删域名。404 当成已经不在，避免旧 id 失效后换域名一直失败。
func deleteSenderDomainIfPresent(ctx context.Context, domainID string) error {
	if domainID == "" {
		return nil
	}
	err := AdtyResend.DeleteDomain(ctx, domainID)
	if err == nil {
		return nil
	}
	var apiErr *AdtyResendError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil
	}
	return adtyResendUserError(err)
}

// AddAdtySiteSenderDomain 给站长的站设置发件域名（sender_domain_enabled 打开后可用）：调 Resend 建域名，
// 存域名 id 和要加的 DNS 记录。换成别的域名时，先建新的并写库，库提交成功后再删旧的；
// 写库失败就删掉刚建的新域名，Resend 和库都留在原来的发件域名上。
func AddAdtySiteSenderDomain(ctx context.Context, ownerUserId int, rawDomain string) (*model.AdtySiteSenderDomain, error) {
	if !resellerSetting().SenderDomainEnabled {
		return nil, ErrAdtySenderDomainOff
	}
	site, err := adtyOwnerSite(ownerUserId)
	if err != nil {
		return nil, err
	}
	domain, err := normalizeSenderDomain(rawDomain)
	if err != nil {
		return nil, err
	}
	var existing model.AdtySiteSenderDomain
	found := model.DB.Where("site_id = ?", site.Id).Limit(1).Find(&existing).RowsAffected > 0
	if found && existing.Domain == domain && (existing.Status == model.AdtySenderDomainPending || existing.Status == model.AdtySenderDomainVerified) {
		return &existing, nil
	}
	// 同名重建时旧域名还占着这个名字，得先删掉才能再建。别的名字先留着，等库写成功再删。
	if found && existing.Domain == domain && existing.ResendDomainId != "" && existing.Status != model.AdtySenderDomainDisabled {
		if err := deleteSenderDomainIfPresent(ctx, existing.ResendDomainId); err != nil {
			return nil, err
		}
	}
	created, err := AdtyResend.CreateDomain(ctx, AdtyResendDomainCreate{Name: domain})
	if err != nil {
		return nil, adtyResendUserError(err)
	}
	records, _ := common.Marshal(created.Records)
	now := time.Now().Unix()
	row := model.AdtySiteSenderDomain{
		SiteId: site.Id, Domain: domain, ResendDomainId: created.ID, Status: model.AdtySenderDomainPending,
		Records: string(records), LastCheckedAt: now, CreatedAt: now,
	}
	if created.Status == "verified" {
		row.Status = model.AdtySenderDomainVerified
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if found {
			return tx.Model(&model.AdtySiteSenderDomain{}).Where("site_id = ?", site.Id).Updates(map[string]any{
				"domain": row.Domain, "resend_domain_id": row.ResendDomainId, "status": row.Status, "records": row.Records,
				"disabled_by": 0, "last_checked_at": now, "created_at": now,
			}).Error
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		if delErr := deleteSenderDomainIfPresent(ctx, created.ID); delErr != nil {
			common.SysError(common.LogText("roll back new sender domain for site %d failed", site.Id))
		}
		return nil, err
	}
	if found && existing.Domain != domain && existing.ResendDomainId != "" && existing.Status != model.AdtySenderDomainDisabled {
		if delErr := deleteSenderDomainIfPresent(ctx, existing.ResendDomainId); delErr != nil {
			common.SysError(common.LogText("delete replaced sender domain for site %d failed", site.Id))
		}
	}
	return &row, nil
}

// GetAdtySiteSenderDomain 是站长自己站上的发件域名；没有返回 nil。
func GetAdtySiteSenderDomain(ownerUserId int) (*model.AdtySiteSenderDomain, error) {
	site, err := adtyOwnerSite(ownerUserId)
	if err != nil {
		return nil, err
	}
	var row model.AdtySiteSenderDomain
	if model.DB.Where("site_id = ? AND status <> ?", site.Id, model.AdtySenderDomainDisabled).Limit(1).Find(&row).RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// RemoveAdtySiteSenderDomain 是站长删除自己的发件域名。
func RemoveAdtySiteSenderDomain(ctx context.Context, ownerUserId int) error {
	site, err := adtyOwnerSite(ownerUserId)
	if err != nil {
		return err
	}
	return DisableAdtySiteSenderDomain(ctx, site.Id, 0)
}

// DisableAdtySiteSenderDomain 从 Resend 删掉站点的发件域名，回退到平台发件地址。operatorId 是停用的管理员（站长自己删时为 0）。
func DisableAdtySiteSenderDomain(ctx context.Context, siteId, operatorId int) error {
	var row model.AdtySiteSenderDomain
	if model.DB.Where("site_id = ?", siteId).Limit(1).Find(&row).RowsAffected == 0 || row.Status == model.AdtySenderDomainDisabled {
		return nil
	}
	if row.ResendDomainId != "" {
		if err := AdtyResend.DeleteDomain(ctx, row.ResendDomainId); err != nil {
			var apiErr *AdtyResendError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
				return adtyResendUserError(err)
			}
		}
	}
	return model.DB.Model(&model.AdtySiteSenderDomain{}).Where("site_id = ?", siteId).Updates(map[string]any{
		"status": model.AdtySenderDomainDisabled, "disabled_by": operatorId, "last_checked_at": time.Now().Unix(),
	}).Error
}

// RunAdtySenderDomainCheckOnce 查一遍 pending 的发件域名，Resend 说 verified 就启用，failed 就标失败。
func RunAdtySenderDomainCheckOnce(ctx context.Context, now time.Time) error {
	var rows []model.AdtySiteSenderDomain
	if err := model.DB.Where("status = ?", model.AdtySenderDomainPending).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		updates := map[string]any{"last_checked_at": now.Unix()}
		domain, err := AdtyResend.GetDomain(ctx, row.ResendDomainId)
		switch {
		case err != nil:
			if errors.Is(err, ErrAdtyResendNotConfigured) {
				return nil
			}
			common.SysError(common.LogText("resend domain check failed for site %d", row.SiteId))
		case domain.Status == "verified":
			updates["status"] = model.AdtySenderDomainVerified
		case domain.Status == "failed":
			updates["status"] = model.AdtySenderDomainFailed
		}
		// 只写回仍是这次巡检读到的 pending 行。停用或换了域名后，不能把旧的 verified/failed 盖上去。
		if err := model.DB.Model(&model.AdtySiteSenderDomain{}).
			Where("site_id = ? AND status = ? AND resend_domain_id = ?", row.SiteId, model.AdtySenderDomainPending, row.ResendDomainId).
			Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

// adtyRemoveSenderDomainOfClosedSite 在关站后删掉发件域名（AdtyOnSiteClosed）。失败只记日志。
func adtyRemoveSenderDomainOfClosedSite(site *model.AdtySite) {
	if err := DisableAdtySiteSenderDomain(context.Background(), site.Id, 0); err != nil {
		common.SysError(common.LogText("remove sender domain of closed site %d failed", site.Id))
	}
}

// adtyMarketingSenderForSite 是营销邮件的发件人（替换 adty_marketing_campaign.go 的挂点）：
// 营销邮件和系统邮件用不同的子域名，地址一律用平台营销地址；分站开了站长发件并验证通过时，显示名换成分站站名。
func adtyMarketingSenderForSite(siteID int) AdtyMarketingSender {
	platform := AdtyMarketingSender{From: marketingSetting().FromAddress, SiteName: common.SystemName}
	site := AdtySiteById(siteID)
	if site == nil || !resellerSetting().SenderDomainEnabled {
		return platform
	}
	var row model.AdtySiteSenderDomain
	if model.DB.Where("site_id = ? AND status = ?", site.Id, model.AdtySenderDomainVerified).Limit(1).Find(&row).RowsAffected == 0 {
		return platform
	}
	parsed, err := mail.ParseAddress(strings.TrimSpace(platform.From))
	if err != nil {
		return platform
	}
	name := AdtyBrandFor(site).Name
	return AdtyMarketingSender{From: adtyFormatSender(name, parsed.Address), SiteName: name}
}

func startAdtySenderDomainTask() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			if err := RunAdtySenderDomainCheckOnce(ctx, time.Now()); err != nil {
				common.SysError(common.LogText("sender domain check task: %s", err.Error()))
			}
			cancel()
		}
	}()
}

func init() {
	adtyMasterTasks = append(adtyMasterTasks, startAdtySenderDomainTask)
	AdtyMarketingSiteSender = adtyMarketingSenderForSite
	AdtyOnSiteClosing = adtyNotifySiteClosing
	AdtyOnSiteMarkupIncrease = adtyNotifySiteMarkupIncrease
	AdtyOnSiteCostFactorChanged = adtyNotifySiteCostFactor
	previous := AdtyOnSiteClosed
	AdtyOnSiteClosed = func(site *model.AdtySite) {
		if previous != nil {
			previous(site)
		}
		adtyRemoveSenderDomainOfClosedSite(site)
	}
}

func adtyMailWhen(unix int64) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).Format("2006-01-02 15:04 MST")
}

func adtySendSiteMail(site *model.AdtySite, user *model.User, subjectKey, bodyKey string, args map[string]any) {
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return
	}
	lang := user.GetSetting().Language
	subject := i18n.Translate(lang, subjectKey, args)
	body := "<p>" + html.EscapeString(i18n.Translate(lang, bodyKey, args)) + "</p>"
	AdtySendSiteEmailAsync(site, subject, user.Email, body)
}

func adtySiteCustomerUsers(siteId int) []model.User {
	var users []model.User
	if err := model.DB.Select("id", "email", "setting", "site_id").Where("site_id = ?", siteId).Find(&users).Error; err != nil {
		common.SysError(common.LogText("load site %d customers for mail: %s", siteId, err.Error()))
		return nil
	}
	return users
}

// adtyNotifySiteClosing 给这个站的客户发关站邮件。
func adtyNotifySiteClosing(site *model.AdtySite) {
	if site == nil {
		return
	}
	link := AdtySiteBaseURL(nil)
	if site.CloseTransferTo != 0 {
		if target := AdtySiteById(site.CloseTransferTo); target != nil {
			link = AdtySiteBaseURL(target)
		}
	}
	args := map[string]any{
		"SystemName": AdtyBrandFor(site).Name,
		"When":       adtyMailWhen(site.CloseExecuteAt),
		"Link":       link,
	}
	for _, user := range adtySiteCustomerUsers(site.Id) {
		copied := user
		adtySendSiteMail(site, &copied, i18n.MsgAdtySiteClosingSubject, i18n.MsgAdtySiteClosingBody, args)
	}
}

func adtyMarkupRaisedDetail(raised []model.AdtySitePrice) (string, int64) {
	parts := make([]string, 0, len(raised))
	var at int64
	for _, row := range raised {
		group := row.Group
		if group == "" {
			group = "all groups"
		}
		next := row.PendingMarkup
		if next.IsZero() && row.Markup.IsPositive() {
			next = row.Markup
		}
		parts = append(parts, group+" → "+next.String())
		if row.PendingAt > at {
			at = row.PendingAt
		}
	}
	return strings.Join(parts, "; "), at
}

// adtyNotifySiteMarkupIncrease 给 k 变高的客户发邮件。
func adtyNotifySiteMarkupIncrease(site *model.AdtySite, userId int, raised []model.AdtySitePrice) {
	if site == nil || len(raised) == 0 {
		return
	}
	detail, at := adtyMarkupRaisedDetail(raised)
	args := map[string]any{
		"SystemName": AdtyBrandFor(site).Name,
		"Detail":     detail,
		"When":       adtyMailWhen(at),
		"Link":       AdtySiteBaseURL(site),
	}
	var users []model.User
	if userId > 0 {
		if user, err := model.GetUserById(userId, false); err == nil {
			users = []model.User{*user}
		}
	} else {
		users = adtySiteCustomerUsers(site.Id)
	}
	for _, user := range users {
		copied := user
		adtySendSiteMail(site, &copied, i18n.MsgAdtySiteMarkupSubject, i18n.MsgAdtySiteMarkupBody, args)
	}
}

// adtyNotifySiteCostFactor 给站长发邮件，说明当前生效的 d。
func adtyNotifySiteCostFactor(site *model.AdtySite) {
	if site == nil {
		return
	}
	owner, err := model.GetUserById(site.OwnerUserId, false)
	if err != nil {
		common.SysError(common.LogText("load site %d owner for cost factor mail: %s", site.Id, err.Error()))
		return
	}
	factor, ok := AdtySiteCostFactor(site)
	if !ok {
		factor = decimal.Zero
	}
	adtySendSiteMail(site, owner, i18n.MsgAdtySiteCostNowSubject, i18n.MsgAdtySiteCostNowBody, map[string]any{
		"Factor": factor.String(),
	})
}

// AdtySiteForEmail 是这个邮箱的账号所属的站点（只有邮箱、没有用户 id 的系统邮件用）；查不到返回 nil。
func AdtySiteForEmail(email string) *model.AdtySite {
	if !resellerSetting().Enabled || strings.TrimSpace(email) == "" {
		return nil
	}
	user, err := model.GetUniqueUserByEmail(model.NormalizeEmail(email))
	if err != nil {
		return nil
	}
	return AdtySiteById(user.SiteId)
}

// 支付跳回（任务 11.3，执行层决定 30）。易支付的 return_url 一律是主站的 /pay/result/<订单号>.<签名>：
// 网关会往 return_url 后面加自己的参数，所以不带 query；签名防止拿别人的订单号探测订单属于哪个站。
// 分站客户在主站没有登录会话（会话按 Host），这里不能靠登录判断订单归属，靠的是签名：只有我们生成的地址才会跳。

const adtyPayResultPath = "/pay/result/"

func adtyPayResultSignature(tradeNo string) string {
	return common.GenerateHMAC("adty-pay-result:" + tradeNo)[:16]
}

// AdtyPayResultURL 是给易支付的 return_url。
func AdtyPayResultURL(tradeNo string) string {
	return AdtySiteBaseURL(nil) + adtyPayResultPath + tradeNo + "." + adtyPayResultSignature(tradeNo)
}

// AdtyPayResultTarget 是 /pay/result/<订单号>.<签名> 要跳到的地址：签名对、订单存在时，跳订单所属用户的站点的
// 钱包并带上订单号；其余一律跳主站钱包，不带订单号。
func AdtyPayResultTarget(token string) string {
	fallback := AdtySiteBaseURL(nil) + "/wallet"
	// 订单号本身可能带点：签名是最后一段
	dot := strings.LastIndex(token, ".")
	if dot <= 0 {
		return fallback
	}
	tradeNo, signature := token[:dot], token[dot+1:]
	if subtle.ConstantTimeCompare([]byte(signature), []byte(adtyPayResultSignature(tradeNo))) != 1 {
		return fallback
	}
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		return fallback
	}
	return AdtySiteBaseURL(AdtySiteForUser(topUp.UserId)) + "/wallet?trade_no=" + url.QueryEscape(tradeNo)
}
