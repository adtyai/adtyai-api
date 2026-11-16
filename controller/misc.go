package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

func TestStatus(c *gin.Context) {
	err := model.PingDB()
	if err != nil {
		writeAccountStatusError(c, http.StatusServiceUnavailable, "Database connection failed")
		return
	}
	// 获取HTTP统计信息
	httpStats := middleware.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Server is running",
		"http_stats": httpStats,
	})
	return
}

// GetReady is the process readiness probe. It does not return configuration,
// database DSNs, or node inventory. A cached status response is not readiness.
func GetReady(c *gin.Context) {
	if err := model.PingDBImmediate(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "not ready",
		})
		return
	}
	if common.RedisEnabled {
		if err := common.PingRedis(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "not ready",
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func GetStatus(c *gin.Context) {
	// 支付方式可能查通道表，必须在配置锁之外算。
	paymentMethods := service.AdtyAvailablePaymentMethodIDs()
	// 分站 widget 要在拿配置锁之前查，避免锁里再查站点表。
	turnstileSiteKey := common.TurnstileSiteKey
	if choice := service.AdtyTurnstileFor(service.AdtySiteForRequest(c), c.Request.Host); choice.SiteKey != "" {
		turnstileSiteKey = choice.SiteKey
	}

	// 公告是一次真实的 DB 查询，同样必须在配置锁之外算（规则见上面 :69-72）：
	// `/api/status` 是匿名热路径，把数据库往返放在全局读锁里，会让保存设置的后台
	// 请求排在每个匿名页面加载后面。开关取自进程内快照，GetConsoleSetting 不拿锁，
	// 所以可以先看开关再决定要不要查。
	cs := console_setting.GetConsoleSetting()
	var publicAnnouncements []service.AdtyPublicAnnouncement
	if cs.AnnouncementsEnabled {
		publicAnnouncements = service.AdtyPublicAnnouncementsForRequest(c)
	}
	passkeySetting := system_setting.PasskeySettingsSnapshot()
	// 锁只包住读 OptionMap / 全局变量的这段，读完立刻放（下面都是拼响应，不碰配置）。
	common.OptionMapRWMutex.RLock()

	legalSetting := system_setting.GetLegalSettings()

	data := gin.H{
		"version":            common.Version,
		"start_time":         common.StartTime,
		"email_verification": common.EmailVerificationEnabled,
		"telegram_bot_name":  common.TelegramBotName,
		"theme":              "default",
		"system_name":        common.SystemName,
		"logo":               common.Logo,
		"footer_html":        common.Footer,
		"server_address":     system_setting.ServerAddress,
		"turnstile_check":    common.TurnstileCheckEnabled,
		"turnstile_site_key": turnstileSiteKey,
		"docs_link":          operation_setting.GetGeneralSetting().DocsLink,
		"quota_per_unit":     common.QuotaPerUnit,
		// 兼容旧前端：保留 display_in_currency，同时提供新的 quota_display_type
		"display_in_currency":           operation_setting.IsCurrencyDisplay(),
		"quota_display_type":            operation_setting.GetQuotaDisplayType(),
		"custom_currency_symbol":        operation_setting.GetGeneralSetting().CustomCurrencySymbol,
		"custom_currency_exchange_rate": operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate,
		"enable_batch_update":           common.BatchUpdateEnabled,
		"enable_drawing":                common.DrawingEnabled,
		"enable_task":                   common.TaskEnabled,
		"enable_data_export":            common.DataExportEnabled,
		"data_export_default_time":      common.DataExportDefaultTime,
		"default_collapse_sidebar":      common.DefaultCollapseSidebar,
		"mj_notify_enabled":             setting.MjNotifyEnabled,
		"chats":                         setting.Chats,
		"demo_site_enabled":             operation_setting.DemoSiteEnabled,
		"self_use_mode_enabled":         operation_setting.SelfUseModeEnabled,
		"register_enabled":              common.RegisterEnabled,
		"password_login_enabled":        common.PasswordLoginEnabled,
		"password_register_enabled":     common.PasswordRegisterEnabled,
		"default_use_auto_group":        setting.DefaultUseAutoGroup,

		"password_login_encryption_enabled": common.PasswordLoginEncryptionEnabled,

		"usd_exchange_rate": operation_setting.USDExchangeRate,
		"price":             operation_setting.Price,

		// 面板启用开关
		"api_info_enabled":      cs.ApiInfoEnabled,
		"uptime_kuma_enabled":   cs.UptimeKumaEnabled,
		"announcements_enabled": cs.AnnouncementsEnabled,
		"faq_enabled":           cs.FAQEnabled,

		// 模块管理配置
		"HeaderNavModules":    common.OptionMap["HeaderNavModules"],
		"SidebarModulesAdmin": common.OptionMap["SidebarModulesAdmin"],

		"passkey_login":             passkeySetting.Enabled,
		"passkey_display_name":      passkeySetting.RPDisplayName,
		"passkey_rp_id":             passkeySetting.EffectiveRPID(),
		"passkey_rp_ids":            passkeySetting.RelyingPartyIDs(),
		"passkey_allow_insecure":    passkeySetting.AllowInsecureOrigin,
		"passkey_user_verification": passkeySetting.UserVerification,
		"passkey_attachment":        passkeySetting.AttachmentPreference,
		"setup":                     constant.Setup,
		"user_agreement_enabled":    legalSetting.UserAgreement != "",
		"privacy_policy_enabled":    legalSetting.PrivacyPolicy != "",
		"log_export_enabled":        operation_setting.GetLogSetting().UserExportEnabled,
		"log_max_range_days":        operation_setting.GetLogSetting().MaxRangeDays,
		"quota_remind_enabled":      common.QuotaRemindEnabled,
		"quota_remind_threshold":    common.QuotaRemindThreshold,
		"payment_methods":           paymentMethods,
	}
	common.OptionMapRWMutex.RUnlock()

	// 根据启用状态注入可选内容
	if cs.ApiInfoEnabled {
		data["api_info"] = console_setting.GetApiInfo()
	}
	if cs.AnnouncementsEnabled {
		// 从这里起数据源是 `adty_notification_publications`，不再是
		// `console_setting.announcements`（那个 option 只作为回滚源保留）。
		// 形状保持不变：这条路由没有认证，公开面板靠它给未登录访客看公告，
		// 换成需要登录的新接口会让匿名访客看不到公告——那是行为退化。
		// 按 Host 判断站点可见性，所以分站只会读到自己的和平台级公告。
		data["announcements"] = publicAnnouncements
	}
	if cs.FAQEnabled {
		data["faq"] = console_setting.GetFAQ()
	}

	service.AdtyOverrideStatus(c, data) // 二开：分站的站名、Logo、地址，关掉第三方登录（任务 11.2）

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
	return
}

func GetNotice(c *gin.Context) {
	if service.AdtySiteForRequest(c) != nil {
		serveRevalidatedJSON(c, "")
		return
	}
	common.OptionMapRWMutex.RLock()
	notice := common.OptionMap["Notice"]
	common.OptionMapRWMutex.RUnlock()
	serveRevalidatedJSON(c, notice)
}

func GetAbout(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	about := common.OptionMap["About"]
	common.OptionMapRWMutex.RUnlock()
	serveRevalidatedJSON(c, about)
}

func GetUserAgreement(c *gin.Context) {
	serveRevalidatedJSON(c, system_setting.GetLegalSettings().UserAgreement)
}

func GetPrivacyPolicy(c *gin.Context) {
	serveRevalidatedJSON(c, system_setting.GetLegalSettings().PrivacyPolicy)
}

func GetMidjourney(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["Midjourney"],
	})
	return
}

