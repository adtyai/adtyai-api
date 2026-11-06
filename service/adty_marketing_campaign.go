package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 营销群发（任务 12.2）：按条件选收件人，每个站点、每种语言建一个临时 segment 和一个 broadcast，交给 Resend 发。

var (
	// ErrAdtyMarketingNotReady：营销总开关没开，或发件地址、邮件底部的发件方名称和联系方式没填（CAN-SPAM）。
	ErrAdtyMarketingNotReady = common.NewMessage("Marketing emails are off or the sender and footer are not filled in")
	// ErrAdtyCampaignNeedsEnglish：英文版是没写的语言的兜底，必须有标题和正文。
	ErrAdtyCampaignNeedsEnglish = common.NewMessage("Write the English subject and body first; other languages fall back to it")
	// ErrAdtyCampaignNotEditable：已经发出或正在发的群发不能再改、再发。
	ErrAdtyCampaignNotEditable = common.NewMessage("This campaign has already been sent")
	// ErrAdtyCampaignEmpty：没有符合条件的收件人。
	ErrAdtyCampaignEmpty = common.NewMessage("No recipients match these filters")
	// ErrAdtyContactNotSendable means the person unsubscribed or was blocked after the audience snapshot.
	ErrAdtyContactNotSendable = errors.New("marketing contact is not sendable")
)

// AdtyMarketingSender 是一个站点的营销发件人。
type AdtyMarketingSender struct {
	From     string // 完整的发件人，如 ADTY AI <news@news.example.com>
	SiteName string // 邮件底部显示的站名
}

// AdtyMarketingSiteSender 是分站发件人的挂点：每个分站用分站站名和发件地址。
// 分站的站名、发件域名（11.3 站长发件）还没做，现在所有站点都返回平台营销发件地址
// adty_marketing_setting.from_address 和平台站名。11.3 做完后在分站服务里替换这个变量：
// 站长发件域名已验证并启用时返回「分站站名 <news@站长域名>」，否则返回「分站站名 <平台营销地址>」。
var AdtyMarketingSiteSender = func(siteID int) AdtyMarketingSender {
	return AdtyMarketingSender{From: marketingSetting().FromAddress, SiteName: common.SystemName}
}

// adtyMarketingReady：总开关开着、发件地址和底部信息都填了。
func adtyMarketingReady() error {
	cfg := marketingSetting()
	if !cfg.Enabled || strings.TrimSpace(cfg.FromAddress) == "" || strings.TrimSpace(cfg.FooterCompany) == "" || strings.TrimSpace(cfg.FooterAddress) == "" {
		return ErrAdtyMarketingNotReady
	}
	return nil
}

// adtyCampaignVersion 返回这个语言用哪一版：有标题和正文就用本语言，否则英文。
func adtyCampaignVersion(campaign *model.AdtyMarketingCampaign, lang string) string {
	if strings.TrimSpace(campaign.Subjects[lang]) != "" && strings.TrimSpace(campaign.Bodies[lang]) != "" {
		return lang
	}
	return i18n.LangEn
}

// 模板变量 → Resend 联系人属性。中文变量名和英文变量名都认。
var adtyCampaignVariables = map[string]string{
	"{{username}}": "{{{contact.first_name}}}",
	"{{用户名}}":      "{{{contact.first_name}}}",
	"{{balance}}":  "{{{contact." + AdtyResendPropertyBalance + "}}}",
	"{{余额}}":       "{{{contact." + AdtyResendPropertyBalance + "}}}",
}

// adtyMarketingFooter 是每封营销邮件底部的发件方、联系方式和退订链接。
func adtyMarketingFooter(lang, siteName, unsubscribeURL string) string {
	cfg := marketingSetting()
	return fmt.Sprintf(`<hr style="margin-top:32px;border:none;border-top:1px solid #e5e7eb"><p style="color:#6b7280;font-size:12px;line-height:18px">%s · %s · %s<br><a href="%s">%s</a></p>`,
		html.EscapeString(siteName), html.EscapeString(cfg.FooterCompany), html.EscapeString(cfg.FooterAddress),
		unsubscribeURL, html.EscapeString(i18n.Translate(lang, i18n.MsgAdtyMarketingUnsubscribe)))
}

