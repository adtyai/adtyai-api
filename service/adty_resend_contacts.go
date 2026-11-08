package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 营销邮件的同意、退订、联系人同步和 Resend webhook（任务 12.1）。

// ErrAdtyMarketingWebhookSignature：webhook 签名不对、过期，或者后台还没填签名密钥。
var ErrAdtyMarketingWebhookSignature = errors.New("invalid marketing webhook signature")

// adtyMarketingSleep 在两次 Resend 请求之间等待，测试里换成不等。
var adtyMarketingSleep = time.Sleep

// adtyResendPropertiesReady 表示联系人自定义属性已经在 Resend 上建好（进程内只建一次）。
var adtyResendPropertiesReady atomic.Bool

// Resend 联系人上的自定义属性，群发模板里的 {{username}}、{{balance}} 映射到它们（任务 12.2）。
const (
	AdtyResendPropertyUsername = "username"
	AdtyResendPropertyBalance  = "balance"
)

func marketingSetting() *adty_setting.MarketingSetting {
	return adty_setting.GetMarketingSetting()
}

// AdtyConsentDefault：注册页的营销同意框是否默认勾上。按 CF-IPCountry 判断；
// 欧盟（含后台设的 eu_countries）、拿不到国家（空、XX、T1）一律不勾，其余地区按 consent_default_non_eu。
func AdtyConsentDefault(country string) bool {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" || country == "XX" || country == "T1" {
		return false
	}
	cfg := marketingSetting()
	if cfg.IsEUCountry(country) {
		return false
	}
	return cfg.ConsentDefaultNonEU
}

// adtyMarketingLang 把界面语言（zhCN、zh-CN、en…）换成内容语言，不认识的当英文。
func adtyMarketingLang(lang string) string {
	// ParseAcceptLanguage 用的是 i18n 包的规范化（认 zhCN、zhTW、zh-Hant、en-US 等），不认识的给 en。
	if normalized := i18n.ParseAcceptLanguage(lang); normalized != "" {
		return normalized
	}
	return i18n.LangEn
}

// AdtySetMarketingConsent 写一个用户的营销同意（注册页或个人设置）。邮箱、所属站点从用户表取。
func AdtySetMarketingConsent(userId int, consent bool, ipHash, lang, source string) (*model.AdtyMarketingContact, error) {
	var user model.User
	if err := model.DB.Select("id", "email", "site_id").Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}
	return model.SaveAdtyMarketingConsent(model.AdtyMarketingConsentInput{
		UserId: user.Id, Email: user.Email, Consent: consent, IpHash: ipHash, Lang: adtyMarketingLang(lang),
		Version: marketingSetting().ConsentVersion, Source: source, SiteId: user.SiteId,
	})
}

// adtyRecordRegisterConsent 在注册钩子里调用：前端在注册请求的 query 里带 marketing_consent=true 和 lang。
// 没勾的也记一行（Consent=false），以后在个人设置里改。出错只记日志。
func adtyRecordRegisterConsent(c *gin.Context, user *model.User) {
	consent := c.Query("marketing_consent") == "true"
	lang := c.Query("lang")
	if lang == "" {
		lang = i18n.StatedLang(c)
	}
	if _, err := AdtySetMarketingConsent(user.Id, consent, HashReferralRiskValue(c.ClientIP()), lang, "register"); err != nil {
		common.SysError(common.LogText("record marketing consent failed user=%d: %s", user.Id, err.Error()))
	}
}

// adtyMarketingProperties 是同步到 Resend 联系人上的属性。余额按美元、保留 2 位。
func adtyMarketingProperties(user *model.User) map[string]any {
	balance := decimal.NewFromInt(int64(user.ToBaseUser().Quota)).Div(decimal.NewFromFloat(common.QuotaPerUnit))
	return map[string]any{
		AdtyResendPropertyUsername: user.Username,
		AdtyResendPropertyBalance:  "$" + balance.StringFixed(2),
	}
}

// ensureAdtyResendProperties 在 Resend 上建好联系人自定义属性。已存在时 Resend 返回 4xx 校验错误，当作成功。
func ensureAdtyResendProperties(ctx context.Context) error {
	if adtyResendPropertiesReady.Load() {
		return nil
	}
	for _, key := range []string{AdtyResendPropertyUsername, AdtyResendPropertyBalance} {
		_, err := AdtyResend.CreateContactProperty(ctx, key, "string", "")
		adtyMarketingSleep(adtyMarketingRequestGap())
		var apiErr *AdtyResendError
		if err != nil && !(errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusConflict || apiErr.StatusCode == http.StatusUnprocessableEntity)) {
			return err
		}
	}
	adtyResendPropertiesReady.Store(true)
	return nil
}

