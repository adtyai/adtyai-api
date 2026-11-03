package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ClaimCodexResetRedeemID 返回这个渠道还没确认的重置幂等键。没有就记下 candidate。
// 并发调用只会留下第一次写入的键。
func ClaimCodexResetRedeemID(channelID int, candidate string) (string, error) {
	candidate = strings.TrimSpace(candidate)
	if channelID <= 0 {
		return "", fmt.Errorf("invalid channel id")
	}
	if candidate == "" || len(candidate) > 64 || strings.ContainsAny(candidate, " \t\r\n") {
		return "", fmt.Errorf("invalid redeem request id")
	}
	var chosen string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row Channel
		if err := lockForUpdate(tx).Select("id", "codex_reset_redeem_id").First(&row, "id = ?", channelID).Error; err != nil {
			return err
		}
		if existing := strings.TrimSpace(row.CodexResetRedeemID); existing != "" {
			chosen = existing
			return nil
		}
		result := tx.Model(&Channel{}).
			Where("id = ? AND (codex_reset_redeem_id IS NULL OR codex_reset_redeem_id = '')", channelID).
			Update("codex_reset_redeem_id", candidate)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var again Channel
			if err := tx.Select("codex_reset_redeem_id").First(&again, "id = ?", channelID).Error; err != nil {
				return err
			}
			chosen = strings.TrimSpace(again.CodexResetRedeemID)
			if chosen == "" {
				return fmt.Errorf("codex reset redeem id was not stored")
			}
			return nil
		}
		chosen = candidate
		return nil
	})
	return chosen, err
}

// ReleaseCodexResetRedeemID 只清掉刚刚被上游确认的那一枚键，避免清掉并发请求新占的键。
func ReleaseCodexResetRedeemID(channelID int, redeemID string) error {
	redeemID = strings.TrimSpace(redeemID)
	if channelID <= 0 || redeemID == "" {
		return fmt.Errorf("invalid codex reset redeem release")
	}
	return DB.Model(&Channel{}).
		Where("id = ? AND codex_reset_redeem_id = ?", channelID, redeemID).
		Update("codex_reset_redeem_id", "").Error
}
