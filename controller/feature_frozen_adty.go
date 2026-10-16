package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// adminFeatureFrozen gates enterprise-pricing / balance-adjustments admin HTTP
// for this release. Flip to false to restore the handlers without rewriting them.
const adminFeatureFrozen = true

// respondAdminFeatureFrozen writes the release-freeze 403 payload.
// Handlers keep their bodies (accept scripts call controllers directly);
// router groups also mount middleware.FeatureFrozen for defense in depth.
func respondAdminFeatureFrozen(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"code":    "feature_frozen",
		"message": "本版已冻结",
	})
}
