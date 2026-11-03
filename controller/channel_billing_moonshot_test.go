package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestMoonshotBalanceRejectsNonPositivePrice(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"status":true,"data":{"available_balance":73,"voucher_balance":0,"cash_balance":73}}`))
	}))
	t.Cleanup(upstream.Close)
	previousURL := moonshotBalanceURL
	moonshotBalanceURL = upstream.URL
	t.Cleanup(func() { moonshotBalanceURL = previousURL })

	previousPrice := operation_setting.Price
	t.Cleanup(func() { operation_setting.Price = previousPrice })

	channel := &model.Channel{Name: "moonshot-price", Type: constant.ChannelTypeMoonshot, Key: "sk-test", Status: 1}
	require.NoError(t, db.Create(channel).Error)

	operation_setting.Price = 0
	_, err := updateChannelMoonshotBalance(channel)
	require.Error(t, err)
	operation_setting.Price = -2
	_, err = updateChannelMoonshotBalance(channel)
	require.Error(t, err)

	var stored model.Channel
	require.NoError(t, db.Select("balance").First(&stored, channel.Id).Error)
	require.Zero(t, stored.Balance)

	operation_setting.Price = 7.3
	balance, err := updateChannelMoonshotBalance(channel)
	require.NoError(t, err)
	require.InDelta(t, 10, balance, 0.0001)
	require.NoError(t, db.Select("balance").First(&stored, channel.Id).Error)
	require.InDelta(t, 10, stored.Balance, 0.0001)
}