func GetHomePageContent(c *gin.Context) {
	if service.AdtySiteForRequest(c) != nil {
		serveRevalidatedJSON(c, "")
		return
	}
	common.OptionMapRWMutex.RLock()
	homePageContent := common.OptionMap["HomePageContent"]
	common.OptionMapRWMutex.RUnlock()
	serveRevalidatedJSON(c, homePageContent)
}

func SendEmailVerification(c *gin.Context) {
	email, err := service.ValidateAccountEmail(c.Query("email"))
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}

	if err := common.VerificationStoreReady(); err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Please try again later"))
		return
	}
	// 二开：已注册的邮箱不发验证码，改发一封提醒，响应和正常情况完全一样（任务 7.3）
	siteID := service.AdtyRequestSiteID(c)
	if model.IsEmailAlreadyTaken(email, siteID) {
		service.AdtySendRegisteredReminder(c, email)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
		return
	}
	code := common.GenerateVerificationCode(6)
	if err := common.RegisterVerificationCodeWithKey(model.SiteEmailCodeKey(email, siteID), code, common.EmailVerificationPurpose); err != nil {
		// Same empty success as a registered address. A 503 here would show that the address is new.
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
		return
	}
	// The address may not belong to an account yet, so the mail follows the
	// request language.
	// 二开：站名用当前请求的站点（任务 11.3），用户还不存在
	site := service.AdtySiteForRequest(c)
	systemName := map[string]any{"SystemName": service.AdtyBrandFor(site).HTMLName()}
	subject := i18n.T(c, i18n.MsgEmailVerificationSubject, systemName)
	content := fmt.Sprintf("<p>%s</p>"+
		"<p>%s</p>"+
		"<p>%s</p>",
		i18n.T(c, i18n.MsgEmailVerificationGreeting, systemName),
		i18n.T(c, i18n.MsgEmailVerificationCode, map[string]any{"Code": code}),
		i18n.T(c, i18n.MsgEmailVerificationValidity, map[string]any{"Minutes": common.VerificationValidMinutes}))
	// 二开：异步发信，响应时间不暴露邮箱是否存在（任务 7.3）；发件人按站点（任务 11.3）
	service.AdtySendSiteEmailAsync(site, subject, email, content)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func SendPasswordResetEmail(c *gin.Context) {
	email := model.NormalizeEmail(c.Query("email"))
	if err := common.Validate.Var(email, "required,email"); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := common.VerificationStoreReady(); err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Please try again later"))
		return
	}
	username := strings.TrimSpace(c.Query("username"))
	user, err := model.ResolveUserForEmailChallenge(email, username, service.AdtyRequestSiteID(c))
	switch {
	case err == nil:
		_ = sendPasswordResetInstructions(c, user)
	case errors.Is(err, model.ErrEmailAmbiguous):
		common.ApiErrorStatus(c, http.StatusOK, common.NewMessage(model.EmailAmbiguousPrompt), gin.H{"code": model.EmailAmbiguousCode})
		return
	case errors.Is(err, model.ErrEmailNotFound):
		// Preserve the same success response for unknown addresses.
	default:
		logger.LogWarn(c.Request.Context(), common.LogText("skip password reset email for %s: %s", email, err.Error()))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// Self-service recovery issues a one-time reset link. No caller, including
// staff, receives the reset token or chooses the customer's new password.
func sendPasswordResetInstructions(c *gin.Context, user *model.User) error {
	email := model.NormalizeEmail(user.Email)
	current, err := model.GetUserById(user.Id, false)
	if err != nil || current == nil || current.Id != user.Id || current.Status != common.UserStatusEnabled {
		return common.NewMessage("Invalid parameters")
	}
	email = model.NormalizeEmail(current.Email)
	code := common.GenerateVerificationCode(0)
	if err := common.RegisterVerificationCodeWithKey(model.PasswordResetCodeKey(current), code, common.PasswordResetPurpose); err != nil {
		return err
	}
	// Derive branding and URL from SQL membership, never from the request Host.
	site := service.AdtySiteForUser(current.Id)
	link := fmt.Sprintf("%s/user/reset#email=%s&token=%s&username=%s", service.AdtySiteBaseURL(site), url.QueryEscape(email), url.QueryEscape(code), url.QueryEscape(current.Username))
	lang := current.GetSetting().Language
	if lang == "" {
		lang = i18n.StatedLang(c)
	}
	systemName := map[string]any{"SystemName": service.AdtyBrandFor(site).HTMLName()}
	linkParams := map[string]any{"Link": link}
	subject := i18n.Translate(lang, i18n.MsgEmailPasswordResetSubject, systemName)
	content := fmt.Sprintf("<p>%s</p><p>%s</p><p>%s </p><p>%s</p>",
		i18n.Translate(lang, i18n.MsgEmailPasswordResetGreeting, systemName),
		i18n.Translate(lang, i18n.MsgEmailPasswordResetLink, linkParams),
		i18n.Translate(lang, i18n.MsgEmailPasswordResetLinkFallback, linkParams),
		i18n.Translate(lang, i18n.MsgEmailPasswordResetValidity, map[string]any{"Minutes": common.VerificationValidMinutes}))
	service.AdtySendSiteEmailAsync(site, subject, email, content)
	writePasswordResetChallengeAudit(current)
	return nil
}

type PasswordResetRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Token    string `json:"token"`
	Password string `json:"password"`
}

