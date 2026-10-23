package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 审查 R-P1-1：所有渠道都熔断、冷却到期后，经过 Distribute 中间件（每个请求都带
// FilterRequestPath 过滤）仍能回退到半开渠道，而不是永久 503。
func TestDistributeAllBrokenFallsBackToHalfOpen(t *testing.T) {
	require.NoError(t, i18n.Init())
	setupRoutingConfigTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelBreakerState{}, &model.ChannelBreakerEvent{}))
	prevGroups := setting.UserUsableGroups2JSONString()
	prevRatios, err := common.Marshal(ratio_setting.GetGroupRatioCopy())
	require.NoError(t, err)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	prevMem := common.MemoryCacheEnabled
	prevW, prevS, prevProbe, prevF := model.SmartWeightHook, model.OnChannelSelectedHook, model.OnHalfOpenProbeHook, model.HalfOpenFallbackHook
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(prevGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(string(prevRatios))
		common.MemoryCacheEnabled = prevMem
		model.SmartWeightHook, model.OnChannelSelectedHook, model.OnHalfOpenProbeHook, model.HalfOpenFallbackHook = prevW, prevS, prevProbe, prevF
	})
	model.RegisterSmartWeightHook(smartrouting.SmartWeights)
	model.RegisterOnChannelSelectedHook(smartrouting.OnChannelSelected)
	model.RegisterOnHalfOpenProbeHook(smartrouting.ReserveHalfOpenProbe)
	model.RegisterHalfOpenFallbackHook(smartrouting.HalfOpenFallback)

	auto := 1
	ids := []int{41, 42}
	for _, id := range ids {
		ch := model.Channel{Id: id, Name: fmt.Sprintf("c%d", id), Type: 1, Key: "k", Status: common.ChannelStatusEnabled, Models: "fb-model", Group: "default", AutoBan: &auto}
		require.NoError(t, model.DB.Create(&ch).Error)
		require.NoError(t, ch.AddAbilities(model.DB))
		smartrouting.TripImmediate(id, "test_trip")
	}
	var stillEnabled int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id IN ? AND status = ?", ids, common.ChannelStatusEnabled).Count(&stillEnabled).Error)
	require.EqualValues(t, len(ids), stillEnabled, "breaker must not write channel status")
	var open int64
	require.NoError(t, model.DB.Model(&model.ChannelBreakerState{}).Where("channel_id IN ? AND stage = ?", ids, "open").Count(&open).Error)
	require.EqualValues(t, len(ids), open, "breaker stage must be open")
	require.NoError(t, model.DB.Model(&model.ChannelBreakerState{}).Where("channel_id IN ?", ids).Update("cooldown_until", time.Now().Add(-time.Second)).Error)
	require.Len(t, smartrouting.AdvanceHalfOpen(), len(ids))

	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", cached), func(t *testing.T) {
			common.MemoryCacheEnabled = cached
			model.InitChannelCache()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"fb-model"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			middleware.Distribute()(c)
			require.False(t, c.IsAborted(), "status=%d body=%s", rec.Code, rec.Body.String())
			got := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
			require.Contains(t, ids, got)
			smartrouting.DecHalfOpenInflight(got)
		})
	}
}