// AdtyRenderCampaignHTML 把正文里的变量换成 Resend 联系人属性，并加上底部和退订链接。
func AdtyRenderCampaignHTML(body, lang, siteName string) string {
	for variable, property := range adtyCampaignVariables {
		body = strings.ReplaceAll(body, variable, property)
	}
	return body + adtyMarketingFooter(lang, siteName, "{{{RESEND_UNSUBSCRIBE_URL}}}")
}

// AdtyCampaignEstimate 是预计人数：总数和每种语言（按收件人的语言，不是邮件版本）。
type AdtyCampaignEstimate struct {
	Total  int64            `json:"total"`
	ByLang map[string]int64 `json:"by_lang"`
}

// EstimateAdtyCampaignAudience 数一下符合条件、可以发的人。
func EstimateAdtyCampaignAudience(audience model.AdtyMarketingAudience) (AdtyCampaignEstimate, error) {
	counts, err := model.AdtyMarketingAudienceCounts(audience)
	if err != nil {
		return AdtyCampaignEstimate{}, err
	}
	estimate := AdtyCampaignEstimate{ByLang: counts}
	for _, count := range counts {
		estimate.Total += count
	}
	return estimate, nil
}

// AdtyCampaignPlan 是发送前确认框显示的内容：总人数、每个邮件版本发多少人、涉及几个站点。
type AdtyCampaignPlan struct {
	Total    int            `json:"total"`
	Versions map[string]int `json:"versions"`
	Sites    int            `json:"sites"`
}

type adtyCampaignBatch struct {
	siteID int
	lang   string // 邮件版本
	rows   []model.AdtyMarketingAudienceRow
}

// adtyCampaignBatches 把收件人按「站点 × 邮件版本」分组，顺序固定。
func adtyCampaignBatches(campaign *model.AdtyMarketingCampaign, rows []model.AdtyMarketingAudienceRow) []adtyCampaignBatch {
	index := map[[2]string]int{}
	var batches []adtyCampaignBatch
	for _, row := range rows {
		version := adtyCampaignVersion(campaign, row.Lang)
		key := [2]string{fmt.Sprint(row.SiteId), version}
		i, ok := index[key]
		if !ok {
			i = len(batches)
			index[key] = i
			batches = append(batches, adtyCampaignBatch{siteID: row.SiteId, lang: version})
		}
		batches[i].rows = append(batches[i].rows, row)
	}
	slices.SortFunc(batches, func(a, b adtyCampaignBatch) int {
		if a.siteID != b.siteID {
			return a.siteID - b.siteID
		}
		return strings.Compare(a.lang, b.lang)
	})
	return batches
}

// PlanAdtyCampaign 算出发送计划，不调 Resend。
func PlanAdtyCampaign(campaign *model.AdtyMarketingCampaign) (AdtyCampaignPlan, error) {
	rows, err := model.AdtyMarketingAudienceRows(campaign.Audience)
	if err != nil {
		return AdtyCampaignPlan{}, err
	}
	plan := AdtyCampaignPlan{Total: len(rows), Versions: map[string]int{}}
	sites := map[int]bool{}
	for _, batch := range adtyCampaignBatches(campaign, rows) {
		plan.Versions[batch.lang] += len(batch.rows)
		sites[batch.siteID] = true
	}
	plan.Sites = len(sites)
	return plan, nil
}

