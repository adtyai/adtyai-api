package model

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 官方价（任务 8.4）：各模型在厂商官网的原价，单位每百万 tokens 美元，价格页拿来算节省。
// 价格可能没有，用 *decimal.Decimal 存 NULL（约定 C 的例外）。

const (
	AdtyOfficialPriceSourceModelsDev = "models.dev"
	AdtyOfficialPriceSourceManual    = "manual"
)

type AdtyOfficialPrice struct {
	ModelName         string           `json:"model_name" gorm:"primaryKey;type:varchar(191)"`
	SourceId          string           `json:"source_id" gorm:"type:varchar(255);not null;default:''"` // models.dev 的 "provider/model"
	InputPerMTok      *decimal.Decimal `json:"input_per_mtok" gorm:"column:input_per_mtok;type:decimal(20,8)"`
	OutputPerMTok     *decimal.Decimal `json:"output_per_mtok" gorm:"column:output_per_mtok;type:decimal(20,8)"`
	CacheReadPerMTok  *decimal.Decimal `json:"cache_read_per_mtok" gorm:"column:cache_read_per_mtok;type:decimal(20,8)"`
	CacheWritePerMTok *decimal.Decimal `json:"cache_write_per_mtok" gorm:"column:cache_write_per_mtok;type:decimal(20,8)"`
	PerCall           *decimal.Decimal `json:"per_call" gorm:"type:decimal(20,8)"` // 按张、按秒计费的模型手填
	Source            string           `json:"source" gorm:"type:varchar(32);not null;default:''"`
	Override          bool             `json:"override" gorm:"not null"` // 手动覆盖后同步不再改价格
	SyncedAt          int64            `json:"synced_at" gorm:"not null;default:0"`
	UpdatedAt         int64            `json:"updated_at" gorm:"autoUpdateTime"`
}

// HasAnyPrice 表示至少有一项官方价，公开接口只返回这样的行。
func (p *AdtyOfficialPrice) HasAnyPrice() bool {
	return p.InputPerMTok != nil || p.OutputPerMTok != nil || p.CacheReadPerMTok != nil ||
		p.CacheWritePerMTok != nil || p.PerCall != nil
}

func ListAdtyOfficialPrices() ([]AdtyOfficialPrice, error) {
	var rows []AdtyOfficialPrice
	err := DB.Order("model_name").Find(&rows).Error
	return rows, err
}

// SaveAdtyOfficialPrice 是后台手动保存：映射、五项价格和覆盖开关一起写，没有的行就建。
func SaveAdtyOfficialPrice(row *AdtyOfficialPrice) error {
	row.UpdatedAt = time.Now().Unix()
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "model_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"source_id", "input_per_mtok", "output_per_mtok", "cache_read_per_mtok", "cache_write_per_mtok", "per_call", "source", "override", "updated_at"}),
	}).Create(row).Error
}

// AdtyOfficialPriceSync 是一次同步要写的一行。Create 为真表示自动映射出的新行。
type AdtyOfficialPriceSync struct {
	Row    AdtyOfficialPrice
	Create bool
}

// ApplyAdtyOfficialPriceSync 在一个事务里写同步结果，返回实际改动的行数。
// 更新带上 override = false 和原来的 source_id 作条件，同步期间后台改了这一行就跳过；
// 新行遇到同名行（后台刚建）也跳过。按次价是手填的，同步不写。
func ApplyAdtyOfficialPriceSync(items []AdtyOfficialPriceSync) (int, error) {
	updated := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			row := item.Row
			var result *gorm.DB
			if item.Create {
				result = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			} else {
				result = tx.Model(&AdtyOfficialPrice{}).
					Where("model_name = ? AND source_id = ? AND override = ?", row.ModelName, row.SourceId, false).
					Select("InputPerMTok", "OutputPerMTok", "CacheReadPerMTok", "CacheWritePerMTok", "Source", "SyncedAt", "UpdatedAt").
					Updates(&row)
			}
			if result.Error != nil {
				return result.Error
			}
			updated += int(result.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return updated, nil
}
