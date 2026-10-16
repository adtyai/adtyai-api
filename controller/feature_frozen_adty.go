package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// adminFeatureFrozen gates balance-adjustments admin HTTP for this release.
// Enterprise-pricing (A8) was unfrozen; keep this true for balance-adjust only.
// Flip to false to restore balance-adjust handlers without rewriting them.
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
