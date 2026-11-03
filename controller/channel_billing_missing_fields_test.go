package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestBatchBalanceUpdateIgnoresIncompleteSuccessJSON(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		key := r.Header.Get("Authorization")
		usage := strings.Contains(r.URL.Path, "/usage")
		switch {
		case strings.Contains(key, "empty-json"):
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(key, "explicit-zero"):
			if usage {
				_, _ = w.Write([]byte(`{"total_usage":0}`))
			} else {
				_, _ = w.Write([]byte(`{"hard_limit_usd":0,"has_payment_method":true}`))
			}
		default:
			if usage {
				_, _ = w.Write([]byte(`{"total_usage":100}`))
			} else {
				_, _ = w.Write([]byte(`{"hard_limit_usd":5,"has_payment_method":true}`))
			}
		}
	}))
	t.Cleanup(upstream.Close)

	autoBan := 1
	base := upstream.URL
	incomplete := &model.Channel{
		Name: "incomplete-json", Type: constant.ChannelTypeCustom, Key: "sk-empty-json",
		Status: common.ChannelStatusEnabled, BaseURL: &base, Balance: 12.5, AutoBan: &autoBan,
	}
	exhausted := &model.Channel{
		Name: "explicit-zero", Type: constant.ChannelTypeOpenAI, Key: "sk-explicit-zero",
		Status: common.ChannelStatusEnabled, BaseURL: &base, Balance: 9, AutoBan: &autoBan,
	}
	funded := &model.Channel{
		Name: "explicit-balance", Type: constant.ChannelTypeOpenAI, Key: "sk-funded",
		Status: common.ChannelStatusEnabled, BaseURL: &base, Balance: 1, AutoBan: &autoBan,
	}
	require.NoError(t, db.Create(incomplete).Error)
	require.NoError(t, db.Create(exhausted).Error)
	require.NoError(t, db.Create(funded).Error)

	require.NoError(t, updateAllChannelsBalance())

	var storedIncomplete, storedExhausted, storedFunded model.Channel
	require.NoError(t, db.First(&storedIncomplete, incomplete.Id).Error)
	require.NoError(t, db.First(&storedExhausted, exhausted.Id).Error)
	require.NoError(t, db.First(&storedFunded, funded.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, storedIncomplete.Status)
	require.Equal(t, 12.5, storedIncomplete.Balance)
	require.Equal(t, common.ChannelStatusAutoDisabled, storedExhausted.Status)
	require.Equal(t, 0.0, storedExhausted.Balance)
	require.Equal(t, common.ChannelStatusEnabled, storedFunded.Status)
	require.Equal(t, 4.0, storedFunded.Balance)
}
