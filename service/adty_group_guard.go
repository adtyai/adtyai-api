package service

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// adtyGroupSpecialUsableKey 是特殊可用分组在 options 表里的键（带点的 config 键）。
const adtyGroupSpecialUsableKey = "group_ratio_setting.group_special_usable_group"

// IsAdtyGroupOption 说明这个配置项的键或值里写的是分组 ID，保存前要校验。
func IsAdtyGroupOption(key string) bool {
	switch key {
	case "GroupRatio", "TopupGroupRatio", "UserUsableGroups", "GroupGroupRatio",
		"AutoGroups", "ModelRequestRateLimitGroup", groupModelAllowOption, adtyGroupSpecialUsableKey:
		return true
	}
	return false
}

// ValidateAdtyGroupOption 检查分组类配置里出现的每个分组 ID 都在分组表里（未删除）。
// auto 只能出现在 UserUsableGroups 的键和特殊可用分组的内层键里。
// GroupRatio 还不能漏掉任何未删除的分组：删除分组只能走迁移和删除。
// 值的格式错误留给上游各自的检查，这里解析不了就放行。
func ValidateAdtyGroupOption(key, value string) error {
	if !IsAdtyGroupOption(key) {
		return nil
	}
	ids, err := groupIDsInOption(key, value)
	if err != nil {
		return nil
	}
	active, err := model.ActiveAdtyGroupIDs()
	if err != nil {
		return err
	}
	for _, ref := range ids {
		if ref.id == "auto" {
			if ref.autoAllowed {
				continue
			}
			return common.NewMessage("Group auto cannot be used in {{key}}", map[string]any{"key": key})
		}
		if _, ok := active[ref.id]; !ok {
			return common.NewMessage("Unknown group ID {{id}} in {{key}}", map[string]any{"id": ref.id, "key": key})
		}
	}
	if key != "GroupRatio" {
		return nil
	}
	present := make(map[string]struct{}, len(ids))
	for _, ref := range ids {
		present[ref.id] = struct{}{}
	}
	missing := make([]string, 0)
	for id := range active {
		if _, ok := present[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	return common.NewMessage("Group ratios are missing groups {{ids}}. Migrate and delete a group instead of removing its ratio", map[string]any{"ids": strings.Join(missing, ", ")})
}

type adtyGroupRef struct {
	id          string
	autoAllowed bool
}

func groupIDsInOption(key, value string) ([]adtyGroupRef, error) {
	var refs []adtyGroupRef
	add := func(id string, autoAllowed bool) {
		refs = append(refs, adtyGroupRef{id: strings.TrimSpace(id), autoAllowed: autoAllowed})
	}
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	switch key {
	case "GroupRatio", "TopupGroupRatio", "UserUsableGroups", "ModelRequestRateLimitGroup", groupModelAllowOption:
		object := map[string]any{}
		if err := common.UnmarshalJsonStr(value, &object); err != nil {
			return nil, err
		}
		for id := range object {
			add(id, key == "UserUsableGroups")
		}
	case "GroupGroupRatio":
		nested := map[string]map[string]any{}
		if err := common.UnmarshalJsonStr(value, &nested); err != nil {
			return nil, err
		}
		for outer, inner := range nested {
			add(outer, false)
			for id := range inner {
				add(id, false)
			}
		}
	case "AutoGroups":
		var list []string
		if err := common.UnmarshalJsonStr(value, &list); err != nil {
			return nil, err
		}
		for _, id := range list {
			add(id, false)
		}
	case adtyGroupSpecialUsableKey:
		nested := map[string]map[string]any{}
		if err := common.UnmarshalJsonStr(value, &nested); err != nil {
			return nil, err
		}
		for owner, inner := range nested {
			add(owner, false)
			for id := range inner {
				if after, ok := strings.CutPrefix(id, "+:"); ok {
					id = after
				} else if after, ok := strings.CutPrefix(id, "-:"); ok {
					id = after
				}
				add(id, true)
			}
		}
	}
	return refs, nil
}
