package model

import "github.com/QuantumNous/new-api/common"

// GetAdtyKeyLimit returns the stored per-minute cap. A missing row means no extra cap.
func GetAdtyKeyLimit(tokenID int) (AdtyKeyLimit, error) {
	var row AdtyKeyLimit
	if tokenID <= 0 {
		return row, common.NewMessage("Invalid parameters")
	}
	err := DB.Where("token_id = ?", tokenID).Limit(1).Find(&row).Error
	return row, err
}

// SaveAdtyKeyLimit stores a positive per-minute cap for one key.
func SaveAdtyKeyLimit(tokenID, perMinute int) error {
	if tokenID <= 0 || perMinute <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	row := AdtyKeyLimit{TokenId: tokenID, PerMinute: perMinute}
	return DB.Where("token_id = ?", tokenID).Assign(row).FirstOrCreate(&row).Error
}

// ClearAdtyKeyLimit removes a key cap. Later requests are not extra-limited.
func ClearAdtyKeyLimit(tokenID int) error {
	if tokenID <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	return DB.Where("token_id = ?", tokenID).Delete(&AdtyKeyLimit{}).Error
}
