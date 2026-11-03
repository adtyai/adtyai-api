package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AdtyUserLimit 是单个用户的请求限制（任务 7.4）。-1 表示跟默认，0 表示不限。
// 列上不设 GORM default：有 default 时 GORM 插入会把 0（不限）当成没填，写成默认值。
type AdtyUserLimit struct {
	UserId      int   `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	RPM         int   `json:"rpm" gorm:"not null"`
	TPM         int   `json:"tpm" gorm:"not null"`
	Concurrency int   `json:"concurrency" gorm:"not null"`
	UpdatedAt   int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyUserLimit) TableName() string { return "adty_user_limits" }

// AdtyOnConsumeTokens 由 adtylimit 注册，RecordConsumeLog 一开头调用它累计 TPM。
// 放在 model 包里，避免 model 反过来依赖限流包。
var AdtyOnConsumeTokens func(userId int, tokens int, withoutToken bool)

// GetAdtyUserLimit 取一个用户的限制；没有设过时返回全部为 -1 的一行。
func GetAdtyUserLimit(userId int) (AdtyUserLimit, error) {
	var rows []AdtyUserLimit
	if err := DB.Where("user_id = ?", userId).Limit(1).Find(&rows).Error; err != nil {
		return AdtyUserLimit{}, err
	}
	if len(rows) == 0 {
		return AdtyUserLimit{UserId: userId, RPM: -1, TPM: -1, Concurrency: -1}, nil
	}
	return rows[0], nil
}

// SaveAdtyUserLimit 写一个用户的限制（有就覆盖）。
func SaveAdtyUserLimit(limit AdtyUserLimit) error {
	return DB.Session(&gorm.Session{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"rpm", "tpm", "concurrency", "updated_at"}),
	}).Create(&limit).Error
}
