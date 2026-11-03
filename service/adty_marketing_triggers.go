package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/shopspring/decimal"
)

// 自动触发邮件（任务 12.3）：主节点每小时扫一次，三条规则各自开关，只发给同意营销的人，同一用户同一规则只发一次。
// 自动邮件属于营销邮件，用 Resend 的发信接口逐封发，带退订链接和 List-Unsubscribe 头（一键退订，RFC 8058），
// 发件地址按用户所属站点取（AdtyMarketingSiteSender，分站部分依赖 11.3）。

// adtyTriggerRule 是一条规则在设置里的模板。
type adtyTriggerRule struct {
	name     string
	enabled  bool
	subjects map[string]string
	bodies   map[string]string
}

func adtyTriggerRules() []adtyTriggerRule {
	cfg := marketingSetting()
	return []adtyTriggerRule{
		{model.AdtyTriggerUnpaid, cfg.TriggerUnpaidEnabled, cfg.TriggerUnpaidSubjects, cfg.TriggerUnpaidBodies},
		{model.AdtyTriggerLowBalance, cfg.TriggerLowBalanceEnabled, cfg.TriggerLowBalanceSubjects, cfg.TriggerLowBalanceBodies},
		{model.AdtyTriggerWinback, cfg.TriggerWinbackEnabled, cfg.TriggerWinbackSubjects, cfg.TriggerWinbackBodies},
	}
}

// AdtyTriggerTemplateReady：英文标题和正文都有（其他语言兜底用英文）。
func AdtyTriggerTemplateReady(subjects, bodies map[string]string) bool {
	return strings.TrimSpace(subjects[i18n.LangEn]) != "" && strings.TrimSpace(bodies[i18n.LangEn]) != ""
}

// adtyMarketingUnsubscribeSecret 是退订链接签名的密钥。
func adtyMarketingUnsubscribeSecret() string {
	return "adty-marketing-unsubscribe:" + referralSecret()
}

// AdtyMarketingUnsubscribeToken 生成一个用户的退订令牌：用户 id 和 HMAC 签名。不过期，退订本来就该随时能用。
func AdtyMarketingUnsubscribeToken(userId int) string {
	id := strconv.Itoa(userId)
	return id + "." + common.HmacSha256(id, adtyMarketingUnsubscribeSecret())
}

var errAdtyUnsubscribeToken = errors.New("invalid unsubscribe token")

// AdtyUnsubscribeByToken 按退订令牌退订（营销邮件，系统邮件不受影响）。
func AdtyUnsubscribeByToken(token string) error {
	id, signature, ok := strings.Cut(strings.TrimSpace(token), ".")
	userId, err := strconv.Atoi(id)
	if !ok || err != nil || userId <= 0 || referralSecret() == "" {
		return errAdtyUnsubscribeToken
	}
	expected := common.HmacSha256(id, adtyMarketingUnsubscribeSecret())
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return errAdtyUnsubscribeToken
	}
	return model.MarkAdtyMarketingUserUnsubscribed(userId)
}

// adtyMarketingUnsubscribeURL 是自动邮件里的退订地址（主站地址，GET 打开确认页，POST 一键退订）。
func adtyMarketingUnsubscribeURL(userId int) string {
	return strings.TrimRight(system_setting.ServerAddress, "/") + "/api/adty/marketing/unsubscribe?token=" + AdtyMarketingUnsubscribeToken(userId)
}

// adtyUpstreamLowBalanceNoticeCovers：上游额度不足通知（service/quota.go 的 quotaReminderDue）开着、
// 这个人没关、阈值不低于营销规则的阈值时，上游那封会先发，营销这封不再发（12.3「上游那封优先」）。
func adtyUpstreamLowBalanceNoticeCovers(candidate model.AdtyTriggerCandidate, marketingThreshold int64) bool {
	if !common.QuotaRemindEnabled {
		return false
	}
	user := model.User{Setting: candidate.Setting}
	setting := user.GetSetting()
	if setting.QuotaWarningEnabled != nil && !*setting.QuotaWarningEnabled {
		return false
	}
	threshold := float64(common.QuotaRemindThreshold)
	if setting.QuotaWarningThreshold != 0 {
		threshold = setting.QuotaWarningThreshold
	}
	return threshold >= float64(marketingThreshold)
}

var adtyTriggerMu sync.Mutex

