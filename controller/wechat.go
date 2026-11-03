package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type wechatLoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func isWeChatSubjectConflict(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "wechat_id") || strings.Contains(text, "ux_users_wechat_id_subject")
}

func getWeChatIdByCode(code string) (string, error) {
	if code == "" {
		return "", common.NewMessage("Invalid parameters")
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/wechat/user?code=%s", common.WeChatServerAddress, url.QueryEscape(code)), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", common.WeChatServerToken)
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	httpResponse, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer httpResponse.Body.Close()
	var res wechatLoginResponse
	err = common.DecodeJson(httpResponse.Body, &res)
	if err != nil {
		return "", err
	}
	if !res.Success {
		return "", errors.New(res.Message)
	}
	if res.Data == "" {
		return "", common.NewMessage("Verification code is incorrect or has expired")
	}
	return res.Data, nil
}

func WeChatAuth(c *gin.Context) {
	if !common.WeChatAuthEnabled {
		common.ApiErrorT(c, "{{provider}} login and registration has not been enabled by administrator", providerParams("WeChat"))
		return
	}
	code := c.Query("code")
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user := model.User{
		WeChatId: wechatId,
	}
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		err := user.FillUserByWeChatId()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if user.Id == 0 {
			common.ApiErrorT(c, "User has been deleted")
			return
		}
	} else {
		if !common.RegisterEnabled {
			common.ApiErrorT(c, "New user registration has been disabled by administrator")
			return
		}
		// 二开：和密码注册一样，分站上的新用户归当前站；closing 的站不再接受注册，并走注册钩子（任务 11.2、6.2）。
		siteId, registrable := service.AdtyRegisterSiteId(c)
		if !registrable {
			common.ApiErrorT(c, "New user registration has been disabled by administrator")
			return
		}
		if !service.AdtyRegisterTermsAccepted(c, siteId) {
			common.ApiErrorT(c, "Accept the current site customer terms first")
			return
		}
		user.DisplayName = "WeChat User"
		user.Role = common.RoleCommonUser
		user.Status = common.UserStatusEnabled
		user.SiteId = siteId

		created := false
		for attempt := 0; attempt < 5; attempt++ {
			user.Id = 0
			user.Username = "wechat_" + strconv.Itoa(model.GetMaxUserId()+1+attempt)
			err := user.Insert(0)
			if err == nil {
				created = true
				service.AdtyAfterRegister(c, &user)
				break
			}
			if !model.IsAdtyUniqueViolation(err) {
				common.ApiError(c, err)
				return
			}
			if isWeChatSubjectConflict(err) {
				// 并发注册撞上刚落库的同一微信时，改走已有账号，不再建第二个用户。
				user = model.User{WeChatId: wechatId}
				if err := user.FillUserByWeChatId(); err != nil || user.Id == 0 {
					common.ApiErrorT(c, "This {{provider}} account has already been bound", providerParams("WeChat"))
					return
				}
				break
			}
		}
		if !created && user.Id == 0 {
			common.ApiErrorT(c, "User registration failed or user ID retrieval failed")
			return
		}
	}

	if user.Status != common.UserStatusEnabled {
		common.ApiErrorT(c, "User has been banned")
		return
	}
	setupLogin(&user, nil, c)
}

type wechatBindRequest struct {
	Code string `json:"code"`
}

func WeChatBind(c *gin.Context) {
	identity, ok := middleware.GetStepUpIdentity(c)
	if !ok {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return
	}
	succeeded, notificationFailed := false, false
	defer func() {
		recordUserSecurityAudit(c, identity.UserID, "user.binding_bind", map[string]any{"provider": "wechat", "success": succeeded, "notification_failed": notificationFailed})
	}()
	if !common.WeChatAuthEnabled {
		common.ApiErrorT(c, "{{provider}} login and registration has not been enabled by administrator", providerParams("WeChat"))
		return
	}
	var req wechatBindRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid request")
		return
	}
	code := strings.TrimSpace(req.Code)
	context, err := common.Marshal(service.AccountBindingContext{Provider: "wechat", Code: code})
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: context}) == nil {
		return
	}
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		common.ApiErrorT(c, "This {{provider}} account has already been bound", providerParams("WeChat"))
		return
	}
	// 只更新绑定列，避免完整用户快照覆盖并发的封禁、降权或分组变更。
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		return model.UpdateUserBindColumnForSessionWithTx(tx, identity, "wechat_id", wechatId)
	}); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	succeeded = true
	user, err := model.GetUserById(identity.UserID, false)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	notificationFailed = service.NotifyAccountSecurityChange(service.AccountMailLanguage(c, user.Id), user.Email, i18n.MsgSecurityEventWeChatLinked) != nil
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"notification_warning": notificationFailed},
	})
	return
}
