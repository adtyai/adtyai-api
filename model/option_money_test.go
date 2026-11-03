package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaPerUnitRejectsGarbageWithoutZeroing(t *testing.T) {
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	require.NoError(t, DB.AutoMigrate(&Option{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM options WHERE key = ?", "QuotaPerUnit") })
	old := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = old })

	require.Error(t, UpdateOption("QuotaPerUnit", "nope"))
	assert.Equal(t, 500000.0, common.QuotaPerUnit)
	require.Error(t, UpdateOption("QuotaPerUnit", "0"))
	assert.Equal(t, 500000.0, common.QuotaPerUnit)

	var stored int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", "QuotaPerUnit").Count(&stored).Error)
	assert.Zero(t, stored)

	require.NoError(t, UpdateOption("QuotaPerUnit", "250000"))
	assert.Equal(t, 250000.0, common.QuotaPerUnit)
}

func TestPriceAndTopupRatioKeepPreviousValue(t *testing.T) {
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	oldPrice, oldMin := operation_setting.Price, operation_setting.MinTopUp
	t.Cleanup(func() {
		operation_setting.Price = oldPrice
		operation_setting.MinTopUp = oldMin
	})
	operation_setting.Price = 7.3
	operation_setting.MinTopUp = 1
	require.Error(t, updateOptionMap("Price", "abc"))
	assert.Equal(t, 7.3, operation_setting.Price)
	require.Error(t, updateOptionMap("MinTopUp", "abc"))
	assert.Equal(t, 1, operation_setting.MinTopUp)

	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1.5}`))
	require.Error(t, UpdateOption("TopupGroupRatio", "{"))
	require.Error(t, common.UpdateTopupGroupRatioByJSONString("{"))
	assert.Equal(t, 1.5, common.GetTopupGroupRatio("default"))
}

func TestPaymentAmountJSONRejectsGarbageWithoutReplacingPresets(t *testing.T) {
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	require.NoError(t, DB.AutoMigrate(&Option{}))
	setting := operation_setting.GetPaymentSetting()
	oldOptions := append([]int(nil), setting.AmountOptions...)
	oldDiscount := map[int]float64{}
	for amount, rate := range setting.AmountDiscount {
		oldDiscount[amount] = rate
	}
	t.Cleanup(func() {
		setting.AmountOptions = oldOptions
		setting.AmountDiscount = oldDiscount
		DB.Where("key IN ?", []string{"payment_setting.amount_options", "payment_setting.amount_discount"}).Delete(&Option{})
	})

	require.NoError(t, UpdateOption("payment_setting.amount_options", "[10,30,80]"))
	assert.Equal(t, []int{10, 30, 80}, setting.AmountOptions)
	require.NoError(t, UpdateOption("payment_setting.amount_discount", `{"30":0.9}`))
	assert.InDelta(t, 0.9, setting.AmountDiscount[30], 0.0001)

	require.Error(t, UpdateOption("payment_setting.amount_options", "{"))
	require.Error(t, UpdateOption("payment_setting.amount_discount", "not-json"))
	assert.Equal(t, []int{10, 30, 80}, setting.AmountOptions)
	assert.InDelta(t, 0.9, setting.AmountDiscount[30], 0.0001)

	var stored []Option
	require.NoError(t, DB.Where("key IN ?", []string{"payment_setting.amount_options", "payment_setting.amount_discount"}).Find(&stored).Error)
	saved := map[string]string{}
	for _, row := range stored {
		saved[row.Key] = row.Value
	}
	assert.Equal(t, "[10,30,80]", saved["payment_setting.amount_options"])
	assert.JSONEq(t, `{"30":0.9}`, saved["payment_setting.amount_discount"])

	setting.AmountOptions = []int{1}
	setting.AmountDiscount = map[int]float64{}
	require.NoError(t, updateOptionMap("payment_setting.amount_options", saved["payment_setting.amount_options"]))
	require.NoError(t, updateOptionMap("payment_setting.amount_discount", saved["payment_setting.amount_discount"]))
	assert.Equal(t, []int{10, 30, 80}, setting.AmountOptions)
	assert.InDelta(t, 0.9, setting.AmountDiscount[30], 0.0001)
}

func TestOptionSaveFailureDoesNotChangeQuotaPerUnit(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Migrator().DropTable(&Option{}))
	t.Cleanup(func() { _ = DB.AutoMigrate(&Option{}) })
	old := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = old })

	require.Error(t, UpdateOption("QuotaPerUnit", "500000"))
	assert.Equal(t, 1.0, common.QuotaPerUnit)
}