// adtyMarketingRequestGap 是两次 Resend 请求之间的间隔，按 api_rate_per_second。
func adtyMarketingRequestGap() time.Duration {
	rate := max(marketingSetting().APIRatePerSecond, 1)
	return time.Second / time.Duration(rate)
}

func adtyResendNotFound(err error) bool {
	var apiErr *AdtyResendError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

var adtyMarketingSyncMu sync.Mutex

// SyncAdtyMarketingContactsOnce 把本地状态变了的联系人（最多 sync_batch_size 个）同步到 Resend：
// 可发的建或更新，不可发的（没同意、退订、退信、投诉、账号停用或删除）从 Resend 删掉。
// 返回调过 Resend 的联系人数。营销总开关关着或没填 API Key 时什么都不做。遇到 429 停在这一轮，下一轮接着。
func SyncAdtyMarketingContactsOnce(ctx context.Context) (int, error) {
	adtyMarketingSyncMu.Lock()
	defer adtyMarketingSyncMu.Unlock()
	cfg := marketingSetting()
	if !cfg.Enabled || strings.TrimSpace(AdtyResend.APIKey()) == "" {
		return 0, nil
	}
	batch := max(cfg.SyncBatchSize, 1)
	if err := model.RefreshAdtyMarketingContacts(batch); err != nil {
		return 0, err
	}
	rows, err := model.AdtyMarketingContactsToSync(batch)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if err := ensureAdtyResendProperties(ctx); err != nil {
		return 0, err
	}
	touched := 0
	for i := range rows {
		called, err := syncAdtyMarketingContact(ctx, &rows[i])
		if called {
			touched++
		}
		if err != nil {
			if errors.Is(err, ErrAdtyResendRateLimited) {
				var apiErr *AdtyResendError
				if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
					adtyMarketingSleep(apiErr.RetryAfter)
				}
				return touched, nil
			}
			var apiErr *AdtyResendError
			if errors.As(err, &apiErr) {
				common.SysError(common.LogText("marketing contact sync user=%d failed: %s", rows[i].UserId, err.Error()))
				continue
			}
			return touched, err
		}
	}
	return touched, nil
}

// syncAdtyMarketingContact 同步一个联系人，返回是否调过 Resend。
func syncAdtyMarketingContact(ctx context.Context, contact *model.AdtyMarketingContact) (bool, error) {
	var users []model.User
	if err := model.DB.Where("id = ?", contact.UserId).Limit(1).Find(&users).Error; err != nil {
		return false, err
	}
	sendable := contact.Sendable() && len(users) == 1 && users[0].Status == common.UserStatusEnabled
	resendID, syncedEmail := contact.ResendContactId, contact.SyncedEmail
	called := false
	gap := adtyMarketingRequestGap()

	// 不可发，或 Resend 上还是旧邮箱：先删掉 Resend 上的联系人。
	if resendID != "" && (!sendable || syncedEmail != contact.Email) {
		called = true
		err := AdtyResend.DeleteContact(ctx, resendID)
		adtyMarketingSleep(gap)
		if err != nil && !adtyResendNotFound(err) {
			return called, err
		}
		resendID, syncedEmail = "", ""
	}
	if sendable {
		user := &users[0]
		properties := adtyMarketingProperties(user)
		unsubscribed := false
		if resendID != "" {
			called = true
			err := AdtyResend.UpdateContact(ctx, resendID, AdtyResendContactUpdate{FirstName: &user.Username, Unsubscribed: &unsubscribed, Properties: properties})
			adtyMarketingSleep(gap)
			if adtyResendNotFound(err) {
				resendID, err = "", nil
			}
			if err != nil {
				return called, err
			}
		}
		if resendID == "" {
			called = true
			id, err := AdtyResend.CreateContact(ctx, AdtyResendContact{Email: contact.Email, FirstName: user.Username, Properties: properties})
			adtyMarketingSleep(gap)
			if err != nil {
				var apiErr *AdtyResendError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
					return called, err
				}
				// 上次创建已经成功、本地还没记下 id 时，Resend 会回冲突。改按邮箱更新，别让这一行永远占住队首。
				err = AdtyResend.UpdateContact(ctx, contact.Email, AdtyResendContactUpdate{FirstName: &user.Username, Unsubscribed: &unsubscribed, Properties: properties})
				adtyMarketingSleep(gap)
				if err != nil {
					return called, err
				}
				id = contact.Email
			}
			resendID, syncedEmail = id, contact.Email
		}
	}
	return called, model.MarkAdtyMarketingContactSynced(contact.UserId, contact.UpdatedAt, resendID, syncedEmail)
}

