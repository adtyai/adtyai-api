package service

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func marketingCampaign(t *testing.T, subjects, bodies model.AdtyLangText, audience model.AdtyMarketingAudience) *model.AdtyMarketingCampaign {
	t.Helper()
	campaign := &model.AdtyMarketingCampaign{Name: "launch", Status: model.AdtyCampaignStatusDraft, Subjects: subjects, Bodies: bodies, Audience: audience, CreatedBy: 1}
	require.NoError(t, model.DB.Create(campaign).Error)
	return campaign
}

func setUserLang(t *testing.T, db *gorm.DB, userId int, lang string) {
	t.Helper()
	require.NoError(t, db.Model(&model.AdtyMarketingContact{}).Where("user_id = ?", userId).Update("lang", lang).Error)
}

func audienceUserIDs(t *testing.T, audience model.AdtyMarketingAudience) []int {
	t.Helper()
	rows, err := model.AdtyMarketingAudienceRows(audience)
	require.NoError(t, err)
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserId)
	}
	return ids
}

func TestCampaignAudienceExcludesUnsubscribed(t *testing.T) {
	db, _ := prepareMarketingDB(t)
	keep := marketingUser(t, db, "cmp-keep", true)
	unsub := marketingUser(t, db, "cmp-unsub", true)
	bounced := marketingUser(t, db, "cmp-bounced", true)
	marketingUser(t, db, "cmp-noconsent", false)
	disabled := marketingUser(t, db, "cmp-disabled", true)
	_, err := model.MarkAdtyMarketingByEmail(unsub.Email, model.AdtyMarketingFlagUnsubscribed)
	require.NoError(t, err)
	_, err = model.MarkAdtyMarketingByEmail(bounced.Email, model.AdtyMarketingFlagBounced)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", disabled.Id).Update("status", common.UserStatusDisabled).Error)

	assert.Equal(t, []int{keep.Id}, audienceUserIDs(t, model.AdtyMarketingAudience{}))
	estimate, err := EstimateAdtyCampaignAudience(model.AdtyMarketingAudience{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, estimate.Total)

	// 余额、充值、分组筛选。
	rich := marketingUser(t, db, "cmp-rich", true)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", rich.Id).Updates(map[string]any{"quota": 10 * 500000, "quota_pending": 90 * 500000, "group": "vip"}).Error)
	require.NoError(t, db.Create(&model.TopUp{UserId: rich.Id, TradeNo: "cmp-t1", Status: common.TopUpStatusSuccess}).Error)
	paid := true
	assert.Equal(t, []int{rich.Id}, audienceUserIDs(t, model.AdtyMarketingAudience{ToppedUp: &paid}))
	assert.Equal(t, []int{rich.Id}, audienceUserIDs(t, model.AdtyMarketingAudience{BalanceMinUSD: "50"}))
	assert.Equal(t, []int{keep.Id}, audienceUserIDs(t, model.AdtyMarketingAudience{BalanceMaxUSD: "50"}))
	assert.Equal(t, []int{rich.Id}, audienceUserIDs(t, model.AdtyMarketingAudience{Groups: []string{"vip"}}))
}

func TestCampaignExcludesSiteCustomersByDefault(t *testing.T) {
	db, fake := prepareMarketingDB(t)
	main := marketingUser(t, db, "cmp-main", true)
	siteA := marketingUser(t, db, "cmp-site-a", true)
	siteB := marketingUser(t, db, "cmp-site-b", true)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", siteA.Id).Update("site_id", 7).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", siteB.Id).Update("site_id", 9).Error)

	assert.Equal(t, []int{main.Id}, audienceUserIDs(t, model.AdtyMarketingAudience{}))
	assert.Equal(t, []int{main.Id, siteA.Id, siteB.Id}, audienceUserIDs(t, model.AdtyMarketingAudience{IncludeSiteCustomers: true}))
	assert.Equal(t, []int{siteA.Id}, audienceUserIDs(t, model.AdtyMarketingAudience{IncludeSiteCustomers: true, SiteIds: []int{7}}))

	// 含分站客户时每个站点单独发一次，发件地址和站名来自挂点。
	oldSender := AdtyMarketingSiteSender
	t.Cleanup(func() { AdtyMarketingSiteSender = oldSender })
	AdtyMarketingSiteSender = func(siteID int) AdtyMarketingSender {
		if siteID == 7 {
			return AdtyMarketingSender{From: "Site Seven <news@seven.example>", SiteName: "Site Seven"}
		}
		return oldSender(siteID)
	}
	en := model.AdtyLangText{"en": "Hello"}
	campaign := marketingCampaign(t, en, model.AdtyLangText{"en": "<p>Hi {{username}}</p>"}, model.AdtyMarketingAudience{IncludeSiteCustomers: true})
	require.NoError(t, SendAdtyCampaign(context.Background(), campaign.Id, 1, time.Now()))
	broadcasts := fake.callsMatching(http.MethodPost, "/broadcasts")
	require.Len(t, broadcasts, 3)
	var froms []string
	for _, call := range broadcasts {
		var body map[string]any
		require.NoError(t, common.Unmarshal([]byte(call.Body), &body))
		froms = append(froms, body["from"].(string))
	}
	slices.Sort(froms)
	assert.Equal(t, []string{"ADTY AI <news@news.example.com>", "ADTY AI <news@news.example.com>", "Site Seven <news@seven.example>"}, froms)
}

