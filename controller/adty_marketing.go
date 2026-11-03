package controller

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// 营销（P12）的接口：注册页同意框默认值、个人设置里的同意、Resend webhook、后台营销设置。

// GetAdtyMarketingConsentDefault 返回注册页的营销同意框是否默认勾上（按 CF-IPCountry，任务 12.1）。
// 营销总开关关着时 show=false，注册页不显示这个框。
func GetAdtyMarketingConsentDefault(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"show":    adty_setting.GetMarketingSetting().Enabled,
		"checked": service.AdtyConsentDefault(c.GetHeader("CF-IPCountry")),
	})
}

// GetAdtyMarketingSelf 返回自己的营销邮件状态。
func GetAdtyMarketingSelf(c *gin.Context) {
	contact, err := model.GetAdtyMarketingContact(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if contact == nil {
		contact = &model.AdtyMarketingContact{UserId: c.GetInt("id")}
	}
	common.ApiSuccess(c, gin.H{
		"consent": contact.Consent && !contact.Unsubscribed, "consent_at": contact.ConsentAt,
		"blocked": contact.Bounced || contact.Complained, "enabled": adty_setting.GetMarketingSetting().Enabled,
	})
}

// PutAdtyMarketingSelf 在个人设置里改营销同意。退订后系统邮件照常发。
func PutAdtyMarketingSelf(c *gin.Context) {
	var body struct {
		Consent *bool  `json:"consent"`
		Lang    string `json:"lang"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.Consent == nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if _, err := service.AdtySetMarketingConsent(c.GetInt("id"), *body.Consent, service.HashReferralRiskValue(c.ClientIP()), body.Lang, "profile"); err != nil {
		common.ApiError(c, err)
		return
	}
	GetAdtyMarketingSelf(c)
}

// PostAdtyResendWebhook 收 Resend 的 webhook（验签，任务 12.1）。签名不对回 401；处理出错回 500，Resend 会重试。
func PostAdtyResendWebhook(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 256<<10))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if err := service.HandleAdtyResendWebhook(c.Request.Header, raw, time.Now()); err != nil {
		if errors.Is(err, service.ErrAdtyMarketingWebhookSignature) {
			common.SysLog(common.LogText("resend webhook rejected: bad signature"))
			c.Status(http.StatusUnauthorized)
			return
		}
		common.SysError(common.LogText("resend webhook failed: %s", err.Error()))
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusOK)
}

// adtyMarketingSettingsView 是后台营销设置。webhook 密钥不回传，只回是否已填。
func adtyMarketingSettingsView() gin.H {
	cfg := adty_setting.GetMarketingSetting()
	common.OptionMapRWMutex.RLock()
	resendKey := strings.TrimSpace(common.OptionMap["ResendApiKey"]) != ""
	common.OptionMapRWMutex.RUnlock()
	return gin.H{
		"enabled": cfg.Enabled, "from_address": cfg.FromAddress, "resend_segment_prefix": cfg.ResendSegmentPrefix,
		"consent_default_non_eu": cfg.ConsentDefaultNonEU, "consent_version": cfg.ConsentVersion, "eu_countries": cfg.EUCountries,
		"footer_company": cfg.FooterCompany, "footer_address": cfg.FooterAddress,
		"open_tracking": cfg.OpenTracking, "click_tracking": cfg.ClickTracking,
		"sync_batch_size": cfg.SyncBatchSize, "sync_interval_seconds": cfg.SyncIntervalSeconds, "api_rate_per_second": cfg.APIRatePerSecond,
		"include_site_customers_default": cfg.IncludeSiteCustomersDefault,
		"promo_enabled":                  cfg.PromoEnabled,
		"webhook_secret_configured":      strings.TrimSpace(cfg.WebhookSecret) != "",
		"resend_key_configured":          resendKey,
		"missing":                        adtyMarketingMissing(cfg, resendKey),
	}
}

// adtyMarketingMissing 列出打开营销总开关前还没填的项。
func adtyMarketingMissing(cfg *adty_setting.MarketingSetting, resendKey bool) []string {
	missing := []string{}
	if strings.TrimSpace(cfg.FromAddress) == "" {
		missing = append(missing, "from_address")
	}
	if strings.TrimSpace(cfg.FooterCompany) == "" {
		missing = append(missing, "footer_company")
	}
	if strings.TrimSpace(cfg.FooterAddress) == "" {
		missing = append(missing, "footer_address")
	}
	if strings.TrimSpace(cfg.WebhookSecret) == "" {
		missing = append(missing, "webhook_secret")
	}
	if !resendKey {
		missing = append(missing, "resend_api_key")
	}
	return missing
}

// GetAdtyMarketingSettings 返回后台营销设置（营销·查看）。
func GetAdtyMarketingSettings(c *gin.Context) {
	common.ApiSuccess(c, adtyMarketingSettingsView())
}

// PutAdtyMarketingSettings 保存营销设置（营销·编辑）。只改传了的项；webhook 密钥传空串不改，
// clear_webhook_secret=true 时清掉。打开总开关（或开着时清掉必填项）前，必填项都要有值。
func PutAdtyMarketingSettings(c *gin.Context) {
	var body struct {
		Enabled                     *bool   `json:"enabled"`
		FromAddress                 *string `json:"from_address"`
		ResendSegmentPrefix         *string `json:"resend_segment_prefix"`
		ConsentDefaultNonEU         *bool   `json:"consent_default_non_eu"`
		ConsentVersion              *string `json:"consent_version"`
		EUCountries                 *string `json:"eu_countries"`
		FooterCompany               *string `json:"footer_company"`
		FooterAddress               *string `json:"footer_address"`
		OpenTracking                *bool   `json:"open_tracking"`
		ClickTracking               *bool   `json:"click_tracking"`
		SyncBatchSize               *int    `json:"sync_batch_size"`
		SyncIntervalSeconds         *int    `json:"sync_interval_seconds"`
		APIRatePerSecond            *int    `json:"api_rate_per_second"`
		IncludeSiteCustomersDefault *bool   `json:"include_site_customers_default"`
		PromoEnabled                *bool   `json:"promo_enabled"`
		WebhookSecret               string  `json:"webhook_secret"`
		ClearWebhookSecret          bool    `json:"clear_webhook_secret"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if strings.TrimSpace(body.WebhookSecret) != "" || body.ClearWebhookSecret {
		common.ApiErrorT(c, "Webhook signing secrets can only be changed on the marketing secrets page")
		return
	}
	values := map[string]string{}
	after := *adty_setting.GetMarketingSetting()
	putString := func(key string, value *string, target *string, maxLen int) bool {
		if value == nil {
			return true
		}
		trimmed := strings.TrimSpace(*value)
		if len(trimmed) > maxLen {
			common.ApiErrorT(c, "Invalid parameters")
			return false
		}
		values[adty_setting.MarketingKeyPrefix+key] = trimmed
		*target = trimmed
		return true
	}
	putBool := func(key string, value *bool, target *bool) {
		if value != nil {
			values[adty_setting.MarketingKeyPrefix+key] = strconv.FormatBool(*value)
			*target = *value
		}
	}
	putInt := func(key string, value *int, target *int, minimum, maximum int) bool {
		if value == nil {
			return true
		}
		if *value < minimum || *value > maximum {
			common.ApiErrorT(c, "Invalid parameters")
			return false
		}
		values[adty_setting.MarketingKeyPrefix+key] = strconv.Itoa(*value)
		*target = *value
		return true
	}
	if !putString("from_address", body.FromAddress, &after.FromAddress, 255) ||
		!putString("resend_segment_prefix", body.ResendSegmentPrefix, &after.ResendSegmentPrefix, 32) ||
		!putString("consent_version", body.ConsentVersion, &after.ConsentVersion, 32) ||
		!putString("eu_countries", body.EUCountries, &after.EUCountries, 1000) ||
		!putString("footer_company", body.FooterCompany, &after.FooterCompany, 255) ||
		!putString("footer_address", body.FooterAddress, &after.FooterAddress, 500) ||
		!putInt("sync_batch_size", body.SyncBatchSize, &after.SyncBatchSize, 1, 1000) ||
		!putInt("sync_interval_seconds", body.SyncIntervalSeconds, &after.SyncIntervalSeconds, 10, 86400) ||
		!putInt("api_rate_per_second", body.APIRatePerSecond, &after.APIRatePerSecond, 1, 100) {
		return
	}
	if body.FromAddress != nil && after.FromAddress != "" {
		if _, err := mail.ParseAddress(after.FromAddress); err != nil {
			common.ApiErrorT(c, "Enter a valid sender address, such as ADTY AI <news@example.com>")
			return
		}
	}
	putBool("enabled", body.Enabled, &after.Enabled)
	putBool("consent_default_non_eu", body.ConsentDefaultNonEU, &after.ConsentDefaultNonEU)
	putBool("open_tracking", body.OpenTracking, &after.OpenTracking)
	putBool("click_tracking", body.ClickTracking, &after.ClickTracking)
	putBool("include_site_customers_default", body.IncludeSiteCustomersDefault, &after.IncludeSiteCustomersDefault)
	putBool("promo_enabled", body.PromoEnabled, &after.PromoEnabled)
	common.OptionMapRWMutex.RLock()
	resendKey := strings.TrimSpace(common.OptionMap["ResendApiKey"]) != ""
	common.OptionMapRWMutex.RUnlock()
	if after.Enabled && len(adtyMarketingMissing(&after, resendKey)) > 0 {
		common.ApiErrorT(c, "Fill in the sender address, footer, webhook signing secret and Resend API key before turning marketing emails on")
		return
	}
	if len(values) > 0 {
		if err := model.UpdateOptionsBulk(values); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, strings.TrimPrefix(key, adty_setting.MarketingKeyPrefix))
	}
	recordManageAudit(c, "adty.marketing.settings", map[string]any{"keys": keys})
	GetAdtyMarketingSettings(c)
}

// PutAdtyMarketingSecrets 只给超级管理员改 webhook 签名密钥和 Resend API key。
func PutAdtyMarketingSecrets(c *gin.Context) {
	var body struct {
		WebhookSecret      string  `json:"webhook_secret"`
		ClearWebhookSecret bool    `json:"clear_webhook_secret"`
		ResendAPIKey       *string `json:"resend_api_key"`
		ClearResendAPIKey  bool    `json:"clear_resend_api_key"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	values := map[string]string{}
	if secret := strings.TrimSpace(body.WebhookSecret); secret != "" {
		if body.ClearWebhookSecret || !strings.HasPrefix(secret, "whsec_") || len(secret) > 200 {
			common.ApiErrorT(c, "The webhook signing secret starts with whsec_")
			return
		}
		values[adty_setting.MarketingKeyWebhookSecret] = secret
	} else if body.ClearWebhookSecret {
		values[adty_setting.MarketingKeyWebhookSecret] = ""
	}
	if body.ClearResendAPIKey && body.ResendAPIKey != nil && strings.TrimSpace(*body.ResendAPIKey) != "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if body.ClearResendAPIKey || body.ResendAPIKey != nil {
		key := ""
		if body.ResendAPIKey != nil && !body.ClearResendAPIKey {
			key = strings.TrimSpace(*body.ResendAPIKey)
		}
		values["ResendApiKey"] = key
	}
	if len(values) == 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	cfg := adty_setting.GetMarketingSetting()
	after := *cfg
	if secret, ok := values[adty_setting.MarketingKeyWebhookSecret]; ok {
		after.WebhookSecret = secret
	}
	common.OptionMapRWMutex.RLock()
	resendKey := strings.TrimSpace(common.OptionMap["ResendApiKey"]) != ""
	common.OptionMapRWMutex.RUnlock()
	if key, ok := values["ResendApiKey"]; ok {
		resendKey = strings.TrimSpace(key) != ""
	}
	if cfg.Enabled && len(adtyMarketingMissing(&after, resendKey)) > 0 {
		common.ApiErrorT(c, "Fill in the sender address, footer, webhook signing secret and Resend API key before turning marketing emails on")
		return
	}
	// 签名密钥和 Resend key 同一次提交。Resend key 校验失败时不能先把签名密钥落库。
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.marketing.secrets", map[string]any{
		"webhook_secret": body.WebhookSecret != "" || body.ClearWebhookSecret,
		"resend_api_key": body.ResendAPIKey != nil || body.ClearResendAPIKey,
	})
	common.ApiSuccess(c, adtyMarketingSettingsView())
}

// adtyCampaignInput 是新建、修改群发和模板的请求。
type adtyCampaignInput struct {
	Name        string                      `json:"name"`
	IsTemplate  bool                        `json:"is_template"`
	Subjects    model.AdtyLangText          `json:"subjects"`
	Bodies      model.AdtyLangText          `json:"bodies"`
	Audience    model.AdtyMarketingAudience `json:"audience"`
	ScheduledAt int64                       `json:"scheduled_at"`
}

// apply 校验并写进 campaign：只留 7 种内容语言、去掉空的语言。
func (in *adtyCampaignInput) apply(campaign *model.AdtyMarketingCampaign) bool {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 128 || in.ScheduledAt < 0 {
		return false
	}
	clean := func(texts model.AdtyLangText, maxLen int) (model.AdtyLangText, bool) {
		out := model.AdtyLangText{}
		for lang, text := range texts {
			if !slices.Contains(model.AdtyContentLangs, lang) || len(text) > maxLen {
				return nil, false
			}
			if strings.TrimSpace(text) != "" {
				out[lang] = text
			}
		}
		return out, true
	}
	subjects, ok := clean(in.Subjects, 300)
	if !ok {
		return false
	}
	bodies, ok := clean(in.Bodies, 200<<10)
	if !ok {
		return false
	}
	campaign.Name, campaign.IsTemplate, campaign.Subjects, campaign.Bodies = name, in.IsTemplate, subjects, bodies
	campaign.Audience, campaign.ScheduledAt = in.Audience, in.ScheduledAt
	return true
}

func adtyCampaignByParam(c *gin.Context) (*model.AdtyMarketingCampaign, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return nil, false
	}
	var campaign model.AdtyMarketingCampaign
	if err := model.DB.First(&campaign, id).Error; err != nil {
		common.ApiErrorT(c, "Campaign not found")
		return nil, false
	}
	return &campaign, true
}

// GetAdtyCampaigns 列出群发；is_template=true 时列模板（营销·查看）。
func GetAdtyCampaigns(c *gin.Context) {
	query := model.DB.Model(&model.AdtyMarketingCampaign{}).Where("is_template = ?", c.Query("is_template") == "true")
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	var items []model.AdtyMarketingCampaign
	referralPage(c, query, &items)
}

// GetAdtyCampaign 返回一个群发、每次 broadcast 和统计（营销·查看）。
func GetAdtyCampaign(c *gin.Context) {
	campaign, ok := adtyCampaignByParam(c)
	if !ok {
		return
	}
	stats, err := service.AdtyCampaignStats(campaign.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var sends []model.AdtyMarketingCampaignSend
	if err := model.DB.Where("campaign_id = ?", campaign.Id).Order("id").Find(&sends).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"campaign": campaign, "stats": stats, "sends": sends})
}

// PostAdtyAudienceEstimate 返回按条件预计的收件人数（只算同意且可发的，营销·查看）。
func PostAdtyAudienceEstimate(c *gin.Context) {
	var audience model.AdtyMarketingAudience
	if err := common.DecodeJson(c.Request.Body, &audience); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	estimate, err := service.EstimateAdtyCampaignAudience(audience)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, estimate)
}

// PostAdtyCampaign 新建群发草稿或模板（营销·编辑）。
func PostAdtyCampaign(c *gin.Context) {
	var in adtyCampaignInput
	campaign := model.AdtyMarketingCampaign{Status: model.AdtyCampaignStatusDraft, CreatedBy: c.GetInt("id")}
	if err := common.DecodeJson(c.Request.Body, &in); err != nil || !in.apply(&campaign) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := model.DB.Create(&campaign).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.marketing.campaign_create", map[string]any{"id": campaign.Id, "template": campaign.IsTemplate})
	common.ApiSuccess(c, campaign)
}

// PutAdtyCampaign 改草稿或模板（营销·编辑）。已经发出的不能改。
func PutAdtyCampaign(c *gin.Context) {
	campaign, ok := adtyCampaignByParam(c)
	if !ok {
		return
	}
	if campaign.Status != model.AdtyCampaignStatusDraft {
		common.ApiErrorT(c, "This campaign has already been sent")
		return
	}
	var in adtyCampaignInput
	if err := common.DecodeJson(c.Request.Body, &in); err != nil || !in.apply(campaign) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	result := model.DB.Model(&model.AdtyMarketingCampaign{}).Where("id = ? AND status = ?", campaign.Id, model.AdtyCampaignStatusDraft).
		Select("name", "is_template", "subjects", "bodies", "audience", "scheduled_at").Updates(campaign)
	if result.Error != nil {
		common.ApiError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		common.ApiErrorT(c, "This campaign has already been sent")
		return
	}
	recordManageAudit(c, "adty.marketing.campaign_update", map[string]any{"id": campaign.Id})
	common.ApiSuccess(c, campaign)
}

// DeleteAdtyCampaign 删草稿或模板（营销·编辑）。
func DeleteAdtyCampaign(c *gin.Context) {
	campaign, ok := adtyCampaignByParam(c)
	if !ok {
		return
	}
	result := model.DB.Where("id = ? AND status = ?", campaign.Id, model.AdtyCampaignStatusDraft).Delete(&model.AdtyMarketingCampaign{})
	if result.Error != nil {
		common.ApiError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		common.ApiErrorT(c, "This campaign has already been sent")
		return
	}
	recordManageAudit(c, "adty.marketing.campaign_delete", map[string]any{"id": campaign.Id})
	common.ApiSuccess(c, nil)
}

// GetAdtyCampaignPlan 返回发送前确认框里的人数和语言版本（营销·查看）。
func GetAdtyCampaignPlan(c *gin.Context) {
	campaign, ok := adtyCampaignByParam(c)
	if !ok {
		return
	}
	plan, err := service.PlanAdtyCampaign(campaign)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plan)
}

// PostAdtyCampaignTest 给自己发一封测试邮件（营销·发送）。
func PostAdtyCampaignTest(c *gin.Context) {
	campaign, ok := adtyCampaignByParam(c)
	if !ok {
		return
	}
	var body struct {
		To   string `json:"to"`
		Lang string `json:"lang"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SendAdtyCampaignTest(c.Request.Context(), campaign.Id, body.Lang, body.To); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.marketing.campaign_test", map[string]any{"id": campaign.Id, "lang": body.Lang})
	common.ApiSuccess(c, nil)
}

// PostAdtySendCampaign 发送或定时发送群发（营销·发送）。要带 confirm=true 和确认框里看到的人数，
// 人数变了就拒绝，让管理员重新确认。每次都写审计。
func PostAdtySendCampaign(c *gin.Context) {
	campaign, ok := adtyCampaignByParam(c)
	if !ok {
		return
	}
	var body struct {
		Confirm    bool `json:"confirm"`
		Recipients int  `json:"recipients"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || !body.Confirm {
		common.ApiErrorT(c, "Confirm the recipients and language versions before sending")
		return
	}
	plan, err := service.PlanAdtyCampaign(campaign)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if plan.Total != body.Recipients {
		common.ApiErrorT(c, "The number of recipients has changed; check it again before sending")
		return
	}
	err = service.SendAdtyCampaign(c.Request.Context(), campaign.Id, c.GetInt("id"), time.Now())
	recordManageAudit(c, "adty.marketing.campaign_send", map[string]any{
		"id": campaign.Id, "recipients": plan.Total, "versions": plan.Versions, "sites": plan.Sites,
		"scheduled_at": campaign.ScheduledAt, "ok": err == nil,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plan)
}

// PostAdtyCancelCampaign 取消定时或还在排队的群发（营销·发送）。
func PostAdtyCancelCampaign(c *gin.Context) {
	campaign, ok := adtyCampaignByParam(c)
	if !ok {
		return
	}
	if err := service.CancelAdtyCampaign(c.Request.Context(), campaign.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.marketing.campaign_cancel", map[string]any{"id": campaign.Id})
	common.ApiSuccess(c, nil)
}

// adtyUnsubscribePage 是退订确认页（不依赖前端），按 Accept-Language 显示。
const adtyUnsubscribePage = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>%s</title></head>` +
	`<body style="font-family:system-ui,sans-serif;max-width:480px;margin:64px auto;padding:0 16px;color:#111827"><p>%s</p>%s</body></html>`

// GetAdtyMarketingUnsubscribe 打开自动邮件里的退订链接：显示一个按钮，点了才退订（邮件扫描器会预先打开链接，GET 不改状态）。
func GetAdtyMarketingUnsubscribe(c *gin.Context) {
	lang := i18n.StatedLang(c)
	token := c.Query("token")
	title := i18n.Translate(lang, i18n.MsgAdtyMarketingUnsubscribe)
	form := fmt.Sprintf(`<form method="post" action="?token=%s"><button type="submit" style="padding:8px 16px">%s</button></form>`,
		url.QueryEscape(token), html.EscapeString(title))
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", fmt.Appendf(nil, adtyUnsubscribePage,
		html.EscapeString(title), html.EscapeString(i18n.Translate(lang, i18n.MsgAdtyMarketingUnsubscribePrompt)), form))
}

// PostAdtyMarketingUnsubscribe 退订：一键退订（List-Unsubscribe-Post，RFC 8058）和确认页的按钮都走这里。
func PostAdtyMarketingUnsubscribe(c *gin.Context) {
	lang := i18n.StatedLang(c)
	if err := service.AdtyUnsubscribeByToken(c.Query("token")); err != nil {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", fmt.Appendf(nil, adtyUnsubscribePage,
			"", html.EscapeString(i18n.Translate(lang, i18n.MsgAdtyMarketingUnsubscribeInvalid)), ""))
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", fmt.Appendf(nil, adtyUnsubscribePage,
		"", html.EscapeString(i18n.Translate(lang, i18n.MsgAdtyMarketingUnsubscribeDone)), ""))
}