// adtyResendWebhook 是 Resend webhook 的外壳和用到的 data 字段。
type adtyResendWebhook struct {
	Type string `json:"type"`
	Data struct {
		ID           string   `json:"id"`
		Email        string   `json:"email"`
		Unsubscribed bool     `json:"unsubscribed"`
		EmailID      string   `json:"email_id"`
		BroadcastID  string   `json:"broadcast_id"`
		To           []string `json:"to"`
		Bounce       struct {
			Type string `json:"type"`
		} `json:"bounce"`
	} `json:"data"`
}

// HandleAdtyResendWebhook 验签并处理一个 Resend webhook。body 必须是原始字节。
// contact.updated 里 unsubscribed 变 true → 退订；email.bounced（Transient 以外）→ 退信；email.complained → 投诉。
// 都是永久标记，以后永不再发。带 broadcast_id 的邮件事件记下来给群发统计用。同一个 svix-id 只处理一次。
func HandleAdtyResendWebhook(headers http.Header, body []byte, now time.Time) error {
	secret := strings.TrimSpace(marketingSetting().WebhookSecret)
	if secret == "" || AdtyVerifySvixSignature(secret, headers, body, now) != nil {
		return ErrAdtyMarketingWebhookSignature
	}
	var event adtyResendWebhook
	if err := common.Unmarshal(body, &event); err != nil {
		return err
	}
	type mark struct {
		email string
		flag  model.AdtyMarketingFlag
	}
	var marks []mark
	switch event.Type {
	case "contact.updated":
		if event.Data.Unsubscribed {
			marks = append(marks, mark{event.Data.Email, model.AdtyMarketingFlagUnsubscribed})
		}
	case "email.bounced":
		if !strings.EqualFold(event.Data.Bounce.Type, "Transient") {
			for _, to := range event.Data.To {
				marks = append(marks, mark{to, model.AdtyMarketingFlagBounced})
			}
		}
	case "email.complained":
		for _, to := range event.Data.To {
			marks = append(marks, mark{to, model.AdtyMarketingFlagComplained})
		}
	}
	if len(marks) == 0 && event.Data.BroadcastID == "" {
		return nil
	}
	email := event.Data.Email
	if len(event.Data.To) > 0 {
		email = event.Data.To[0]
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		fresh, err := model.RecordAdtyMarketingWebhookEventTx(tx, model.AdtyMarketingWebhookEvent{
			Id: headers.Get("svix-id"), Type: event.Type, BroadcastId: event.Data.BroadcastID, EmailId: event.Data.EmailID,
			Email: strings.ToLower(strings.TrimSpace(email)), CreatedAt: now.Unix(),
		})
		if err != nil || !fresh {
			return err
		}
		for _, m := range marks {
			if _, err := model.MarkAdtyMarketingByEmailTx(tx, m.email, m.flag); err != nil {
				return err
			}
		}
		return nil
	})
}

var adtyMarketingSyncOnce sync.Once

// StartAdtyMarketingSync 只在主节点启动：每 sync_interval_seconds 秒同步一轮联系人。
func StartAdtyMarketingSync() {
	if !common.IsMasterNode {
		return
	}
	adtyMarketingSyncOnce.Do(func() {
		go func() {
			for {
				if _, err := SyncAdtyMarketingContactsOnce(context.Background()); err != nil {
					common.SysError(common.LogText("marketing contact sync failed: %s", err.Error()))
				}
				time.Sleep(time.Duration(max(marketingSetting().SyncIntervalSeconds, 10)) * time.Second)
			}
		}()
	})
}

func init() {
	adtyMasterTasks = append(adtyMasterTasks, StartAdtyMarketingSync)
}
