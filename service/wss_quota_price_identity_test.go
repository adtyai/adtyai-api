package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

// ISS-R4-0096: a realtime settlement must price with one model identity.
//
// The only production caller (relay/websocket.go:59) passes UpstreamModelName
// while the channel maps the client model to another name. The three ratios and
// the quota fallback must resolve against the billing model, otherwise the
// charge and the ratios recorded in the consume log come from two price tables.
func TestPostWssConsumeQuotaPricesWithOneModelIdentity(t *testing.T) {
	db := prepareFloorDB(t)
	user, token := floorCustomer(t, db, nil, "wss-price-identity", 1_000_000)
	ctx, info, _ := floorRelay(t, user, token)
	info.OriginModelName = "gpt-4o-realtime"
	info.PriceData.UsePrice = false
	info.PriceData.ModelPrice = 0
	info.PriceData.ModelRatio = 1

	// model_mapping: the price tables do not know the upstream name, so every
	// ratio looked up with it falls back to 1.
	const upstreamModel = "gpt-4o-realtime-2026-11-15-mapped"

	// The audio completion table is empty in tests; seed exactly the billed model.
	savedAudioCompletion := ratio_setting.AudioCompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(savedAudioCompletion))
	})
	require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(`{"gpt-4o-realtime": 2}`))

	require.Nil(t, PreConsumeBilling(ctx, 4_000, info))
	usage := &dto.RealtimeUsage{
		InputTokens: 1000, OutputTokens: 1000, TotalTokens: 2000,
		OutputTokenDetails: dto.OutputTokenDetails{AudioTokens: 1000},
	}
	require.NoError(t, PostWssConsumeQuota(ctx, info, upstreamModel, usage, nil))

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", user.Id, model.LogTypeConsume).Take(&log).Error)
	// 1000 output audio tokens x audio_ratio(1) x audio_completion_ratio(2)
	// gpt-4o-realtime is the billed model; its audio completion ratio is 2.
	require.Equal(t, 2_000, log.Quota)

	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	require.EqualValues(t, 2, other["audio_completion_ratio"], "logged ratio must come from the billed price identity")
	require.EqualValues(t, 1, other["audio_ratio"])
}
