package controller

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func epayTestSign(p map[string]string, key string) string {
	keys := make([]string, 0, len(p))
	for k, v := range p {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+p[k])
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + key))
	return hex.EncodeToString(sum[:])
}

// 审查 R-P2-8：已入账（人工补单）的订单再收到另一个 trade_no 的回调：返回 success、不重复入账。
func TestEpayNotifyAlreadyPaidIsIdempotentSuccess(t *testing.T) {
	t.Setenv("ADTY_SECRET_KEY", "test-secret-key-for-epay-notify-0123456789")
	_, aliceID := seedCompleteFixture(t)
	inst, err := model.CreateEpayInstance(model.CreateEpayInstanceInput{Name: "i1", PartnerId: "2002", Secret: "sk-test", PayAddress: "https://pay.example.com", Enabled: true})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.TopupOrder{}).Where("order_no = ?", "P11-EPAY").Updates(map[string]any{
		"status": model.TopupOrderStatusPaid, "provider_trade_no": "GW-001", "epay_instance_id": inst.Id, "pay_cny": 72.0,
	}).Error)
	var before model.User
	require.NoError(t, model.DB.First(&before, aliceID).Error)

	p := map[string]string{"pid": "2002", "trade_no": "GW-999", "out_trade_no": "P11-EPAY", "type": "alipay", "name": "topup", "money": "72.00", "trade_status": "TRADE_SUCCESS"}
	p["sign"] = epayTestSign(p, "sk-test")
	p["sign_type"] = "MD5"
	q := url.Values{}
	for k, v := range p {
		q.Set(k, v)
	}
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/pay/epay/notify/"+strconv.FormatInt(inst.Id, 10)+"?"+q.Encode(), nil)
	c.Params = gin.Params{{Key: "instance_id", Value: strconv.FormatInt(inst.Id, 10)}}
	AdtyEpayNotify(c)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "success", rec.Body.String())

	var after model.User
	require.NoError(t, model.DB.First(&after, aliceID).Error)
	require.Equal(t, before.Quota, after.Quota, "must not credit twice")
	var o model.TopupOrder
	require.NoError(t, model.DB.Where("order_no = ?", "P11-EPAY").First(&o).Error)
	require.Equal(t, "GW-001", o.ProviderTradeNo)
}
