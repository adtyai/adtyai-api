package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionRejectsRetiredHostedPaymentChannels(t *testing.T) {
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	require.NoError(t, DB.AutoMigrate(&Option{}))

	previousStripe := setting.StripeApiSecret
	previousEnabled := setting.WaffoEnabled
	previousMin := operation_setting.MinTopUp
	previousPayAddress := operation_setting.PayAddress
	previousPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		setting.StripeApiSecret = previousStripe
		setting.WaffoEnabled = previousEnabled
		operation_setting.MinTopUp = previousMin
		operation_setting.PayAddress = previousPayAddress
		operation_setting.PayMethods = previousPayMethods
		DB.Exec("DELETE FROM options WHERE key IN ?", []string{
			"StripeApiSecret", "WaffoEnabled", "CreemApiKey", "WaffoPancakeEnvironment",
			"MinTopUp", "PayAddress", "PayMethods",
		})
	})

	setting.StripeApiSecret = "sk_existing_secret"
	require.NoError(t, DB.Where("key = ?", "StripeApiSecret").Delete(&Option{}).Error)
	require.NoError(t, DB.Create(&Option{Key: "StripeApiSecret", Value: "sk_existing_secret"}).Error)

	for _, key := range []string{
		"StripeApiSecret",
		"StripeWebhookSecret",
		"StripeMinTopUp",
		"CreemApiKey",
		"CreemProducts",
		"WaffoEnabled",
		"WaffoApiKey",
		"WaffoPancakeMerchantID",
		"WaffoPancakeEnvironment",
	} {
		t.Run(key, func(t *testing.T) {
			err := UpdateOption(key, "prod")
			require.ErrorIs(t, err, ErrRetiredHostedPaymentChannel)
		})
	}

	var stored Option
	require.NoError(t, DB.Where("key = ?", "StripeApiSecret").First(&stored).Error)
	assert.Equal(t, "sk_existing_secret", stored.Value)
	assert.Equal(t, "sk_existing_secret", setting.StripeApiSecret)

	err := UpdateOptionsBulk(map[string]string{"WaffoEnabled": "true", "MinTopUp": "12"})
	require.ErrorIs(t, err, ErrRetiredHostedPaymentChannel)
	assert.Equal(t, previousMin, operation_setting.MinTopUp)

	require.ErrorIs(t, UpdateOption("PayMethods", `[{"name":"Stripe","type":"stripe"}]`), ErrRetiredHostedPaymentChannel)
	require.ErrorIs(t, UpdateOption("PayMethods", `[{"name":"Alipay","type":"alipay"},{"name":"Waffo Pancake","type":"waffo_pancake"}]`), ErrRetiredHostedPaymentChannel)
	require.NoError(t, UpdateOption("PayMethods", `[{"name":"支付宝","type":"alipay"},{"name":"微信","type":"wxpay"}]`))
	require.Equal(t, "alipay", operation_setting.PayMethods[0]["type"])

	require.NoError(t, UpdateOption("MinTopUp", "25"))
	assert.Equal(t, 25, operation_setting.MinTopUp)
	require.NoError(t, UpdateOption("PayAddress", "https://epay.example/pay"))
	assert.Equal(t, "https://epay.example/pay", operation_setting.PayAddress)
}

func TestFilterRetiredPayMethodsDropsHostedTypes(t *testing.T) {
	filtered := FilterRetiredPayMethods([]map[string]string{
		{"name": "支付宝", "type": "alipay"},
		{"name": "Stripe", "type": "stripe"},
		{"name": "Creem", "type": "creem"},
		{"name": "微信", "type": "wxpay"},
		{"name": "Waffo", "type": "waffo"},
	})
	require.Len(t, filtered, 2)
	assert.Equal(t, "alipay", filtered[0]["type"])
	assert.Equal(t, "wxpay", filtered[1]["type"])
}
