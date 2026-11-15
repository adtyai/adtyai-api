package controller

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	oauthAuthFlowTTL     = 10 * time.Minute
	oauthLoginCookieName = "adtyai_oauth_login"
)

type oauthStateRequest struct {
	Provider         string          `json:"provider"`
	Intent           string          `json:"intent"`
	Scope            string          `json:"scope,omitempty"`
	Context          json.RawMessage `json:"context,omitempty"`
	SiteTermsVersion string          `json:"site_terms_version,omitempty"`
}

type oauthFlowPayload struct {
	BrowserBindingHash string                         `json:"browser_binding_hash,omitempty"`
	Verification       *service.OAuthVerificationFlow `json:"verification,omitempty"`
	SessionIdentity    *service.AuthIdentity          `json:"session_identity,omitempty"`
	Authorization      *model.AuthFlowAuthorization   `json:"authorization,omitempty"`
	SiteID             int                            `json:"site_id,omitempty"`
	SiteOrigin         string                         `json:"site_origin,omitempty"`
	SiteTermsVersion   string                         `json:"site_terms_version,omitempty"`
}

// providerParams returns the params of messages that name the OAuth provider.
func providerParams(name string) map[string]any {
	return map[string]any{"provider": name}
}

// GenerateOAuthCode generates a state code for OAuth CSRF protection
func GenerateOAuthCode(c *gin.Context) {
	var request oauthStateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	request.Provider = strings.TrimSpace(request.Provider)
	request.Intent = strings.TrimSpace(request.Intent)
	// 约定 I：Telegram 只是通知通道，OAuth 登录链已退役；这里直接拒绝，避免
	// 任何“先填凭据再打开开关”的路径把它重新变成登录入口。
	if request.Provider == "telegram" {
		writeSecurityOperationError(c, oauth.TelegramConfigurationError())
		return
	}
	provider := oauth.GetProvider(request.Provider)
	if provider == nil || !provider.IsEnabled() ||
		(request.Intent != model.AuthFlowIntentLogin && request.Intent != model.AuthFlowIntentBind && request.Intent != model.AuthFlowIntentVerify) ||
		(request.Intent != model.AuthFlowIntentVerify && (request.Scope != "" || len(request.Context) != 0)) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	userID := 0
	sessionID := ""
	flowPayload := oauthFlowPayload{}
	// Bind only a known platform site, never a caller-supplied return URL or a
	// custom domain that is not an account API entry point.
	site := service.AdtySiteForRequest(c)
	if site == nil {
		site = service.AdtySiteForHost(c.Request.Host)
	}
	if site != nil {
		var current model.AdtySite
		if model.DB.First(&current, site.Id).Error != nil || !service.AdtySiteServesCustomers(&current) ||
			!oauthRequestMatchesOrigin(c, service.AdtySiteBaseURL(&current)) {
			writeAccountStatusError(c, http.StatusForbidden, "Invalid parameters")
			return
		}
		flowPayload.SiteID, flowPayload.SiteOrigin = current.Id, service.AdtySiteBaseURL(&current)
	} else {
		if service.AdtyRejectSessionAPIStatus(c.Request.Host) != 0 {
			writeAccountStatusError(c, http.StatusForbidden, "Invalid parameters")
			return
		}
		flowPayload.SiteOrigin = service.AdtySiteBaseURL(nil)
	}
	request.SiteTermsVersion = strings.TrimSpace(request.SiteTermsVersion)
	if request.Intent == model.AuthFlowIntentLogin && flowPayload.SiteID > 0 {
		if request.SiteTermsVersion != "" && request.SiteTermsVersion != service.AdtyCurrentSiteCustomerTermsVersion() {
			common.ApiErrorT(c, "Accept the current site customer terms first")
			return
		}
		flowPayload.SiteTermsVersion = request.SiteTermsVersion
	}
	browserToken := ""
	if request.Intent == model.AuthFlowIntentLogin {
		browserToken, _ = c.Cookie(oauthLoginCookieName)
		decoded, err := base64.RawURLEncoding.DecodeString(browserToken)
		if err != nil || len(decoded) != model.AuthFlowTokenBytes {
			random := make([]byte, model.AuthFlowTokenBytes)
			if _, err := rand.Read(random); err != nil {
				writeSecurityOperationError(c, err)
				return
			}
			browserToken = base64.RawURLEncoding.EncodeToString(random)
		}
		flowPayload.BrowserBindingHash = oauthLoginBrowserHash(browserToken)
	}
	bindingStarted := false
	if request.Intent == model.AuthFlowIntentBind || request.Intent == model.AuthFlowIntentVerify {
		identity, ok := middleware.GetSessionAuthIdentity(c)
		if !ok {
			writeAccountStatusError(c, http.StatusUnauthorized, "Sign in to bind an account")
			return
		}
		userID = identity.UserID
		sessionID = identity.SessionID
		if request.Intent == model.AuthFlowIntentBind {
			defer func() {
				recordUserSecurityAudit(c, userID, "user.binding_start", map[string]any{"provider": request.Provider, "success": bindingStarted})
			}()
			context, err := common.Marshal(service.AccountBindingContext{Provider: request.Provider})
			if err != nil {
				writeSecurityOperationError(c, err)
				return
			}
			flowPayload.Authorization = middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: context})
			if flowPayload.Authorization == nil {
				return
			}
			flowPayload.SessionIdentity = &identity
		}
		if request.Intent == model.AuthFlowIntentVerify {
			verification, err := service.StartOAuthVerification(identity, service.VerificationOperation{Scope: request.Scope, Context: request.Context}, request.Provider)
			if err != nil {
				writeSecurityOperationError(c, err)
				return
			}
			flowPayload.Verification = verification
		}
	}
	payload, err := common.Marshal(flowPayload)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	expiresAt := time.Now().Add(oauthAuthFlowTTL)
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeOAuth,
		Provider:  request.Provider,
		Intent:    request.Intent,
		UserId:    userID,
		SessionId: sessionID,
		Payload:   string(payload),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	bindingStarted = request.Intent == model.AuthFlowIntentBind
	data := gin.H{"flow_token": state, "expires_at": expiresAt.Unix(),
		"redirect_uri": strings.TrimRight(system_setting.ServerAddress, "/") + "/oauth/" + url.PathEscape(request.Provider)}
	if browserToken != "" {
		// Reuse this host-only token across concurrent login tabs. Only its HMAC
		// is kept in the immutable flow payload; a state alone cannot log in.
		http.SetCookie(c.Writer, &http.Cookie{
			Name: oauthLoginCookieName, Value: browserToken, Path: "/api/oauth",
			MaxAge: int(oauthAuthFlowTTL.Seconds()), Expires: expiresAt,
			HttpOnly: true, Secure: common.SessionCookieSecure, SameSite: http.SameSiteLaxMode,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

func oauthLoginBrowserHash(token string) string {
	return common.GenerateHMACWithKey([]byte("oauth-login-browser-v1:"+common.SessionSecret), token)
}

func oauthRequestMatchesOrigin(c *gin.Context, origin string) bool {
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && strings.EqualFold(c.Request.Host, parsed.Host)
}

// Return provider callbacks to the platform origin that owns the host-only
// browser cookie. This happens before session checks, code exchange or flow
// consumption; only the original origin may finish the authentication.
func prepareOAuthCallbackSite(c *gin.Context, payload *oauthFlowPayload, providerName, state string) bool {
	if payload.SiteID <= 0 {
		if service.AdtySiteForRequest(c) != nil || service.AdtySiteForHost(c.Request.Host) != nil || service.AdtyRejectSessionAPIStatus(c.Request.Host) != 0 {
			writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
			return false
		}
		return true
	}
	var site model.AdtySite
	if model.DB.First(&site, payload.SiteID).Error != nil || !service.AdtySiteServesCustomers(&site) ||
		payload.SiteOrigin != service.AdtySiteBaseURL(&site) {
		writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
		return false
	}
	if oauthRequestMatchesOrigin(c, payload.SiteOrigin) {
		c.Set(service.AdtySiteContextKey, &site)
		return true
	}
	if !oauthRequestMatchesOrigin(c, strings.TrimRight(system_setting.ServerAddress, "/")) {
		writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
		return false
	}
	query := url.Values{"state": {state}}
	for _, name := range []string{"code", "error", "error_description"} {
		if value := c.Query(name); value != "" {
			query.Set(name, value)
		}
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	common.ApiSuccess(c, gin.H{"oauth_handoff_url": payload.SiteOrigin + "/oauth/" + url.PathEscape(providerName) + "?" + query.Encode()})
	return false
}

// HandleOAuth handles the OAuth callback for every provider registered in the
// oauth package. Only the retired Telegram placeholder is registered today, and
// it reports itself as disabled, so this endpoint can no longer sign anyone in.
// The generic state, site, and verification checks stay in place so that the
// endpoint fails closed for any name that is not registered.
func HandleOAuth(c *gin.Context) {
	providerName := c.Param("provider")
	provider := oauth.GetProvider(providerName)
	if provider == nil {
		writeAccountStatusError(c, http.StatusBadRequest, "Unknown OAuth provider")
		return
	}

	// 1. Validate state (CSRF protection)
	state := c.Query("state")
	pendingFlow, err := model.GetAuthFlow(state, model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeOAuth,
		Provider: providerName,
	})
	if err != nil {
		writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
		return
	}
	var flowPayload oauthFlowPayload
	if common.UnmarshalJsonStr(pendingFlow.Payload, &flowPayload) != nil {
		writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
		return
	}
	if !prepareOAuthCallbackSite(c, &flowPayload, providerName, state) {
		return
	}
	// The callback query cannot grant or replace consent recorded at flow start.
	c.Set(service.AdtyOAuthSiteTermsContextKey, flowPayload.SiteTermsVersion)

	consumeMatch := model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeOAuth,
		Provider: providerName,
		Intent:   pendingFlow.Intent,
	}
	bindSucceeded, notificationFailed := false, false
	if pendingFlow.Intent == model.AuthFlowIntentBind {
		defer func() {
			recordUserSecurityAudit(c, pendingFlow.UserId, "user.binding_bind", map[string]any{"provider": providerName, "success": bindSucceeded, "notification_failed": notificationFailed})
		}()
	}
	// Bind and verification callbacks must use the dashboard session that started them.
	if pendingFlow.Intent == model.AuthFlowIntentBind || pendingFlow.Intent == model.AuthFlowIntentVerify {
		identity, ok := middleware.GetSessionAuthIdentity(c)
		if !ok || identity.UserID != pendingFlow.UserId || identity.SessionID != pendingFlow.SessionId {
			writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
			return
		}
		consumeMatch.UserId = identity.UserID
		consumeMatch.SessionId = identity.SessionID
		if pendingFlow.Intent == model.AuthFlowIntentBind {
			var payload oauthFlowPayload
			if err := common.UnmarshalJsonStr(pendingFlow.Payload, &payload); err != nil {
				writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
				return
			}
			context, err := common.Marshal(service.AccountBindingContext{Provider: providerName})
			if err != nil {
				writeSecurityOperationError(c, err)
				return
			}
			if err := service.ValidateFlowAuthorization(identity, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: context}, payload.Authorization); err != nil {
				writeSecurityOperationError(c, err)
				return
			}
		}
	} else if pendingFlow.Intent == model.AuthFlowIntentLogin {
		var payload oauthFlowPayload
		browserToken, cookieErr := c.Cookie(oauthLoginCookieName)
		if cookieErr != nil || browserToken == "" ||
			common.UnmarshalJsonStr(pendingFlow.Payload, &payload) != nil || payload.BrowserBindingHash == "" ||
			!hmac.Equal([]byte(payload.BrowserBindingHash), []byte(oauthLoginBrowserHash(browserToken))) {
			writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
			return
		}
	} else {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}

	// 3. Check if provider is enabled
	if !provider.IsEnabled() {
		common.ApiErrorT(c, "{{provider}} login and registration has not been enabled by administrator", providerParams(provider.GetName()))
		return
	}

	// 4. Handle error from provider
	errorCode := c.Query("error")
	if errorCode != "" {
		if _, err := model.ConsumeAuthFlow(state, consumeMatch); err != nil {
			writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
			return
		}
		errorDescription := c.Query("error_description")
		if errorDescription == "" {
			errorDescription = errorCode
		}
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": errorDescription,
		})
		return
	}
	// 5. Exchange code for token
	code := c.Query("code")
	token, err := provider.ExchangeToken(c.Request.Context(), code, c)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}

	// 6. Get user info
	oauthUser, err := provider.GetUserInfo(c.Request.Context(), token)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	if pendingFlow.Intent == model.AuthFlowIntentBind {
		bindSucceeded, notificationFailed = handleOAuthBind(c, providerName, provider, oauthUser, pendingFlow, state, consumeMatch)
		return
	}
	flow, err := model.ConsumeAuthFlow(state, consumeMatch)
	if err != nil {
		writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
		return
	}

	switch flow.Intent {
	case model.AuthFlowIntentLogin:
		handleOAuthLogin(c, provider, oauthUser, flow)
	case model.AuthFlowIntentVerify:
		handleOAuthVerification(c, providerName, oauthUser, flow)
	}
}

