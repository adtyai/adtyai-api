package controller

import (
	"github.com/gin-gonic/gin"
)

// adtyFail writes a console-facing error JSON:
//
//	{success:false, message, code, params?}
//
// params is omitted when empty. Prefer stable codes from the frontend contract
// (invalid_id / invalid_parameters / not_found / forbidden / unauthorized /
// bad_request, plus business codes). Keep message as English (or existing CN)
// fallback; dynamic numbers belong in params (snake_case), not the message.
func adtyFail(c *gin.Context, status int, code, message string, params map[string]any) {
	body := gin.H{
		"success": false,
		"message": message,
		"code":    code,
	}
	if len(params) > 0 {
		body["params"] = params
	}
	c.JSON(status, body)
}
