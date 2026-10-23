package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	return GetUserAllowedGroups(0, userGroup)
}

// GetUserAllowedGroups is the public group map, plus the caller's identity
// group, plus hidden groups granted to that user. A grant lookup failure does
// not expose hidden groups.
func GetUserAllowedGroups(userID int, userGroup string) map[string]string {
	groups := setting.GetUserUsableGroupsCopy()
	delete(groups, "auto")
	for code := range groups {
		if !ratio_setting.ContainsGroupRatio(code) {
			delete(groups, code)
		}
	}
	if userGroup != "" && userGroup != "auto" && ratio_setting.ContainsGroupRatio(userGroup) {
		if _, ok := groups[userGroup]; !ok {
			groups[userGroup] = "User group"
		}
	}
	if userID <= 0 {
		return groups
	}
	codes, err := model.ListUserGroupGrantCodes(userID)
	if err != nil {
		common.SysError(common.LogText("failed to load hidden group grants for user %d: %s", userID, err.Error()))
		return groups
	}
	for _, code := range codes {
		if code == "" || code == "auto" || !ratio_setting.ContainsGroupRatio(code) {
			continue
		}
		if _, ok := groups[code]; ok {
			continue
		}
		groups[code] = model.GetGroupDisplayName(code)
	}
	return groups
}

// UserMayUseGroup reports whether this user may select the group. Empty and
// auto are never selectable. A grant lookup failure keeps hidden groups closed.
func UserMayUseGroup(userID int, userGroup, groupName string) bool {
	if groupName == "" || groupName == "auto" {
		return false
	}
	_, ok := GetUserAllowedGroups(userID, userGroup)[groupName]
	return ok
}

// IdentityGroupChangeAllowed reports whether an administrator may write this
// identity group. The current value stays valid so an existing hidden identity
// can still be edited. A new value must be a public billing group.
func IdentityGroupChangeAllowed(current, next string) bool {
	return model.IdentityGroupChangeAllowed(current, next)
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

func IsUserSelectableGroup(userGroup, groupName string) bool {
	if groupName == "" || groupName == "auto" {
		return false
	}
	return GroupInUserUsableGroups(userGroup, groupName) && ratio_setting.ContainsGroupRatio(groupName)
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	autoGroups := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range setting.GetAutoGroups() {
		if !IsUserSelectableGroup(userGroup, group) {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		autoGroups = append(autoGroups, group)
	}
	return autoGroups
}

// FilterUserTokenAutoGroups applies current permissions before the current
// per-token limit. It intentionally does not fall back to the global Auto list.
func FilterUserTokenAutoGroups(userGroup string, groups []string) []string {
	maxCount := setting.GetMaxTokenAutoGroups()
	filtered := make([]string, 0, min(len(groups), maxCount))
	seen := make(map[string]struct{})
	for _, group := range groups {
		if !IsUserSelectableGroup(userGroup, group) {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		filtered = append(filtered, group)
		if len(filtered) == maxCount {
			break
		}
	}
	return filtered
}

// GetRequestAutoGroups resolves the ordered Auto groups for the current token.
// The absence of the context value means that the token inherits the complete
// global Auto list; a present (even empty) value is an explicit token snapshot.
func GetRequestAutoGroups(c *gin.Context, userGroup string) []string {
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenAutoGroups)
	if !ok {
		return GetUserAutoGroup(userGroup)
	}
	groups, ok := value.([]string)
	if !ok {
		return []string{}
	}
	return FilterUserTokenAutoGroups(userGroup, groups)
}

// GetGroupsEnabledModels 按 groups 顺序获取各分组启用的模型并去重
func GetGroupsEnabledModels(groups []string) []string {
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, group := range groups {
		for _, modelName := range model.GetGroupEnabledModels(group) {
			if _, ok := seen[modelName]; !ok {
				seen[modelName] = struct{}{}
				models = append(models, modelName)
			}
		}
	}
	return models
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	return ratio_setting.GetGroupRatio(group)
}
