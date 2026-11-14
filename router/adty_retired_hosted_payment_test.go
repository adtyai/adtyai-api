package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetiredHostedPaymentWritesAreRejectedAndCannotBeBypassed(t *testing.T) {
	f := newAdtyGuardFixture(t)
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	previousSecret := setting.StripeApiSecret
	t.Cleanup(func() {
		setting.StripeApiSecret = previousSecret
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	setting.StripeApiSecret = "sk_existing"

	for _, token := range []string{"", f.token} {
		resp := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"StripeApiSecret","value":"sk_bypass"}`, token)
		assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, resp.Code, resp.Body.String())
	}
	rootWrite := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"StripeApiSecret","value":"sk_bypass"}`, f.root)
	require.Equal(t, http.StatusOK, rootWrite.Code, rootWrite.Body.String())
	assert.Contains(t, rootWrite.Body.String(), `"success":false`)
	assert.Contains(t, rootWrite.Body.String(), "no longer operable collection channels")
	assert.Equal(t, "sk_existing", setting.StripeApiSecret)

	supported := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"MinTopUp","value":"30"}`, f.root)
	require.Equal(t, http.StatusOK, supported.Code, supported.Body.String())
	assert.Contains(t, supported.Body.String(), `"success":true`)

	adminMin := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"MinTopUp","value":"99"}`, f.token)
	assert.Equal(t, http.StatusForbidden, adminMin.Code)

	const path = "/api/adty/admin/payment-scenes"
	initial := adtyJSON(t, f.engine, http.MethodGet, path, "", f.root)
	require.Equal(t, http.StatusOK, initial.Code, initial.Body.String())
	assert.NotContains(t, initial.Body.String(), `"provider":"stripe"`)
	assert.NotContains(t, initial.Body.String(), `"provider":"creem"`)
	assert.Contains(t, initial.Body.String(), `"provider":"bepusdt"`)

	for _, token := range []string{"", f.token} {
		resp := adtyJSON(t, f.engine, http.MethodPut, path, `{"expected_version":1,"scenes":[{"key":"wallet_topup","name":"Wallet top-up","business":"wallet_topup","enabled":true,"providers":["stripe"],"methods":[],"channel_ids":[],"currencies":[],"site_scope":"all","site_ids":[],"user_types":[],"min_usd":"0","max_usd":"0"}]}`, token)
		assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, resp.Code, resp.Body.String())
	}

	retired := adtyJSON(t, f.engine, http.MethodPut, path, `{"expected_version":1,"scenes":[{"key":"wallet_topup","name":"Wallet top-up","business":"wallet_topup","enabled":true,"providers":["stripe"],"methods":[],"channel_ids":[],"currencies":[],"site_scope":"all","site_ids":[],"user_types":[],"min_usd":"0","max_usd":"0"}]}`, f.root)
	assert.Contains(t, retired.Body.String(), `"success":false`)
	assert.Contains(t, retired.Body.String(), "no longer operable collection channels")

	official := adtyJSON(t, f.engine, http.MethodPut, path, `{"expected_version":1,"scenes":[{"key":"wallet_topup","name":"Wallet top-up","business":"wallet_topup","enabled":true,"providers":["bepusdt"],"methods":[],"channel_ids":[],"currencies":[],"site_scope":"all","site_ids":[],"user_types":[],"min_usd":"0","max_usd":"0"}]}`, f.root)
	require.Equal(t, http.StatusOK, official.Code, official.Body.String())
	assert.Contains(t, official.Body.String(), `"success":true`)
	assert.Contains(t, official.Body.String(), `"providers":["bepusdt"]`)

}