// adtyTriggerView 是一条自动邮件规则在后台的样子。
type adtyTriggerView struct {
	Enabled  bool              `json:"enabled"`
	Days     int               `json:"days,omitempty"`
	USD      string            `json:"usd,omitempty"`
	Subjects map[string]string `json:"subjects"`
	Bodies   map[string]string `json:"bodies"`
}

// GetAdtyMarketingTriggers 返回三条自动邮件规则（营销·查看）。
func GetAdtyMarketingTriggers(c *gin.Context) {
	cfg := adty_setting.GetMarketingSetting()
	common.ApiSuccess(c, gin.H{
		"unpaid":      adtyTriggerView{cfg.TriggerUnpaidEnabled, cfg.TriggerUnpaidDays, "", cfg.TriggerUnpaidSubjects, cfg.TriggerUnpaidBodies},
		"low_balance": adtyTriggerView{cfg.TriggerLowBalanceEnabled, 0, cfg.TriggerLowBalanceUSD, cfg.TriggerLowBalanceSubjects, cfg.TriggerLowBalanceBodies},
		"winback":     adtyTriggerView{cfg.TriggerWinbackEnabled, cfg.TriggerWinbackDays, "", cfg.TriggerWinbackSubjects, cfg.TriggerWinbackBodies},
		"batch_size":  cfg.TriggerBatchSize,
	})
}

