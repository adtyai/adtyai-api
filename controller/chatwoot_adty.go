package controller

import (
	"net/http"
	"strconv"

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
// Root only. 不再要求 confirm（前端普通确认框即可）。
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
	_, _, err := model.ApplyChatwootPut(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error(), "code": "bad_request"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.BuildChatwootConfigView()})
}

// GetChatwootPublicConfig GET /api/chatwoot/config（公开）审查 N-P2-10
func GetChatwootPublicConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.GetChatwootPublicConfig()})
}

// GetChatwootIdentity GET /api/chatwoot/identity（登录用户）审查 N-P2-10
// 返回 Chatwoot setUser 所需 identifier + identifier_hash；未配置 HMAC 密钥时 identifier_hash 为空。
func GetChatwootIdentity(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		adtyFail(c, http.StatusNotFound, "not_found", "user not found", nil)
		return
	}
	identifier := strconv.Itoa(user.Id)
	hash, _ := model.ChatwootIdentityHash(identifier)
	name := user.DisplayName
	if name == "" {
		name = user.Username
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"identifier": identifier, "identifier_hash": hash, "name": name, "email": user.Email,
	}})
}
