package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestUpdateEpayChannelKeepsReturnType(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.AdtyEpayChannel{}, &model.AdtyEpayChannelMethod{}))
	channel := &model.AdtyEpayChannel{
		Name: "epay", GatewayURL: "https://pay.example.com", MerchantId: "m1", MerchantKey: "secret",
		Weight: 1, OrderMode: model.AdtyEpayOrderModeAPI, ProbeMode: "off", Status: model.AdtyEpayStatusDisabled,
		Methods: []model.AdtyEpayChannelMethod{{
			Method: "alipay", TypeCode: "alipay", Enabled: true, ReturnType: "qrcode",
			MinCNY: decimal.Zero, MaxCNY: decimal.Zero, DailyLimitCNY: decimal.Zero,
		}},
	}
	require.NoError(t, model.SaveAdtyEpayChannel(channel, false))

	body := `{"name":"epay","gateway_url":"https://pay.example.com","merchant_id":"m1","merchant_key":"","priority":0,"weight":1,"order_mode":"api","probe_mode":"off","unpaid_threshold":0,"methods":[{"method":"alipay","type_code":"alipay","min_cny":"0","max_cny":"0","daily_limit_cny":"0","enabled":true,"return_type":"qrcode"}]}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/adty/admin/epay/channels/"+strconv.Itoa(channel.Id), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
	UpdateAdtyEpayChannel(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)

	stored, err := model.GetAdtyEpayChannel(channel.Id)
	require.NoError(t, err)
	require.Len(t, stored.Methods, 1)
	require.Equal(t, "qrcode", stored.Methods[0].ReturnType)
}