// adtyCampaignSendable：有英文版；草稿状态。
func adtyCampaignSendable(campaign *model.AdtyMarketingCampaign) error {
	if campaign.IsTemplate || campaign.Status != model.AdtyCampaignStatusDraft {
		return ErrAdtyCampaignNotEditable
	}
	if strings.TrimSpace(campaign.Subjects[i18n.LangEn]) == "" || strings.TrimSpace(campaign.Bodies[i18n.LangEn]) == "" {
		return ErrAdtyCampaignNeedsEnglish
	}
	return nil
}

// SendAdtyCampaign 发一个群发：先把状态从 draft 改成 preparing（防止重复发），再按站点、语言建 segment、加联系人、建 broadcast。
// 收件人里还没同步到 Resend 的先同步。scheduled_at 晚于 now 时交给 Resend 定时发。中途出错时状态记 failed，已经建好的 broadcast 照样记下。
func SendAdtyCampaign(ctx context.Context, campaignID, operatorID int, now time.Time) error {
	if err := adtyMarketingReady(); err != nil {
		return err
	}
	var campaign model.AdtyMarketingCampaign
	if err := model.DB.First(&campaign, campaignID).Error; err != nil {
		return err
	}
	if err := adtyCampaignSendable(&campaign); err != nil {
		return err
	}
	rows, err := model.AdtyMarketingAudienceRows(campaign.Audience)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrAdtyCampaignEmpty
	}
	claimed := model.DB.Model(&model.AdtyMarketingCampaign{}).Where("id = ? AND status = ?", campaign.Id, model.AdtyCampaignStatusDraft).
		Updates(map[string]any{"status": model.AdtyCampaignStatusPreparing, "sent_by": operatorID, "recipients": len(rows), "error": ""})
	if claimed.Error != nil {
		return claimed.Error
	}
	if claimed.RowsAffected == 0 {
		return ErrAdtyCampaignNotEditable
	}
	scheduledAt := ""
	finalStatus := model.AdtyCampaignStatusSent
	if campaign.ScheduledAt > now.Unix() {
		scheduledAt = time.Unix(campaign.ScheduledAt, 0).UTC().Format(time.RFC3339)
		finalStatus = model.AdtyCampaignStatusScheduled
	}
	if err := sendAdtyCampaignBatches(ctx, &campaign, rows, scheduledAt); err != nil {
		message := err.Error()
		if len(message) > 250 {
			message = message[:250]
		}
		_ = model.DB.Model(&model.AdtyMarketingCampaign{}).Where("id = ?", campaign.Id).
			Updates(map[string]any{"status": model.AdtyCampaignStatusFailed, "error": message}).Error
		return err
	}
	return model.DB.Model(&model.AdtyMarketingCampaign{}).Where("id = ?", campaign.Id).
		Updates(map[string]any{"status": finalStatus, "sent_at": now.Unix()}).Error
}

