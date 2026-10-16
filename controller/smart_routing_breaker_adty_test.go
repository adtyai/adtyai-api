package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/stretchr/testify/require"
)

// 审查 N-P1-2 / N-P2-2 / N-P2-7：后台推进半开、空候选回退、首次熔断事件、手动启用重置。
func TestBreakerHalfOpenLifecycle(t *testing.T) {
	setupRoutingConfigTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelBreakerState{}, &model.ChannelBreakerEvent{}))
	auto := 1
	ch := model.Channel{Id: 31, Name: "c31", Key: "k", Status: common.ChannelStatusEnabled, Models: "m", Group: "default", AutoBan: &auto}
	require.NoError(t, model.DB.Create(&ch).Error)

	smartrouting.TripImmediate(ch.Id, "test_trip")
	var ev model.ChannelBreakerEvent
	require.NoError(t, model.DB.Where("channel_id = ?", ch.Id).Order("id asc").First(&ev).Error)
	require.Equal(t, "closed", ev.FromState)
	require.Equal(t, "open", ev.ToState)

	// 冷却未到 → 无半开、无回退
	smartrouting.InvalidateHalfOpenCache()
	require.Nil(t, smartrouting.HalfOpenFallback("m", "default"))
	// 冷却到期 → 推进到 half_2，空候选可回退到该渠道
	require.NoError(t, model.DB.Model(&model.ChannelBreakerState{}).Where("channel_id = ?", ch.Id).Update("cooldown_until", time.Now().Add(-time.Second)).Error)
	list := smartrouting.AdvanceHalfOpen()
	require.Len(t, list, 1)
	require.Equal(t, "half_2", list[0].Stage)
	fb := smartrouting.HalfOpenFallback("m", "default")
	require.NotNil(t, fb)
	require.Equal(t, ch.Id, fb.Id)
	require.Nil(t, smartrouting.HalfOpenFallback("other-model", "default"))

	// 手动启用 → closed
	smartrouting.ResetBreakerOnManualEnable(ch.Id)
	var st model.ChannelBreakerState
	require.NoError(t, model.DB.First(&st, "channel_id = ?", ch.Id).Error)
	require.Equal(t, "closed", st.Stage)
	require.False(t, smartrouting.IsHalfOpenChannel(ch.Id))
}
