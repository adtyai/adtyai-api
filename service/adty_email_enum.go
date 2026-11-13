package service

import (
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// 防邮箱枚举（任务 7.3）：已注册邮箱请求验证码时，接口照常回成功，但只发一封提醒，不发验证码。
// 提醒按目标邮箱限频，存在进程内（首期单实例，第八节）。

// AdtyReminderInterval 是同一个邮箱两封提醒之间的最短间隔；测试可以改短。
var AdtyReminderInterval = 10 * time.Minute

var adtyReminderSent = struct {
	sync.Mutex
	at map[string]time.Time
}{at: map[string]time.Time{}}

// AdtySendEmailAsync 在后台发信，失败只记日志。发信的耗时不再暴露邮箱是否存在。
// 测试把它换成同步记录的函数。
var AdtySendEmailAsync = func(subject, to, content string) {
	gopool.Go(func() {
		if err := common.SendEmail(subject, to, content); err != nil {
			common.SysError(common.LogText("send email to %s failed: %s", to, err.Error()))
		}
	})
}

// AdtySendSiteEmailAsync 同 AdtySendEmailAsync，发件人按站点（任务 11.3）；site 为 nil 时就是 AdtySendEmailAsync。
// 测试把 AdtySendEmailAsync 换掉时这里也跟着走同一个记录函数，发件人另由 AdtySenderFor 测。
var AdtySendSiteEmailAsync = func(site *model.AdtySite, subject, to, content string) {
	if site == nil {
		AdtySendEmailAsync(subject, to, content)
		return
	}
	adtySendFromAsync(AdtySenderFor(site), subject, to, content)
}

// adtySendFromAsync 是带发件人的异步发信；测试里换成记录函数。
var adtySendFromAsync = func(from, subject, to, content string) {
	gopool.Go(func() {
		if err := common.SendEmailFrom(from, subject, to, content); err != nil {
			common.SysError(common.LogText("send email to %s failed: %s", to, err.Error()))
		}
	})
}

// AdtySendRegisteredReminder 给已注册的邮箱发「如果你在本站有账号，请直接登录或找回密码」。
// 10 分钟内同一邮箱只发一封；返回是否真的发了（只给测试看，调用方的响应不能因此不同）。
func AdtySendRegisteredReminder(c *gin.Context, email string) bool {
	key := strings.ToLower(strings.TrimSpace(email))
	site := AdtySiteForRequest(c)
	siteID := 0
	if site != nil {
		siteID = site.Id
	}
	throttleKey := model.SiteEmailCodeKey(key, siteID)
	now := time.Now()
	adtyReminderSent.Lock()
	if last, ok := adtyReminderSent.at[throttleKey]; ok && now.Sub(last) < AdtyReminderInterval {
		adtyReminderSent.Unlock()
		return false
	}
	adtyReminderSent.at[throttleKey] = now
	for addr, at := range adtyReminderSent.at {
		if now.Sub(at) >= AdtyReminderInterval {
			delete(adtyReminderSent.at, addr)
		}
	}
	adtyReminderSent.Unlock()

	// 按当前站点上该邮箱账号的语言发；软删除或未注册时用请求语言。
	lang := i18n.StatedLang(c)
	if user, err := model.GetUniqueUserByEmail(key, siteID); err == nil {
		if userLang := user.GetSetting().Language; userLang != "" {
			lang = userLang
		}
	}
	params := map[string]any{"SystemName": AdtyBrandFor(site).HTMLName()}
	subject := i18n.Translate(lang, i18n.MsgAdtyEmailRegisteredSubject, params)
	content := "<p>" + i18n.Translate(lang, i18n.MsgAdtyEmailRegisteredBody, params) + "</p>"
	AdtySendSiteEmailAsync(site, subject, key, content)
	return true
}

// ResetAdtyReminderThrottle 清空限频记录，只给测试用。
func ResetAdtyReminderThrottle() {
	adtyReminderSent.Lock()
	adtyReminderSent.at = map[string]time.Time{}
	adtyReminderSent.Unlock()
}
