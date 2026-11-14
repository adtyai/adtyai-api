package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionAndTopUpInfoKeepRetiredHostedChannelsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}

	previousSecret := setting.StripeApiSecret
	previousPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		setting.StripeApiSecret = previousSecret
		operation_setting.PayMethods = previousPayMethods
	})
	setting.StripeApiSecret = "sk_live_existing"
	operation_setting.PayMethods = []map[string]string{
		{"name": "支付宝", "type": "alipay"},
		{"name": "Stripe", "type": "stripe"},
	}

	for _, key := range []string{"StripeApiSecret", "CreemApiKey", "WaffoEnabled", "WaffoPancakeEnvironment"} {
		body, err := common.Marshal(OptionUpdateRequest{Key: key, Value: "true"})
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))
		UpdateOption(ctx)
		assert.Contains(t, recorder.Body.String(), `"success":false`)
		assert.Contains(t, recorder.Body.String(), "no longer operable collection channels")
	}
	assert.Equal(t, "sk_live_existing", setting.StripeApiSecret)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	GetTopUpInfo(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var body struct {
		Success bool
		Data    struct {
			EnableStripeTopup       bool                `json:"enable_stripe_topup"`
			EnableCreemTopup        bool                `json:"enable_creem_topup"`
			EnableWaffoTopup        bool                `json:"enable_waffo_topup"`
			EnableWaffoPancakeTopup bool                `json:"enable_waffo_pancake_topup"`
			PayMethods              []map[string]string `json:"pay_methods"`
			WaffoPayMethods         []any               `json:"waffo_pay_methods"`
			CreemProducts           string              `json:"creem_products"`
		}
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.False(t, body.Data.EnableStripeTopup)
	require.False(t, body.Data.EnableCreemTopup)
	require.False(t, body.Data.EnableWaffoTopup)
	require.False(t, body.Data.EnableWaffoPancakeTopup)
	require.Len(t, body.Data.PayMethods, 1)
	assert.Equal(t, "alipay", body.Data.PayMethods[0]["type"])
	assert.Empty(t, body.Data.WaffoPayMethods)
	assert.Empty(t, body.Data.CreemProducts)
}