func ResetPassword(c *gin.Context) {
	var req PasswordResetRequest
	err := common.DecodeJson(c.Request.Body, &req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req.Email = model.NormalizeEmail(req.Email)
	req.Username = strings.TrimSpace(req.Username)
	if (req.Email == "" && req.Username == "") || req.Token == "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := common.Validate.StructPartial(&model.User{Password: req.Password}, "Password"); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	siteID := service.AdtyRequestSiteID(c)
	user, err := model.ResolveUserForEmailChallenge(req.Email, req.Username, siteID)
	if err != nil {
		common.ApiErrorT(c, "Password reset link is invalid or has expired")
		return
	}
	codeKey := model.PasswordResetCodeKey(user)
	issued, ok := common.ConsumeVerificationCodeWithKey(codeKey, req.Token, common.PasswordResetPurpose)
	if !ok {
		if _, uniqueErr := model.GetUniqueUserByEmail(user.Email, siteID); uniqueErr == nil {
			legacyKey := model.SiteEmailCodeKey(user.Email, siteID)
			issued, ok = common.ConsumeVerificationCodeWithKey(legacyKey, req.Token, common.PasswordResetPurpose)
			if ok {
				codeKey = legacyKey
			}
		}
	}
	if !ok {
		common.ApiErrorT(c, "Password reset link is invalid or has expired")
		return
	}
	outcome, err := model.ResetUserPasswordByIdentity(user.Email, user.Username, req.Password, siteID)
	if outcome.UserID > 0 {
		committed, notificationFailed := outcome.State == model.PasswordResetCommitted, false
		if committed {
			lang := outcome.Language
			if lang == "" {
				lang = i18n.StatedLang(c)
			}
			notificationFailed = service.NotifyAccountSecurityChange(lang, outcome.Email, outcome.UserID, i18n.MsgSecurityEventPasswordUpdated) != nil
		}
		c.Set("role", outcome.Role)
		defer recordUserSecurityAudit(c, outcome.UserID, "user.password_change", map[string]any{
			"password_reset": true, "success": committed, "notification_failed": notificationFailed,
		})
	}
	if err != nil {
		if errors.Is(err, model.ErrEmailNotFound) || errors.Is(err, model.ErrEmailAmbiguous) {
			common.ApiErrorT(c, "Password reset link is invalid or has expired")
			return
		}
		if outcome.State == model.PasswordResetNotCommitted {
			common.RestoreVerificationCodeWithKey(codeKey, req.Token, common.PasswordResetPurpose, issued)
		}
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func writePasswordResetChallengeAudit(user *model.User) {
	if user == nil {
		return
	}
	rec := model.OfficialAuditWrite{
		UserId: user.Id, Username: user.Username, ActorRole: user.Role,
		SiteID:     model.OfficialAuditSiteIDPointer(user.SiteId),
		ObjectType: "user", ObjectID: strconv.Itoa(user.Id),
		Action: "adty.user.password_reset.challenge", Category: model.AuditCategoryOperation,
		Content: "adty.user.password_reset.challenge",
		After: map[string]any{
			"user_id": user.Id, "username": user.Username,
			"site_id": user.SiteId, "inherited_from_site_id": user.InheritedFromSiteId,
			"purpose": common.PasswordResetPurpose, "result": model.AuditResultSuccess,
		},
		Result: model.AuditResultSuccess, Success: true,
	}
	if err := model.WriteOfficialAuditLog(nil, rec); err != nil {
		common.SysLog(common.LogText("password reset challenge audit write failed: %s", err.Error()))
	}
}