func TestCampaignPerLanguageFallbackEnglish(t *testing.T) {
	db, fake := prepareMarketingDB(t)
	enUser := marketingUser(t, db, "cmp-en", true)
	jaUser := marketingUser(t, db, "cmp-ja", true)
	zhUser := marketingUser(t, db, "cmp-zh", true)
	setUserLang(t, db, jaUser.Id, "ja")
	setUserLang(t, db, zhUser.Id, "zh-CN")

	campaign := marketingCampaign(t,
		model.AdtyLangText{"en": "News", "zh-CN": "新闻"},
		model.AdtyLangText{"en": "<p>Hi {{username}}, balance {{balance}}</p>", "zh-CN": "<p>{{用户名}}，余额 {{余额}}</p>"},
		model.AdtyMarketingAudience{})
	estimate, err := EstimateAdtyCampaignAudience(model.AdtyMarketingAudience{})
	require.NoError(t, err)
	assert.EqualValues(t, 3, estimate.Total)

	plan, err := PlanAdtyCampaign(campaign)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"en": 2, "zh-CN": 1}, plan.Versions, "Japanese readers get the English version")

	require.NoError(t, SendAdtyCampaign(context.Background(), campaign.Id, 1, time.Now()))
	broadcasts := fake.callsMatching(http.MethodPost, "/broadcasts")
	require.Len(t, broadcasts, 2)
	subjects := map[string]string{}
	for _, call := range broadcasts {
		var body map[string]any
		require.NoError(t, common.Unmarshal([]byte(call.Body), &body))
		html := body["html"].(string)
		subjects[body["subject"].(string)] = html
		assert.Contains(t, html, "{{{RESEND_UNSUBSCRIBE_URL}}}")
		assert.Contains(t, html, "ADTY Ltd")
		assert.Contains(t, html, "1 Example Road")
		assert.Equal(t, true, body["send"])
		assert.NotContains(t, html, "{{username}}")
		assert.NotContains(t, html, "{{用户名}}")
	}
	require.Contains(t, subjects, "News")
	require.Contains(t, subjects, "新闻")
	assert.Contains(t, subjects["News"], "{{{contact.first_name}}}")
	assert.Contains(t, subjects["新闻"], "{{{contact.balance}}}")

	// 每个人进了对应语言的 segment。
	assert.Len(t, fake.callsMatching(http.MethodPost, "/segments"), 2)
	adds := 0
	for _, call := range fake.callsMatching(http.MethodPost, "/contacts/") {
		if strings.Contains(call.Path, "/segments/") {
			adds++
		}
	}
	assert.Equal(t, 3, adds)

	var saved model.AdtyMarketingCampaign
	require.NoError(t, db.First(&saved, campaign.Id).Error)
	assert.Equal(t, model.AdtyCampaignStatusSent, saved.Status)
	assert.Equal(t, 3, saved.Recipients)
	var recipients int64
	require.NoError(t, db.Model(&model.AdtyMarketingCampaignRecipient{}).Where("campaign_id = ?", campaign.Id).Count(&recipients).Error)
	assert.EqualValues(t, 3, recipients)
	_ = enUser

	// 同一个群发不能再发一次。
	require.Error(t, SendAdtyCampaign(context.Background(), campaign.Id, 1, time.Now()))
}