func handleOAuthVerification(c *gin.Context, provider string, oauthUser *oauth.OAuthUser, flow *model.AuthFlow) {
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	identity, _ := middleware.GetSessionAuthIdentity(c)
	proof, err := service.FinishOAuthVerification(identity, provider, oauthUser.ProviderUserID, payload.Verification)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	recordUserSecurityAudit(c, identity.UserID, "user.security_verify", map[string]any{"method": proof.Method, "scope": proof.Scope, "provider": provider})
	common.ApiSuccess(c, proof)
}

func handleOAuthLogin(c *gin.Context, provider oauth.Provider, oauthUser *oauth.OAuthUser, flow *model.AuthFlow) {
	// 7. Find or create user
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	user, err := findOrCreateOAuthUser(c, provider, oauthUser)
	if err != nil {
		if errors.Is(err, model.ErrEmailAlreadyTaken) {
			common.ApiErrorT(c, "Email address is already in use")
			return
		}
		switch err.(type) {
		case *OAuthUserDeletedError:
			common.ApiErrorT(c, "User has been deleted")
		case *OAuthRegistrationDisabledError:
			common.ApiErrorT(c, "New user registration has been disabled by administrator")
		case *OAuthSiteTermsError:
			common.ApiErrorT(c, "Accept the current site customer terms first")
		case *OAuthEmailAlreadyTakenError:
			common.ApiErrorT(c, "Email address is already in use")
		default:
			writeSecurityOperationError(c, err)
		}
		return
	}

	// 8. Check user status
	if user.Status != common.UserStatusEnabled {
		common.ApiErrorT(c, "User has been banned")
		return
	}

	// 9. Setup login
	setupLogin(user, c)
}