// RunAdtyMarketingTriggersOnce 跑一轮三条规则，返回发出的封数。营销总开关关着、没填 Resend Key 时什么都不做。
func RunAdtyMarketingTriggersOnce(ctx context.Context, now time.Time) (int, error) {
	adtyTriggerMu.Lock()
	defer adtyTriggerMu.Unlock()
	if adtyMarketingReady() != nil || strings.TrimSpace(AdtyResend.APIKey()) == "" {
		return 0, nil
	}
	cfg := marketingSetting()
	limit := max(cfg.TriggerBatchSize, 1)
	sent := 0
	for _, rule := range adtyTriggerRules() {
		if !rule.enabled || !AdtyTriggerTemplateReady(rule.subjects, rule.bodies) {
			continue
		}
		filter := model.AdtyTriggerCandidateFilter{Rule: rule.name, Limit: limit, Now: now.Unix()}
		var lowThreshold int64
		switch rule.name {
		case model.AdtyTriggerUnpaid:
			filter.RegisteredBefore = now.Add(-time.Duration(max(cfg.TriggerUnpaidDays, 0)) * 24 * time.Hour).Unix()
		case model.AdtyTriggerLowBalance:
			usd, err := decimal.NewFromString(strings.TrimSpace(cfg.TriggerLowBalanceUSD))
			if err != nil || !usd.IsPositive() {
				continue
			}
			lowThreshold = usd.Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Floor().IntPart()
			filter.BalanceBelowQuota = lowThreshold
		case model.AdtyTriggerWinback:
			filter.IdleSince = now.Add(-time.Duration(max(cfg.TriggerWinbackDays, 1)) * 24 * time.Hour).Unix()
		}
		candidates, err := model.AdtyTriggerCandidates(filter)
		if err != nil {
			return sent, err
		}
		for _, candidate := range candidates {
			if rule.name == model.AdtyTriggerLowBalance && adtyUpstreamLowBalanceNoticeCovers(candidate, lowThreshold) {
				continue
			}
			ok, err := sendAdtyTriggerEmail(ctx, rule, candidate, now)
			if err != nil {
				if errors.Is(err, ErrAdtyResendRateLimited) || errors.Is(err, ErrAdtyResendQuotaExceeded) {
					return sent, nil
				}
				common.SysError(common.LogText("marketing trigger %s user=%d failed: %s", rule.name, candidate.UserId, err.Error()))
				continue
			}
			if ok {
				sent++
			}
		}
	}
	return sent, nil
}

// sendAdtyTriggerEmail 发一封自动邮件。先占住发送记录，已经发过返回 false。
func sendAdtyTriggerEmail(ctx context.Context, rule adtyTriggerRule, candidate model.AdtyTriggerCandidate, now time.Time) (bool, error) {
	claimed, err := model.ClaimAdtyTriggerSend(rule.name, candidate.UserId, now.Unix())
	if err != nil || !claimed {
		return false, err
	}
	lang := adtyMarketingLang(candidate.Lang)
	version := lang
	if strings.TrimSpace(rule.subjects[lang]) == "" || strings.TrimSpace(rule.bodies[lang]) == "" {
		version = i18n.LangEn
	}
	balance := decimal.NewFromInt(candidate.Quota).Div(decimal.NewFromFloat(common.QuotaPerUnit)).StringFixed(2)
	body := rule.bodies[version]
	for variable, value := range map[string]string{
		"{{username}}": html.EscapeString(candidate.Username), "{{用户名}}": html.EscapeString(candidate.Username),
		"{{balance}}": "$" + balance, "{{余额}}": "$" + balance,
	} {
		body = strings.ReplaceAll(body, variable, value)
	}
	sender := AdtyMarketingSiteSender(candidate.SiteId)
	unsubscribe := adtyMarketingUnsubscribeURL(candidate.UserId)
	emailID, sendErr := AdtyResend.SendEmail(ctx, AdtyResendEmail{
		From: sender.From, To: []string{candidate.Email}, Subject: rule.subjects[version],
		HTML: body + adtyMarketingFooter(version, sender.SiteName, html.EscapeString(unsubscribe)),
		Headers: map[string]string{
			"List-Unsubscribe":      "<" + unsubscribe + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		},
		Tags:           []AdtyResendTag{{Name: "adty_trigger", Value: rule.name}},
		IdempotencyKey: fmt.Sprintf("adty-trigger-%s-%d", rule.name, candidate.UserId),
	})
	adtyMarketingSleep(adtyMarketingRequestGap())
	if err := model.FinishAdtyTriggerSend(rule.name, candidate.UserId, emailID, sendErr == nil); err != nil {
		return false, err
	}
	return sendErr == nil, sendErr
}

var adtyMarketingTriggerOnce sync.Once

// StartAdtyMarketingTriggers 只在主节点启动：每小时跑一轮自动触发邮件。
func StartAdtyMarketingTriggers() {
	if !common.IsMasterNode {
		return
	}
	adtyMarketingTriggerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				if _, err := RunAdtyMarketingTriggersOnce(context.Background(), time.Now()); err != nil {
					common.SysError(common.LogText("marketing triggers failed: %s", err.Error()))
				}
			}
		}()
	})
}

func init() {
	adtyMasterTasks = append(adtyMasterTasks, StartAdtyMarketingTriggers)
}
