package oauth

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// 约定 I（docs/adty/conventions.md）：Telegram 只有用户通知和管理通知两个
// 机器人，Token 仅 Root 配置、加密保存、脱敏展示；绑定不复用 Telegram OAuth，
// 也不能登录、找回密码或充当二次验证。原先把这条约定实现成了一条完整的
// OAuth 登录链，只要管理员填上 client_id/client_secret 再打开开关就能用。
//
// 这条链现在已退役：Provider 仍然注册（占住 "telegram" 这个名字，防止被
// 自定义 OAuth Provider 顶替），但 IsEnabled 恒为 false，换取 token 和取用户
// 信息都直接返回 ErrTelegramOAuthNotConfigured。下表的 Telegram 绑定与通知
// 机器人（model/adty_telegram.go）与本文件无关，继续正常工作。
var (
	ErrTelegramOAuthNotConfigured = errors.New("Telegram sign-in has been retired. Use another sign-in method.")
	ErrTelegramOAuthConflict      = errors.New("The telegram OAuth provider name is reserved. Ask your administrator to rename the conflicting custom provider.")
	ErrTelegramOAuthFailed        = errors.New("Telegram authorization failed. Please try again.")
	ErrTelegramAccountNotBound    = errors.New("This Telegram account is not linked. Sign in using another method and link it first.")
)

// TelegramConfigurationError reports why Telegram is not offered as a sign-in
// method. It never returns nil.
func TelegramConfigurationError() error {
	return ErrTelegramOAuthNotConfigured
}

type TelegramProvider struct{}

func init() {
	Register("telegram", &TelegramProvider{})
}

func (p *TelegramProvider) GetName() string { return "Telegram" }

func (p *TelegramProvider) IsEnabled() bool { return false }

func (p *TelegramProvider) ExchangeToken(context.Context, string, *gin.Context) (*OAuthToken, error) {
	return nil, ErrTelegramOAuthNotConfigured
}

func (p *TelegramProvider) GetUserInfo(context.Context, *OAuthToken) (*OAuthUser, error) {
	return nil, ErrTelegramOAuthNotConfigured
}

// The remaining methods only read and write the telegram_id column that
// historical bindings already use; no sign-in chain reaches them any more.

func (p *TelegramProvider) IsUserIDTaken(id string, siteID int) bool {
	return model.IsTelegramIdAlreadyTaken(id, siteID)
}

func (p *TelegramProvider) FillUserByProviderID(user *model.User, id string, siteID int) error {
	stored, err := model.GetUserByTelegramID(id, siteID)
	if err != nil {
		return err
	}
	*user = *stored
	return nil
}

func (p *TelegramProvider) SetProviderUserID(user *model.User, id string) { user.TelegramId = id }

func (p *TelegramProvider) GetProviderPrefix() string { return "telegram_" }

func (p *TelegramProvider) ProviderUserIDColumn() string { return "telegram_id" }