// handleOAuthBind handles binding an external account to the signed-in user.
func handleOAuthBind(c *gin.Context, providerName string, provider oauth.Provider, oauthUser *oauth.OAuthUser, flow *model.AuthFlow, state string, match model.AuthFlowMatch) (bool, bool) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return false, false
	}
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
		return false, false
	}
	context, err := common.Marshal(service.AccountBindingContext{Provider: providerName})
	if err != nil {
		writeSecurityOperationError(c, err)
		return false, false
	}
	// Recheck after the external provider round trip, then validate the session
	// under the transaction's locks before consuming the flow and writing.
	if err := service.ValidateFlowAuthorization(identity, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: context}, payload.Authorization); err != nil {
		writeSecurityOperationError(c, err)
		return false, false
	}
	if provider.IsUserIDTaken(oauthUser.ProviderUserID, service.AdtyRequestSiteID(c)) {
		common.ApiErrorT(c, "This {{provider}} account has already been bound", providerParams(provider.GetName()))
		return false, false
	}
	// Telegram is the only provider still registered, and it is disabled, so a
	// bind flow can no longer be started. Its historical binding column is the
	// one exception kept wired here.
	if providerName != "telegram" {
		writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
		return false, false
	}
	_, err = model.ConsumeAuthFlowWithAction(state, match, func(tx *gorm.DB, _ *model.AuthFlow) error {
		return model.BindTelegramForSessionWithTx(tx, identity, oauthUser.ProviderUserID)
	})
	if err != nil {
		writeSecurityOperationError(c, err)
		return false, false
	}
	user, err := model.GetUserById(identity.UserID, false)
	if err != nil {
		writeSecurityOperationError(c, err)
		return true, true
	}
	notificationFailed := service.NotifyAccountSecurityChange(service.AccountMailLanguage(c, user.Id), user.Email, user.Id, i18n.MsgSecurityEventLoginLinked, map[string]any{"Provider": provider.GetName()}) != nil
	common.ApiSuccessT(c, "Binding successful", gin.H{"action": "bind", "notification_warning": notificationFailed})
	return true, notificationFailed
}

