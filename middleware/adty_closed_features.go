package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdtyClosedFeature rejects excluded products that can still move money:
// check-in rewards, upstream single-channel epay, Stripe/Creem/Waffo
// checkout and webhooks, and the leftover Waffo Pancake catalog tools.
// Those pay handlers credit a wallet without the official ADTY order,
// bonus, commission, and ledger path. Official POST /api/user/pay and
// GET/POST /api/user/epay/notify/:channelId stay open.
func AdtyClosedFeature(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": "This feature is not available",
	})
}
