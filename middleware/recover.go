package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func RelayPanicRecover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				common.SysLog(common.LogText("panic detected: %v", err))
				common.SysLog(common.LogText("stacktrace from panic: %s", string(debug.Stack())))
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"message": "Internal server error, please contact support",
						"type":    "adtyai_panic",
					},
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