func TestCampaignRequiresFooter(t *testing.T) {
	db, fake := prepareMarketingDB(t)
	marketingUser(t, db, "cmp-footer", true)
	en := model.AdtyLangText{"en": "Hello"}
	body := model.AdtyLangText{"en": "<p>Hi</p>"}

	adty_setting.GetMarketingSetting().FooterAddress = ""
	campaign := marketingCampaign(t, en, body, model.AdtyMarketingAudience{})
	require.ErrorIs(t, SendAdtyCampaign(context.Background(), campaign.Id, 1, time.Now()), ErrAdtyMarketingNotReady)
	require.ErrorIs(t, SendAdtyCampaignTest(context.Background(), campaign.Id, "en", "me@example.com"), ErrAdtyMarketingNotReady)
	adty_setting.GetMarketingSetting().FooterAddress = "1 Example Road"

	// 没有英文版不能发（英文是兜底）。
	noEnglish := marketingCampaign(t, model.AdtyLangText{"fr": "Bonjour"}, model.AdtyLangText{"fr": "<p>Salut</p>"}, model.AdtyMarketingAudience{})
	require.ErrorIs(t, SendAdtyCampaign(context.Background(), noEnglish.Id, 1, time.Now()), ErrAdtyCampaignNeedsEnglish)

	// 营销总开关关着时不发。
	adty_setting.GetMarketingSetting().Enabled = false
	require.ErrorIs(t, SendAdtyCampaign(context.Background(), campaign.Id, 1, time.Now()), ErrAdtyMarketingNotReady)
	assert.Empty(t, fake.callsMatching(http.MethodPost, "/broadcasts"))
}

func TestCampaignScheduleAndCancel(t *testing.T) {
	db, fake := prepareMarketingDB(t)
	marketingUser(t, db, "cmp-sched", true)
	now := time.Now()
	campaign := marketingCampaign(t, model.AdtyLangText{"en": "Later"}, model.AdtyLangText{"en": "<p>Soon</p>"}, model.AdtyMarketingAudience{})
	require.NoError(t, db.Model(campaign).Update("scheduled_at", now.Add(2*time.Hour).Unix()).Error)
	require.NoError(t, SendAdtyCampaign(context.Background(), campaign.Id, 1, now))
	broadcasts := fake.callsMatching(http.MethodPost, "/broadcasts")
	require.Len(t, broadcasts, 1)
	var body map[string]any
	require.NoError(t, common.Unmarshal([]byte(broadcasts[0].Body), &body))
	assert.Equal(t, now.Add(2*time.Hour).UTC().Format(time.RFC3339), body["scheduled_at"])

	var saved model.AdtyMarketingCampaign
	require.NoError(t, db.First(&saved, campaign.Id).Error)
	assert.Equal(t, model.AdtyCampaignStatusScheduled, saved.Status)

	require.NoError(t, CancelAdtyCampaign(context.Background(), campaign.Id))
	cancels := fake.callsMatching(http.MethodPost, "/broadcasts/")
	require.Len(t, cancels, 1)
	assert.True(t, strings.HasSuffix(cancels[0].Path, "/cancel"))
	require.NoError(t, db.First(&saved, campaign.Id).Error)
	assert.Equal(t, model.AdtyCampaignStatusCanceled, saved.Status)
}

