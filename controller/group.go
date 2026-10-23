package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]any)
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserAllowedGroups(userId, userGroup)
	displayNames := model.BuildGroupDisplayNameMap()
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		if groupName == "auto" {
			continue
		}
		if _, ok := userUsableGroups[groupName]; !ok {
			continue
		}
		displayName := displayNames[groupName]
		if displayName == "" {
			displayName = groupName
		}
		usableGroups[groupName] = map[string]any{
			"ratio":        service.GetUserGroupRatio(userGroup, groupName),
			"desc":         displayName,
			"display_name": displayName,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
