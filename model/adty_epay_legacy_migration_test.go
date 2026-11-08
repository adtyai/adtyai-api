package model

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestLegacyEpayMigrationIsIdempotentAndKeepsOperatorEdits(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}, &AdtyEpayChannel{}, &AdtyEpayChannelMethod{}))
	oldURL, oldID, oldKey, oldMethods := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://epay.example", "merchant-old", "synthetic-secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}, {"type": "wxpay"}}
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods = oldURL, oldID, oldKey, oldMethods
	})
	for range 2 {
		require.NoError(t, MigrateLegacyAdtyEpayChannel())
	}
	channels, err := ListAdtyEpayChannels()
	require.NoError(t, err)
	require.Len(t, channels, 1)
	require.Equal(t, "merchant-old", channels[0].MerchantId)
	require.Equal(t, AdtyEpayStatusEnabled, channels[0].Status)
	require.Len(t, channels[0].Methods, 2)
	require.NoError(t, db.Model(&AdtyEpayChannel{}).Where("id = ?", channels[0].Id).Updates(map[string]any{"name": "Operator renamed", "status": AdtyEpayStatusDisabled}).Error)
	operation_setting.EpayId, operation_setting.EpayKey = "merchant-new", "new-secret"
	require.NoError(t, MigrateLegacyAdtyEpayChannel())
	channels, err = ListAdtyEpayChannels()
	require.NoError(t, err)
	require.Len(t, channels, 1)
	require.Equal(t, "Operator renamed", channels[0].Name)
	require.Equal(t, "merchant-old", channels[0].MerchantId)
	require.Equal(t, AdtyEpayStatusDisabled, channels[0].Status)
}

func TestLegacyEpayMigrationSerializesConcurrentStartupAndReusesChannel(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}, &AdtyEpayChannel{}, &AdtyEpayChannelMethod{}))
	oldURL, oldID, oldKey, oldMethods := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods = oldURL, oldID, oldKey, oldMethods
	})
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://epay.concurrent.example/", "merchant-concurrent", "synthetic-secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}, {"type": "alipay"}, {"type": "unsupported"}}
	existing := AdtyEpayChannel{Name: "Operator channel", GatewayURL: "https://epay.concurrent.example", MerchantId: "merchant-concurrent", MerchantKey: "operator-secret",
		Weight: 7, Status: AdtyEpayStatusDisabled, OrderMode: AdtyEpayOrderModeSubmit,
		Methods: []AdtyEpayChannelMethod{{Method: "wxpay", TypeCode: "custom-wx", ReturnType: "jump", Enabled: true}}}
	require.NoError(t, db.Create(&existing).Error)
	start := make(chan struct{})
	errors := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { <-start; errors <- MigrateLegacyAdtyEpayChannel() })
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	channels, err := ListAdtyEpayChannels()
	require.NoError(t, err)
	require.Len(t, channels, 1)
	require.Equal(t, existing.Id, channels[0].Id)
	require.Equal(t, "operator-secret", channels[0].MerchantKey)
	require.Equal(t, AdtyEpayStatusDisabled, channels[0].Status)
	require.Len(t, channels[0].Methods, 1)
	require.Equal(t, "custom-wx", channels[0].Methods[0].TypeCode)
}

func TestLegacyEpayMigrationWaitsForAUsableConfiguration(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}, &AdtyEpayChannel{}, &AdtyEpayChannelMethod{}))
	oldURL, oldID, oldKey, oldMethods := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods = oldURL, oldID, oldKey, oldMethods
	})
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://epay.later.example", "merchant-later", ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	require.NoError(t, MigrateLegacyAdtyEpayChannel())
	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	require.Zero(t, count, "an incomplete config must not permanently mark the migration complete")
	operation_setting.EpayKey = "later-secret"
	for range 2 {
		require.NoError(t, MigrateLegacyAdtyEpayChannel())
	}
	channels, err := ListAdtyEpayChannels()
	require.NoError(t, err)
	require.Len(t, channels, 1)
}
