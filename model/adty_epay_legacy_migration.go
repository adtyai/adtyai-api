package model

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MigrateLegacyAdtyEpayChannel runs after persisted options have been loaded.
// Its durable anchor also serializes simultaneous startup on multiple nodes.
// Once migrated, gateway settings never overwrite the operator's channel edits.
func MigrateLegacyAdtyEpayChannel() error {
	gateway, merchant, secret := strings.TrimRight(strings.TrimSpace(operation_setting.PayAddress), "/"), strings.TrimSpace(operation_setting.EpayId), strings.TrimSpace(operation_setting.EpayKey)
	if gateway == "" || merchant == "" || secret == "" {
		return nil
	}
	methods := make([]AdtyEpayChannelMethod, 0, 2)
	seen := map[string]bool{}
	for _, method := range operation_setting.PayMethods {
		kind := method["type"]
		if (kind == "alipay" || kind == "wxpay") && !seen[kind] {
			seen[kind] = true
			methods = append(methods, AdtyEpayChannelMethod{Method: kind, TypeCode: kind, ReturnType: "jump", Enabled: true})
		}
	}
	if len(methods) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		marker := Option{Key: "adty_legacy_epay_channel_migrated", Value: ""}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where(commonKeyCol+" = ?", marker.Key).First(&marker).Error; err != nil {
			return err
		}
		if marker.Value != "" {
			return nil
		}
		var existing []AdtyEpayChannel
		if err := tx.Where("gateway_url = ? AND merchant_id = ?", gateway, merchant).Order("id").Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		id := 0
		if len(existing) > 0 {
			id = existing[0].Id
		} else {
			channel := AdtyEpayChannel{Name: "Legacy Epay", GatewayURL: gateway, MerchantId: merchant, MerchantKey: secret,
				Weight: 1, OrderMode: AdtyEpayOrderModeSubmit, Status: AdtyEpayStatusEnabled, Methods: methods}
			if err := tx.Create(&channel).Error; err != nil {
				return err
			}
			id = channel.Id
		}
		return tx.Model(&Option{}).Where(commonKeyCol+" = ?", marker.Key).Update("value", strconv.Itoa(id)).Error
	})
}
