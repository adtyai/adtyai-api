package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AuthzActionCatalogState is separate from Casbin policies so policy saves cannot
// erase a completed migration and restore a subsequently revoked action.
type AuthzActionCatalogState struct {
	Key     string `gorm:"size:64;primaryKey"`
	Version int    `gorm:"not null"`
}

func (AuthzActionCatalogState) TableName() string { return "authz_action_catalog_states" }

// LockAuthzActionCatalogState must be called within the migration transaction.
// Inserting the anchor also serializes the first migration of an empty table.
func LockAuthzActionCatalogState(tx *gorm.DB, key string) (*AuthzActionCatalogState, error) {
	state := AuthzActionCatalogState{Key: key}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return nil, err
	}
	if err := lockForUpdate(tx).Where(&AuthzActionCatalogState{Key: key}).First(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}
