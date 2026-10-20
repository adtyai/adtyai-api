package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// FeatureFrozen aborts every request with HTTP 403.
// Used to freeze admin feature groups for this release without deleting routes/handlers.
func FeatureFrozen() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"code":    "feature_frozen",
			"message": "本版已冻结",
		})
		c.Abort()
	}
}

// LegacyTopupClosed refuses new orders on the pre-ledger Stripe/Creem/Waffo/epay pay routes.
// In-flight notify handlers stay registered so an already-paid callback can still credit.
func LegacyTopupClosed() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"code":    "legacy_topup_closed",
			"message": "请使用充值订单接口",
		})
		c.Abort()
	}
}

// SubscriptionClosed aborts subscription purchase/management requests.
// Tables and historical subscriptions stay; only HTTP entrances refuse.
func SubscriptionClosed() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"code":    "subscription_disabled",
			"message": "订阅功能已关闭",
		})
		c.Abort()
	}
}
