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
