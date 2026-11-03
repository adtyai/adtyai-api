package service

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// 注册来源统计（任务 12.5）。前端首次访问把 UTM 参数、落地页和来源页的主机名写进 cookie adty_src（30 天，
// 和 ?ref= 处理放在一起）；注册时后端读 cookie，按白名单字段、长度和字符校验后写进 adty_signup_sources。

// AdtySignupSourceCookie 是前端写的来源 cookie，值是 URL 编码过的 query string。
const AdtySignupSourceCookie = "adty_src"

// AdtySignupSourceFields 是 cookie 里认的字段。
type AdtySignupSourceFields struct {
	UtmSource    string
	UtmMedium    string
	UtmCampaign  string
	LandingPath  string
	ReferrerHost string
}

var (
	adtySourceTokenPattern = regexp.MustCompile(`^[a-z0-9._~-]{1,64}$`)
	adtySourceHostPattern  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
	adtySourcePathPattern  = regexp.MustCompile(`^/[A-Za-z0-9/._~%-]{0,254}$`)
)

// adtySourceToken：UTM 值转小写，只允许字母数字和 ._~-，最长 64；不合格的整项丢掉。
func adtySourceToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !adtySourceTokenPattern.MatchString(value) {
		return ""
	}
	return value
}

// adtySourceHost：主机名，最长 255。
func adtySourceHost(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) > 255 || !adtySourceHostPattern.MatchString(value) {
		return ""
	}
	return value
}

// adtySourcePath：本站路径（以单个 / 开头），去掉 query 和 fragment，最长 255。
func adtySourcePath(value string) string {
	value = strings.TrimSpace(value)
	value, _, _ = strings.Cut(value, "?")
	value, _, _ = strings.Cut(value, "#")
	if strings.HasPrefix(value, "//") || !adtySourcePathPattern.MatchString(value) {
		return ""
	}
	return value
}

// ParseAdtySignupSourceCookie 解析并校验 adty_src。整个 cookie 超过 2048 字节或解不开时返回空。
func ParseAdtySignupSourceCookie(raw string) AdtySignupSourceFields {
	if raw == "" || len(raw) > 2048 {
		return AdtySignupSourceFields{}
	}
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return AdtySignupSourceFields{}
	}
	values, err := url.ParseQuery(decoded)
	if err != nil {
		return AdtySignupSourceFields{}
	}
	return AdtySignupSourceFields{
		UtmSource:    adtySourceToken(values.Get("utm_source")),
		UtmMedium:    adtySourceToken(values.Get("utm_medium")),
		UtmCampaign:  adtySourceToken(values.Get("utm_campaign")),
		LandingPath:  adtySourcePath(values.Get("landing")),
		ReferrerHost: adtySourceHost(values.Get("ref_host")),
	}
}

// recordAdtySignupSource 在注册钩子里调用：读 cookie，补上推广人（P6 绑定）和所属站点，写一行，清 cookie。
// cookie 里没有来源页时，用注册请求的 Referer 主机名。出错只记日志。
func recordAdtySignupSource(c *gin.Context, user *model.User, now time.Time) {
	raw, _ := c.Cookie(AdtySignupSourceCookie)
	fields := ParseAdtySignupSourceCookie(raw)
	if fields.ReferrerHost == "" {
		if referer, err := url.Parse(c.GetHeader("Referer")); err == nil {
			fields.ReferrerHost = adtySourceHost(referer.Hostname())
		}
	}
	var inviter int
	if err := model.DB.Model(&model.ReferralBinding{}).Select("inviter_user_id").
		Where("invitee_user_id = ?", user.Id).Limit(1).Scan(&inviter).Error; err != nil {
		common.SysError(common.LogText("read referral binding for sign-up source user=%d: %s", user.Id, err.Error()))
	}
	row := model.AdtySignupSource{
		UserId: user.Id, InviterUserId: inviter, SiteId: user.SiteId,
		UtmSource: fields.UtmSource, UtmMedium: fields.UtmMedium, UtmCampaign: fields.UtmCampaign,
		LandingPath: fields.LandingPath, ReferrerHost: fields.ReferrerHost, CreatedAt: now.Unix(),
	}
	if err := model.DB.Create(&row).Error; err != nil {
		common.SysError(common.LogText("record sign-up source failed user=%d: %s", user.Id, err.Error()))
	}
	if raw != "" {
		c.SetCookie(AdtySignupSourceCookie, "", -1, "/", "", false, false)
	}
}
