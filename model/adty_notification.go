package model

import "gorm.io/gorm/clause"

// AdtyNotificationRead records the first read of a content version by one account
// on one site. Public browser history never becomes an account receipt.
type AdtyNotificationRead struct {
	UserId  int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	SiteId  int    `json:"site_id" gorm:"primaryKey;autoIncrement:false"`
	ReadKey string `json:"read_key" gorm:"type:char(64);primaryKey"`
	ReadAt  int64  `json:"read_at" gorm:"not null"`
}

func GetAdtyNotificationReadKeys(userID, siteID int, keys []string) ([]string, error) {
	read := []string{}
	if len(keys) == 0 {
		return read, nil
	}
	err := DB.Model(&AdtyNotificationRead{}).Where("user_id = ? AND site_id = ? AND read_key IN ?", userID, siteID, keys).
		Order("read_key").Pluck("read_key", &read).Error
	return read, err
}

func MarkAdtyNotificationsRead(userID, siteID int, keys []string, now int64) error {
	if len(keys) == 0 {
		return nil
	}
	rows := make([]AdtyNotificationRead, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, AdtyNotificationRead{UserId: userID, SiteId: siteID, ReadKey: key, ReadAt: now})
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}