func TestFailedCampaignCanCancelPreviouslyScheduledBatches(t *testing.T) {
	db, fake := prepareMarketingDB(t)
	marketingUser(t, db, "partial-en", true)
	other := marketingUser(t, db, "partial-fr", true)
	setUserLang(t, db, other.Id, "fr")
	campaign := marketingCampaign(t, model.AdtyLangText{"en": "Later", "fr": "Plus tard"}, model.AdtyLangText{"en": "<p>Hi</p>", "fr": "<p>Salut</p>"}, model.AdtyMarketingAudience{})
	now := time.Now()
	require.NoError(t, db.Model(campaign).Update("scheduled_at", now.Add(time.Hour).Unix()).Error)
	broadcasts := 0
	fake.fail = func(request *http.Request) (int, string) {
		if request.Method == http.MethodPost && request.URL.Path == "/broadcasts" {
			broadcasts++
			if broadcasts == 2 {
				return http.StatusBadRequest, `{"message":"rejected"}`
			}
		}
		return 0, ""
	}
	require.Error(t, SendAdtyCampaign(context.Background(), campaign.Id, 1, now))
	var saved model.AdtyMarketingCampaign
	require.NoError(t, db.First(&saved, campaign.Id).Error)
	require.Equal(t, model.AdtyCampaignStatusFailed, saved.Status)
	require.NoError(t, CancelAdtyCampaign(context.Background(), campaign.Id))
	cancels := fake.callsMatching(http.MethodPost, "/broadcasts/")
	require.Len(t, cancels, 1)
	assert.True(t, strings.HasSuffix(cancels[0].Path, "/cancel"))
	require.NoError(t, db.First(&saved, campaign.Id).Error)
	assert.Equal(t, model.AdtyCampaignStatusCanceled, saved.Status)
}

func TestCampaignStatsFromWebhooks(t *testing.T) {
	db, _ := prepareMarketingDB(t)
	user := marketingUser(t, db, "cmp-stats", true)
	campaign := marketingCampaign(t, model.AdtyLangText{"en": "Stats"}, model.AdtyLangText{"en": "<p>x</p>"}, model.AdtyMarketingAudience{})
	require.NoError(t, SendAdtyCampaign(context.Background(), campaign.Id, 1, time.Now()))
	var send model.AdtyMarketingCampaignSend
	require.NoError(t, db.Where("campaign_id = ?", campaign.Id).First(&send).Error)
	require.NotEmpty(t, send.BroadcastId)

	now := time.Now()
	events := []struct{ id, kind string }{
		{"s1", "email.delivered"}, {"s2", "email.opened"}, {"s3", "email.opened"}, {"s4", "email.clicked"}, {"s5", "email.bounced"},
	}
	for _, event := range events {
		body := `{"type":"` + event.kind + `","data":{"email_id":"e","broadcast_id":"` + send.BroadcastId + `","to":["other@example.com"],"bounce":{"type":"Permanent"}}}`
		require.NoError(t, HandleAdtyResendWebhook(signedWebhook(t, event.id, now, body), []byte(body), now))
	}
	_, err := model.MarkAdtyMarketingByEmail(user.Email, model.AdtyMarketingFlagUnsubscribed)
	require.NoError(t, err)

	stats, err := AdtyCampaignStats(campaign.Id)
	require.NoError(t, err)
	assert.Equal(t, AdtyCampaignStatsView{Recipients: 1, Delivered: 1, Opened: 2, Clicked: 1, Bounced: 1, Complained: 0, Unsubscribed: 1}, stats)
}