// PutAdtyMarketingTriggers 保存自动邮件规则（营销·编辑）。打开规则前英文标题和正文必须写好。
func PutAdtyMarketingTriggers(c *gin.Context) {
	var body struct {
		Unpaid     *adtyTriggerView `json:"unpaid"`
		LowBalance *adtyTriggerView `json:"low_balance"`
		Winback    *adtyTriggerView `json:"winback"`
		BatchSize  *int             `json:"batch_size"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	values := map[string]string{}
	put := func(prefix string, view *adtyTriggerView, numberKey string) bool {
		if view == nil {
			return true
		}
		subjects, bodies := map[string]string{}, map[string]string{}
		for _, pair := range []struct {
			in, out map[string]string
			maxLen  int
		}{{view.Subjects, subjects, 300}, {view.Bodies, bodies, 100 << 10}} {
			for lang, text := range pair.in {
				if !slices.Contains(model.AdtyContentLangs, lang) || len(text) > pair.maxLen {
					return false
				}
				if strings.TrimSpace(text) != "" {
					pair.out[lang] = text
				}
			}
		}
		if view.Enabled && !service.AdtyTriggerTemplateReady(subjects, bodies) {
			common.ApiErrorT(c, "Write the English subject and body before turning this email on")
			return false
		}
		switch numberKey {
		case "days":
			if view.Days < 1 || view.Days > 3650 {
				common.ApiErrorT(c, "Invalid parameters")
				return false
			}
			values[adty_setting.MarketingKeyPrefix+prefix+"_days"] = strconv.Itoa(view.Days)
		case "usd":
			usd, err := decimal.NewFromString(strings.TrimSpace(view.USD))
			if err != nil || !usd.IsPositive() {
				common.ApiErrorT(c, "Invalid parameters")
				return false
			}
			values[adty_setting.MarketingKeyPrefix+prefix+"_usd"] = usd.String()
		}
		rawSubjects, _ := common.Marshal(subjects)
		rawBodies, _ := common.Marshal(bodies)
		values[adty_setting.MarketingKeyPrefix+prefix+"_enabled"] = strconv.FormatBool(view.Enabled)
		values[adty_setting.MarketingKeyPrefix+prefix+"_subjects"] = string(rawSubjects)
		values[adty_setting.MarketingKeyPrefix+prefix+"_bodies"] = string(rawBodies)
		return true
	}
	if !put("trigger_unpaid", body.Unpaid, "days") || !put("trigger_low_balance", body.LowBalance, "usd") || !put("trigger_winback", body.Winback, "days") {
		if !c.Writer.Written() {
			common.ApiErrorT(c, "Invalid parameters")
		}
		return
	}
	if body.BatchSize != nil {
		if *body.BatchSize < 1 || *body.BatchSize > 10000 {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		values[adty_setting.MarketingKeyPrefix+"trigger_batch_size"] = strconv.Itoa(*body.BatchSize)
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.marketing.triggers", map[string]any{"keys": len(values)})
	GetAdtyMarketingTriggers(c)
}

// GetAdtySignupSourceStats 按来源维度统计注册数、付费数、充值额、消费额（营销·查看，任务 12.5）。
// 参数：dimension（utm_source、utm_medium、utm_campaign、landing_path、referrer_host、inviter、site），
// from、to（秒级时间戳，按注册时间），site_id（不传不限）。分站明细（11.9）用 site_id 复用这个接口。
func GetAdtySignupSourceStats(c *gin.Context) {
	dimension := c.DefaultQuery("dimension", "utm_source")
	if _, ok := model.AdtySignupSourceDimensions[dimension]; !ok {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	from, _ := strconv.ParseInt(c.Query("from"), 10, 64)
	to, _ := strconv.ParseInt(c.Query("to"), 10, 64)
	filter := model.AdtySignupSourceFilter{Dimension: dimension, From: from, To: to, Limit: 200}
	if raw := c.Query("site_id"); raw != "" {
		siteID, err := strconv.Atoi(raw)
		if err != nil || siteID < 0 {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		filter.SiteId = &siteID
	}
	stats, err := model.AdtySignupSourceStats(filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"dimension": dimension, "items": stats})
}
