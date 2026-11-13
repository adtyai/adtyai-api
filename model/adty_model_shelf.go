package model

import "strings"

// CatalogModelCallable reports whether a live catalog row allows new calls.
// A name with no catalog row stays callable through its channel. A shelved row does not.
func CatalogModelCallable(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || DB == nil {
		return true
	}
	var rows []Model
	if err := DB.Select("status").Where("model_name = ?", name).Limit(1).Find(&rows).Error; err != nil {
		return false
	}
	if len(rows) == 0 {
		return true
	}
	return rows[0].Status == 1
}
