package service

import (
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// 注册即推广员、按 cookie 绑定、指纹、自邀和批量注册判断（任务 6.2、6.5）。

const (
	// AdtyReferralCookie 是推广点击写的签名 cookie。
	AdtyReferralCookie = "adty_ref"
	// AdtyDeviceCookie 是前端首次访问写的随机设备标识。
	AdtyDeviceCookie = "adty_did"
)

// SetAdtyReferralCookie 写签名 cookie：按 cookie_ttl_days，SameSite=Lax、HttpOnly，https 下加 Secure。
func SetAdtyReferralCookie(c *gin.Context, code string) error {
	value, err := BuildReferralCookie(code, time.Now())
	if err != nil {
		return err
	}
	maxAge := max(referralSetting().CookieTTLDays, 1) * 86400
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AdtyReferralCookie, value, maxAge, "/", "", secure, true)
	return nil
}

func clearAdtyReferralCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AdtyReferralCookie, "", -1, "/", "", false, true)
}

// recordAdtyFingerprint 记下一条指纹；没有 IP 也没有设备时不记。
func recordAdtyFingerprint(c *gin.Context, userId int, kind string) (ipHash, deviceHash string) {
	ipHash = HashReferralRiskValue(c.ClientIP())
	if device, err := c.Cookie(AdtyDeviceCookie); err == nil {
		deviceHash = HashReferralRiskValue(device)
	}
	if ipHash == "" && deviceHash == "" {
		return
	}
	if err := model.DB.Create(&model.AdtyUserFingerprint{
		UserId: userId, IpHash: ipHash, DeviceHash: deviceHash, Kind: kind, CreatedAt: time.Now().Unix(),
	}).Error; err != nil {
		common.SysError(common.LogText("record fingerprint failed user=%d: %s", userId, err.Error()))
	}
	return
}

// AdtyRecordLoginFingerprint 在登录统一出口调用（controller/user.go 的 writeLoginResponse）。
func AdtyRecordLoginFingerprint(c *gin.Context, userId int) {
	if userId <= 0 {
		return
	}
	recordAdtyFingerprint(c, userId, "login")
}

// AdtyAfterRegister 在注册成功、生成默认令牌之前调用（传查出来的用户，不是请求体）：
// 建推广员、记指纹、读 cookie 绑定并判断自邀和批量注册、清 cookie。出错只记日志，不影响注册。
func AdtyAfterRegister(c *gin.Context, user *model.User) {
	if user == nil || user.Id <= 0 {
		return
	}
	// 分站客户（P11）：记客户行和指纹，不建推广员、不走 ?ref= 绑定（执行层决定 27）。
	if user.SiteId > 0 {
		ipHash, deviceHash := recordAdtyFingerprint(c, user.Id, "register")
		AdtyRecordSiteCustomer(user.Id, user.SiteId, ipHash, deviceHash, AdtyAcceptedSiteCustomerTerms(c))
		adtyRecordRegisterConsent(c, user)
		if _, err := c.Cookie(AdtyReferralCookie); err == nil {
			clearAdtyReferralCookie(c)
		}
		return
	}
	if _, err := Referral.EnsureAffiliate(user.Id); err != nil {
		common.SysError(common.LogText("ensure affiliate failed user=%d: %s", user.Id, err.Error()))
	}
	ipHash, deviceHash := recordAdtyFingerprint(c, user.Id, "register")
	adtyRecordRegisterConsent(c, user) // 营销同意（任务 12.1）
	// 注册来源（任务 12.5）放在推广绑定之后写，才能记下推广人。
	defer recordAdtySignupSource(c, user, time.Now())
	raw, err := c.Cookie(AdtyReferralCookie)
	if err != nil || raw == "" || !Referral.IsEnabled() {
		return
	}
	defer clearAdtyReferralCookie(c)
	code, err := ParseReferralCookie(raw, time.Now())
	if err != nil {
		return
	}
	binding, err := Referral.BindInvitee(user.Id, code, ipHash)
	if err != nil {
		common.SysLog(common.LogText("referral bind skipped user=%d: %s", user.Id, err.Error()))
		return
	}
	checkAdtySelfInvite(binding, ipHash, deviceHash)
	checkAdtyBatchRegister(binding, ipHash)
}

// checkAdtySelfInvite：被邀请人注册时的 IP 或设备和邀请人最近 self_invite_window_days 天的指纹相同，
// 绑定照建，但记 self_invite，不计佣；两个账号都记风险原因。
func checkAdtySelfInvite(binding *model.ReferralBinding, ipHash, deviceHash string) {
	if ipHash == "" && deviceHash == "" {
		return
	}
	since := time.Now().Unix() - int64(max(referralSetting().SelfInviteWindowDays, 1))*86400
	query := model.DB.Model(&model.AdtyUserFingerprint{}).Where("user_id = ? AND created_at >= ?", binding.InviterUserId, since)
	switch {
	case ipHash != "" && deviceHash != "":
		query = query.Where("(ip_hash = ? OR device_hash = ?)", ipHash, deviceHash)
	case ipHash != "":
		query = query.Where("ip_hash = ?", ipHash)
	default:
		query = query.Where("device_hash = ?", deviceHash)
	}
	var matches int64
	if err := query.Count(&matches).Error; err != nil || matches == 0 {
		return
	}
	_ = model.DB.Model(&model.ReferralBinding{}).Where("id = ?", binding.Id).Update("risk_flag", model.ReferralRiskSelfInvite).Error
	note := "self invite: invitee shares IP or device with the inviter"
	_ = model.DB.Model(&model.ReferralAffiliate{}).Where("id = ?", binding.AffiliateId).Update("risk_note", note).Error
	_ = model.DB.Model(&model.ReferralAffiliate{}).Where("user_id = ?", binding.InviteeUserId).Update("risk_note", note).Error
}

// checkAdtyBatchRegister：同一推广员 batch_register_window_hours 小时内来自同一 IP 的绑定达到阈值，
// 冻结这个推广员的结算并记原因。阈值 0 表示关闭（上线时先关，真实 IP 验证后再开）。
func checkAdtyBatchRegister(binding *model.ReferralBinding, ipHash string) {
	cfg := referralSetting()
	if cfg.BatchRegisterThreshold <= 0 || ipHash == "" {
		return
	}
	since := time.Now().Unix() - int64(max(cfg.BatchRegisterWindowHours, 1))*3600
	var count int64
	if err := model.DB.Model(&model.ReferralBinding{}).
		Where("affiliate_id = ? AND bind_ip_hash = ? AND bound_at >= ?", binding.AffiliateId, ipHash, since).
		Count(&count).Error; err != nil || count < int64(cfg.BatchRegisterThreshold) {
		return
	}
	_ = model.DB.Model(&model.ReferralAffiliate{}).Where("id = ?", binding.AffiliateId).Updates(map[string]any{
		"settlement_enabled": false,
		"risk_reason":        model.ReferralRiskBatchRegister,
		"risk_note":          "settlement frozen: several sign-ups from one IP",
	}).Error
}