// findOrCreateOAuthUser finds the existing user on this site, or creates one
// when registration is enabled.
func findOrCreateOAuthUser(c *gin.Context, provider oauth.Provider, oauthUser *oauth.OAuthUser) (*model.User, error) {
	user := &model.User{}
	siteID := service.AdtyRequestSiteID(c)
	if provider.ProviderUserIDColumn() == "telegram_id" {
		err := provider.FillUserByProviderID(user, oauthUser.ProviderUserID, siteID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, oauth.ErrTelegramAccountNotBound
		}
		return user, err
	}

	// Check if user already exists with new ID on this site.
	if provider.IsUserIDTaken(oauthUser.ProviderUserID, siteID) {
		err := provider.FillUserByProviderID(user, oauthUser.ProviderUserID, siteID)
		if err != nil {
			return nil, err
		}
		// Check if user has been deleted
		if user.Id == 0 {
			return nil, &OAuthUserDeletedError{}
		}
		return user, nil
	}

	// User doesn't exist, create new user if registration is enabled
	if !common.RegisterEnabled {
		return nil, &OAuthRegistrationDisabledError{}
	}

	// Set up new user
	user.Username = provider.GetProviderPrefix() + strconv.Itoa(model.GetMaxUserId()+1)

	if oauthUser.Username != "" {
		if exists, err := model.CheckUserExistOrDeleted(oauthUser.Username, "", 0); err == nil && !exists {
			// 防止索引退化
			// 二开：含 @ 的不用，保留自动生成的名字（任务 8.7）
			if len(oauthUser.Username) <= model.UserNameMaxLength && !strings.Contains(oauthUser.Username, "@") {
				user.Username = oauthUser.Username
			}
		}
	}

	if oauthUser.DisplayName != "" {
		user.DisplayName = oauthUser.DisplayName
	} else if oauthUser.Username != "" {
		user.DisplayName = oauthUser.Username
	} else {
		user.DisplayName = provider.GetName() + " User"
	}
	// 二开：和密码注册一样，分站上的新用户归当前站；closing 的站不再接受注册（任务 11.2）
	siteId, registrable := service.AdtyRegisterSiteId(c)
	if !registrable {
		return nil, &OAuthRegistrationDisabledError{}
	}
	user.SiteId = siteId
	if oauthUser.Email != "" {
		user.Email = model.NormalizeEmail(oauthUser.Email)
		if err := model.EnsureEmailAvailable(user.Email, siteId, 0); err != nil {
			if errors.Is(err, model.ErrEmailAlreadyTaken) {
				return nil, &OAuthEmailAlreadyTakenError{}
			}
			return nil, err
		}
	}
	user.Role = common.RoleCommonUser
	user.Status = common.UserStatusEnabled
	if !service.AdtyRegisterTermsAccepted(c, siteId) {
		return nil, &OAuthSiteTermsError{}
	}

	// Create the user. The only provider still registered (Telegram) never
	// reaches this path: it looks up an existing binding instead of creating one.
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		return user.InsertWithTx(tx, 0)
	})
	if err != nil {
		return nil, err
	}

	// Perform post-transaction tasks
	user.FinalizeOAuthUserCreation(0)
	service.AdtyAfterRegister(c, user)

	return user, nil
}

// Error types for OAuth
type OAuthUserDeletedError struct{}

func (e *OAuthUserDeletedError) Error() string {
	return "user has been deleted"
}

type OAuthSiteTermsError struct{}

func (e *OAuthSiteTermsError) Error() string {
	return "Accept the current site customer terms first"
}

type OAuthRegistrationDisabledError struct{}

func (e *OAuthRegistrationDisabledError) Error() string {
	return "registration is disabled"
}

type OAuthEmailAlreadyTakenError struct{}

func (e *OAuthEmailAlreadyTakenError) Error() string {
	return "email is already in use"
}
