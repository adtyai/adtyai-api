package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// TelegramLegacyAuth retires the unsigned-flow widget endpoints. Telegram is a
// notification channel only (docs/adty/conventions.md 约定 I); neither these
// endpoints nor the unified OAuth provider offer sign-in any more.
func TelegramLegacyAuth(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"success": false,
		"code":    "TELEGRAM_LEGACY_AUTH_REMOVED",
		"message": "Telegram login has changed. Reload the page and start Telegram OAuth again.",
	})
}
