package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdminGetChatwoot GET /api/admin/chatwoot (AdminAuth)
func AdminGetChatwoot(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    model.BuildChatwootConfigView(),
	})
}

// AdminPutChatwoot PUT /api/admin/chatwoot
// Root only. Changing script or setting hmac_secret requires confirm=true.
func AdminPutChatwoot(c *gin.Context) {
	var req model.ChatwootPutInput
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid json", "code": "bad_request"})
		return
	}
	role := c.GetInt("role")
	if role < common.RoleRootUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "only root can update chatwoot settings", "code": "forbidden"})
		return
	}
	if model.ChatwootPutNeedsConfirm(req) && !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "confirm=true required to change chatwoot script or hmac secret", "code": "confirm_required"})
		return
	}
	_, _, err := model.ApplyChatwootPut(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error(), "code": "bad_request"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.BuildChatwootConfigView()})
}
