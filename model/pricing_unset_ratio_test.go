package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

// 渠道启用但没单独标价的模型不能把 37.5 兜底倍率放进价格页。配了价的模型仍返回原倍率。
func TestUnconfiguredModelRatioIsOmittedFromPricing(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &Model{}, &Vendor{}))

	savedRatios := ratio_setting.ModelRatio2JSONString()
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedSelfUse := operation_setting.SelfUseModeEnabled
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		operation_setting.SelfUseModeEnabled = savedSelfUse
		InvalidatePricingCache()
	})
	operation_setting.SelfUseModeEnabled = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"f041-priced-model":2.5}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))

	channel := &Channel{
		Name:   "f041-pricing",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-test",
		Status: 1,
		Group:  "default",
		Models: "f041-unset-model,f041-priced-model",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{
		Group: "default", Model: "f041-unset-model", ChannelId: channel.Id, Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&Ability{
		Group: "default", Model: "f041-priced-model", ChannelId: channel.Id, Enabled: true,
	}).Error)

	InvalidatePricingCache()
	pricing := GetPricing()
	var unset, priced *Pricing
	for i := range pricing {
		switch pricing[i].ModelName {
		case "f041-unset-model":
			unset = &pricing[i]
		case "f041-priced-model":
			priced = &pricing[i]
		}
	}
	require.NotNil(t, unset)
	require.Nil(t, unset.ModelRatio)
	require.Equal(t, 0, unset.QuotaType)
	raw, err := common.Marshal(unset)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "37.5")
	require.False(t, strings.Contains(string(raw), `"model_ratio"`))

	require.NotNil(t, priced)
	require.NotNil(t, priced.ModelRatio)
	require.Equal(t, 2.5, *priced.ModelRatio)
}