func sendAdtyCampaignBatches(ctx context.Context, campaign *model.AdtyMarketingCampaign, rows []model.AdtyMarketingAudienceRow, scheduledAt string) error {
	if err := ensureAdtyResendProperties(ctx); err != nil {
		return err
	}
	cfg := marketingSetting()
	gap := adtyMarketingRequestGap()
	for _, batch := range adtyCampaignBatches(campaign, rows) {
		sender := AdtyMarketingSiteSender(batch.siteID)
		send := model.AdtyMarketingCampaignSend{CampaignId: campaign.Id, SiteId: batch.siteID, Lang: batch.lang, Recipients: len(batch.rows), Status: model.AdtyCampaignStatusPreparing}
		if err := model.DB.Create(&send).Error; err != nil {
			return err
		}
		segmentID, err := AdtyResend.CreateSegment(ctx, fmt.Sprintf("%s-c%d-s%d-%s", cfg.ResendSegmentPrefix, campaign.Id, batch.siteID, batch.lang))
		adtyMarketingSleep(gap)
		if err != nil {
			return err
		}
		send.SegmentId = segmentID
		recipients := make([]model.AdtyMarketingCampaignRecipient, 0, len(batch.rows))
		for _, row := range batch.rows {
			contactID, err := adtyEnsureResendContact(ctx, row)
			if errors.Is(err, ErrAdtyContactNotSendable) {
				continue
			}
			if err != nil {
				return err
			}
			if err := AdtyResend.AddContactToSegment(ctx, contactID, segmentID); err != nil {
				return err
			}
			adtyMarketingSleep(gap)
			recipients = append(recipients, model.AdtyMarketingCampaignRecipient{CampaignId: campaign.Id, UserId: row.UserId, SendId: send.Id, Email: row.Email})
		}
		if err := model.DB.CreateInBatches(recipients, 500).Error; err != nil {
			return err
		}
		broadcastID, err := AdtyResend.CreateBroadcast(ctx, AdtyResendBroadcast{
			SegmentID: segmentID, From: sender.From, Subject: campaign.Subjects[batch.lang],
			HTML: AdtyRenderCampaignHTML(campaign.Bodies[batch.lang], batch.lang, sender.SiteName),
			Name: fmt.Sprintf("%s #%d %s site %d", campaign.Name, campaign.Id, batch.lang, batch.siteID),
			Send: true, ScheduledAt: scheduledAt,
		})
		adtyMarketingSleep(gap)
		if err != nil {
			return err
		}
		if err := model.DB.Model(&send).Updates(map[string]any{"segment_id": segmentID, "broadcast_id": broadcastID, "status": model.AdtyCampaignStatusSent}).Error; err != nil {
			return err
		}
	}
	return nil
}

// adtyEnsureResendContact 返回收件人在 Resend 上的联系人 id；还没同步的先同步这一个人。
func adtyEnsureResendContact(ctx context.Context, row model.AdtyMarketingAudienceRow) (string, error) {
	contactNow, err := model.GetAdtyMarketingContact(row.UserId)
	if err != nil || contactNow == nil {
		return "", errors.Join(err, errors.New("marketing contact missing"))
	}
	if !contactNow.Sendable() {
		return "", ErrAdtyContactNotSendable
	}
	if row.Synced && row.ResendContactId != "" {
		return row.ResendContactId, nil
	}
	contact, err := model.GetAdtyMarketingContact(row.UserId)
	if err != nil || contact == nil {
		return "", errors.Join(err, errors.New("marketing contact missing"))
	}
	if _, err := syncAdtyMarketingContact(ctx, contact); err != nil {
		return "", err
	}
	contact, err = model.GetAdtyMarketingContact(row.UserId)
	if err != nil {
		return "", err
	}
	if contact.ResendContactId == "" || !contact.Sendable() {
		return "", ErrAdtyContactNotSendable
	}
	return contact.ResendContactId, nil
}

// SendAdtyCampaignTest 用 Resend 的发信接口给一个地址发测试邮件（标题前加 [Test]）。
// 测试邮件不经过联系人，变量换成示例值，退订链接指向站点首页。
func SendAdtyCampaignTest(ctx context.Context, campaignID int, lang, to string) error {
	if err := adtyMarketingReady(); err != nil {
		return err
	}
	address, err := mail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return common.NewMessage("Invalid email address")
	}
	var campaign model.AdtyMarketingCampaign
	if err := model.DB.First(&campaign, campaignID).Error; err != nil {
		return err
	}
	if strings.TrimSpace(campaign.Subjects[i18n.LangEn]) == "" || strings.TrimSpace(campaign.Bodies[i18n.LangEn]) == "" {
		return ErrAdtyCampaignNeedsEnglish
	}
	version := adtyCampaignVersion(&campaign, adtyMarketingLang(lang))
	body := campaign.Bodies[version]
	for variable, sample := range map[string]string{"{{username}}": "Alex", "{{用户名}}": "Alex", "{{balance}}": "$12.34", "{{余额}}": "$12.34"} {
		body = strings.ReplaceAll(body, variable, sample)
	}
	sender := AdtyMarketingSiteSender(0)
	_, err = AdtyResend.SendEmail(ctx, AdtyResendEmail{
		From: sender.From, To: []string{address.Address}, Subject: "[Test] " + campaign.Subjects[version],
		HTML: body + adtyMarketingFooter(version, sender.SiteName, html.EscapeString(strings.TrimRight(system_setting.ServerAddress, "/")+"/")),
	})
	return err
}

