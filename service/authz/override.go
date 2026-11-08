package authz

import (
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type overridePolicy struct {
	Resource string
	Action   string
	Effect   string
}

func SetUserPermissions(userID int, permissions PermissionsMap) error {
	e := currentEnforcer()
	if e == nil {
		return fmt.Errorf("authz enforcer is not initialized")
	}

	adapter, ok := e.GetAdapter().(*gormAdapter)
	if !ok {
		return fmt.Errorf("authz adapter does not support transactional permission patches")
	}
	if err := adapter.db.Transaction(func(tx *gorm.DB) error {
		return SetUserPermissionsInTx(tx, userID, permissions)
	}); err != nil {
		return err
	}
	return ReloadPolicy()
}

func SetUserPermissionsInTx(tx *gorm.DB, userID int, permissions PermissionsMap) error {
	if currentEnforcer() == nil {
		return fmt.Errorf("authz enforcer is not initialized")
	}
	policies, err := permissionPatchPolicies(permissions)
	if err != nil {
		return err
	}
	for _, policy := range policies {
		// Replace only this explicitly supplied global cell. Legacy own/unknown
		// restrictions remain stored and continue to load as deny.
		if err := tx.Where("ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?", "p", UserSubject(userID), policy.Resource, policy.Action).
			Where("(v4 = ? OR v4 = ? OR v4 IS NULL) AND (v5 = ? OR v5 IS NULL)", "", "all", "").
			Delete(&model.CasbinRule{}).Error; err != nil {
			return err
		}
		rule := newRule("p", []string{UserSubject(userID), policy.Resource, policy.Action, policy.Effect})
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
	}
	return nil
}

func ClearUserPermissions(userID int) error {
	e := currentEnforcer()
	if e == nil {
		return fmt.Errorf("authz enforcer is not initialized")
	}

	for _, resource := range registry {
		if _, err := e.RemoveFilteredPolicy(0, UserSubject(userID), resource.Resource); err != nil {
			return err
		}
	}
	return nil
}

func ClearUserPermissionsInTx(tx *gorm.DB, userID int) error {
	for _, resource := range registry {
		if err := tx.Where("ptype = ? AND v0 = ? AND v1 = ?", "p", UserSubject(userID), resource.Resource).Delete(&model.CasbinRule{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func ClearUserAuthorization(userID int) error {
	return ClearUserPermissions(userID)
}

func ClearUserAuthorizationInTx(tx *gorm.DB, userID int) error {
	return ClearUserPermissionsInTx(tx, userID)
}

// ExplicitUserPermissions returns the effective permission matrix for the
// managed role plus any per-user overrides.
func ExplicitUserPermissions(userID int) PermissionsMap {
	return Capabilities(userID, common.RoleAdminUser)
}

// ExplicitUserOverrides returns only the per-user override entries.
func ExplicitUserOverrides(userID int) PermissionsMap {
	e := currentEnforcer()
	if e == nil {
		return PermissionsMap{}
	}

	result := PermissionsMap{}
	for _, resource := range registry {
		policies, err := e.GetFilteredPolicy(0, UserSubject(userID), resource.Resource)
		if err != nil {
			return PermissionsMap{}
		}
		actions := make(map[string]bool, len(policies))
		for _, policy := range policies {
			if len(policy) >= 3 && isKnownPermission(Permission{Resource: policy[1], Action: policy[2]}) {
				effect := policyEffect(policy)
				if effect == EffectAllow || effect == EffectDeny {
					if previous, exists := actions[policy[2]]; !exists || previous {
						actions[policy[2]] = effect == EffectAllow
					}
				}
			}
		}
		if len(actions) > 0 {
			result[resource.Resource] = actions
		}
	}
	return result
}

// permissionPatchPolicies validates the entire patch before any write. Explicit
// false values are durable denies, even when the current role baseline is false.
func permissionPatchPolicies(permissions PermissionsMap) ([]overridePolicy, error) {
	var overrides []overridePolicy
	for resource, actions := range permissions {
		if !isKnownResource(resource) {
			return nil, fmt.Errorf("unknown authorization resource %q", resource)
		}
		for action, desired := range actions {
			if !isKnownPermission(Permission{Resource: resource, Action: action}) {
				return nil, fmt.Errorf("unknown authorization action %q for resource %q", action, resource)
			}
			effect := EffectDeny
			if desired {
				effect = EffectAllow
			}
			overrides = append(overrides, overridePolicy{Resource: resource, Action: action, Effect: effect})
		}
	}
	sort.Slice(overrides, func(i, j int) bool {
		if overrides[i].Resource != overrides[j].Resource {
			return overrides[i].Resource < overrides[j].Resource
		}
		return overrides[i].Action < overrides[j].Action
	})
	return overrides, nil
}
