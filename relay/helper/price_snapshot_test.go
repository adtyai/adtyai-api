package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperRetryReusesFrozenSnapshot(t *testing.T) {
	savedRatios, savedPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"snap-helper-model":2}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	info := &relaycommon.RelayInfo{UserId: 1, UsingGroup: "default", OriginModelName: "snap-helper-model"}
	price, err := ModelPriceHelper(ctx, info, 1000, &kittypes.TokenCountMeta{})
	require.NoError(t, err)
	snap := service.AdtyFrozenPriceSnapshot(ctx)
	require.NotNil(t, snap)
	require.True(t, snap.HasOfficialFields())
	assert.Equal(t, "2", snap.ModelBasePrice)
	assert.Equal(t, "1", snap.GroupRatio)
	assert.Equal(t, "0", snap.SiteMarkupRatio)
	assert.Equal(t, "1", snap.SiteCostFactor)
	assert.True(t, snap.MainSite)

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"snap-helper-model":9}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":4}`))
	retried, err := ModelPriceHelper(ctx, info, 1000, &kittypes.TokenCountMeta{})
	require.NoError(t, err)
	assert.Equal(t, price.ModelRatio, retried.ModelRatio)
	assert.True(t, retried.GroupRatioInfo.BilledGroupRatio().Equal(price.GroupRatioInfo.BilledGroupRatio()))
	reused := HandleGroupRatio(ctx, info)
	assert.True(t, reused.BilledGroupRatio().Equal(price.GroupRatioInfo.BilledGroupRatio()))
	assert.Equal(t, snap.FinalPrice, service.AdtyFrozenPriceSnapshot(ctx).FinalPrice)
}
