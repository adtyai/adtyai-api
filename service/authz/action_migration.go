package authz

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const AdtyActionCatalogVersion = 2

const adtyActionCatalogStateKey = "adty_action_catalog"

// MigrateAdtyActionCatalog runs before loading the new catalog or serving any
// requests. It never derives grants again after the durable version is stored.
func MigrateAdtyActionCatalog(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		state, err := model.LockAuthzActionCatalogState(tx, adtyActionCatalogStateKey)
		if err != nil {
			return err
		}
		if state.Version == AdtyActionCatalogVersion {
			return nil
		}
		if state.Version > AdtyActionCatalogVersion {
			return fmt.Errorf("authorization action catalog version %d is newer than supported version %d", state.Version, AdtyActionCatalogVersion)
		}
		expansions := map[Permission][]Permission{
			AdtyUserWrite:         {AdtyUserCreate},
			AdtyModelWrite:        {AdtyModelCreate, AdtyModelDelete},
			AdtyLogRead:           {AdtyLogExport},
			ChannelSensitiveWrite: {ChannelCreate, ChannelDelete},
			AdtySiteAdjust:        {AdtySiteCreate, AdtySiteDelete},
			AdtySiteSettings:      {AdtySiteSettingsRead, AdtySiteSettingsCreate, AdtySiteSettingsDelete},
			AdtyReferralSettings:  {AdtyReferralSettingsRead},
		}
		type subjectPermission struct {
			subject string
			Permission
		}
		var existing []model.CasbinRule
		if err := tx.Where("ptype = ?", "p").Order("id").Find(&existing).Error; err != nil {
			return err
		}
		present := make(map[subjectPermission]bool, len(existing))
		for _, rule := range existing {
			if rule.V0 == "" || rule.V1 == "" || rule.V2 == "" || rule.V3 != "" && rule.V3 != EffectAllow && rule.V3 != EffectDeny {
				return fmt.Errorf("authorization policy %d cannot be migrated: invalid subject, resource, action or effect", rule.Id)
			}
			present[subjectPermission{rule.V0, Permission{Resource: rule.V1, Action: rule.V2}}] = true
		}
		var expanded []model.CasbinRule
		for _, rule := range existing {
			for _, target := range expansions[Permission{Resource: rule.V1, Action: rule.V2}] {
				// Every preexisting destination rule wins, including a scoped deny.
				// Do not mark newly copied rows as preexisting: all source effects and
				// scopes must survive so a deny still dominates a source allow.
				if present[subjectPermission{rule.V0, target}] {
					continue
				}
				copy := rule
				copy.Id, copy.V1, copy.V2 = 0, target.Resource, target.Action
				expanded = append(expanded, copy)
			}
		}
		if len(expanded) > 0 {
			if err := tx.CreateInBatches(&expanded, 100).Error; err != nil {
				return err
			}
		}
		return tx.Model(state).Update("version", AdtyActionCatalogVersion).Error
	})
}

// RequireAdtyActionCatalogVersion fails closed on replicas and older binaries;
// it does not create a state row or modify any policy.
func RequireAdtyActionCatalogVersion(db *gorm.DB) error {
	var state model.AuthzActionCatalogState
	if err := db.Where(&model.AuthzActionCatalogState{Key: adtyActionCatalogStateKey}).First(&state).Error; err != nil {
		return fmt.Errorf("authorization action catalog migration is not ready: %w", err)
	}
	if state.Version != AdtyActionCatalogVersion {
		return fmt.Errorf("authorization action catalog version %d is not ready; expected %d", state.Version, AdtyActionCatalogVersion)
	}
	return nil
}
