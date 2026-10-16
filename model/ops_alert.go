package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

const (
	OpsAlertLevelPending = "pending"
	OpsAlertLevelUrgent  = "urgent"
)

// OpsAlertEvent 运维告警 stub：真通道未接前落库供验收断言。
type OpsAlertEvent struct {
	Id        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Level     string    `json:"level" gorm:"type:varchar(32);index;not null"`
	Kind      string    `json:"kind" gorm:"type:varchar(64);index;not null"`
	Payload   string    `json:"payload" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

func (OpsAlertEvent) TableName() string { return "ops_alert_events" }

// RecordOpsAlert 写入一条告警事件（stub，不推 Telegram）。
func RecordOpsAlert(level, kind string, payload map[string]any) error {
	b, _ := json.Marshal(payload)
	row := OpsAlertEvent{Level: level, Kind: kind, Payload: string(b)}
	return DB.Create(&row).Error
}

// ListOpsAlertsByKind 验收用查询。
func ListOpsAlertsByKind(kind string, limit int) ([]OpsAlertEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []OpsAlertEvent
	err := DB.Where("kind = ?", kind).Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// EnsureOpsAlertTable 供验收脚本显式迁移。
func EnsureOpsAlertTable(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	return db.AutoMigrate(&OpsAlertEvent{})
}