// CancelAdtyCampaign 取消一个定时（或还在排队）的群发：逐个调 Resend 的取消接口。
func CancelAdtyCampaign(ctx context.Context, campaignID int) error {
	var campaign model.AdtyMarketingCampaign
	if err := model.DB.First(&campaign, campaignID).Error; err != nil {
		return err
	}
	if campaign.Status != model.AdtyCampaignStatusScheduled && campaign.Status != model.AdtyCampaignStatusSent {
		return common.NewMessage("Only scheduled or sending campaigns can be canceled")
	}
	var sends []model.AdtyMarketingCampaignSend
	if err := model.DB.Where("campaign_id = ? AND broadcast_id <> ''", campaign.Id).Find(&sends).Error; err != nil {
		return err
	}
	for _, send := range sends {
		if err := AdtyResend.CancelBroadcast(ctx, send.BroadcastId); err != nil {
			return err
		}
		adtyMarketingSleep(adtyMarketingRequestGap())
	}
	return model.DB.Model(&model.AdtyMarketingCampaign{}).Where("id = ?", campaign.Id).Update("status", model.AdtyCampaignStatusCanceled).Error
}

// AdtyCampaignStatsView 是一次群发的统计。送达、退信、投诉、打开、点击按 webhook 数（打开、点击是次数）；
// 退订按收件人里现在已退订的人数。
type AdtyCampaignStatsView struct {
	Recipients   int64 `json:"recipients"`
	Delivered    int64 `json:"delivered"`
	Bounced      int64 `json:"bounced"`
	Complained   int64 `json:"complained"`
	Unsubscribed int64 `json:"unsubscribed"`
	Opened       int64 `json:"opened"`
	Clicked      int64 `json:"clicked"`
}

// AdtyCampaignStats 汇总一次群发的统计。
func AdtyCampaignStats(campaignID int) (AdtyCampaignStatsView, error) {
	var stats AdtyCampaignStatsView
	if err := model.DB.Model(&model.AdtyMarketingCampaignRecipient{}).Where("campaign_id = ?", campaignID).Count(&stats.Recipients).Error; err != nil {
		return stats, err
	}
	broadcasts := model.DB.Model(&model.AdtyMarketingCampaignSend{}).Select("broadcast_id").Where("campaign_id = ? AND broadcast_id <> ''", campaignID)
	var rows []struct {
		Type  string
		Count int64
	}
	if err := model.DB.Model(&model.AdtyMarketingWebhookEvent{}).Select("type, COUNT(*) AS count").
		Where("broadcast_id IN (?)", broadcasts).Group("type").Scan(&rows).Error; err != nil {
		return stats, err
	}
	for _, row := range rows {
		switch row.Type {
		case "email.delivered":
			stats.Delivered = row.Count
		case "email.bounced":
			stats.Bounced = row.Count
		case "email.complained":
			stats.Complained = row.Count
		case "email.opened":
			stats.Opened = row.Count
		case "email.clicked":
			stats.Clicked = row.Count
		}
	}
	err := model.DB.Model(&model.AdtyMarketingCampaignRecipient{}).
		Joins("JOIN adty_marketing_contacts ON adty_marketing_contacts.user_id = adty_marketing_campaign_recipients.user_id").
		Where("adty_marketing_campaign_recipients.campaign_id = ? AND adty_marketing_contacts.unsubscribed = ?", campaignID, true).
		Count(&stats.Unsubscribed).Error
	return stats, err
}
