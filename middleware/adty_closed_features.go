package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdtyClosedFeature rejects excluded products that can still move money:
// check-in rewards, upstream single-channel epay, and Stripe/Creem/Waffo.
// Those handlers credit a wallet without the adty order, bonus, commission,
// and ledger path. The fork's own /api/user/epay/notify/:channelId stays open.
func AdtyClosedFeature(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": "This feature is not available",
	})
}
