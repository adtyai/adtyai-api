package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperWiresDecimalAuthorityIntoBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const literal = "0.1234567890123456789"
	exact := decimal.RequireFromString(literal)
	require.False(t, exact.Equal(decimal.NewFromFloat(exact.InexactFloat64())),
		"the test coefficient must actually be inexact in float64")

	savedRatios := ratio_setting.ModelRatio2JSONString()
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"iss-r3-0004-ratio":`+literal+`}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "iss-r3-0004-ratio",
		UserGroup:       "default",
		UsingGroup:      "default",
	}

	price, err := ModelPriceHelper(ctx, info, 1000, &kittypes.TokenCountMeta{})
	require.NoError(t, err)
	require.True(t, price.HasBilledModelRatio())
	assert.True(t, price.BilledModelRatio().Equal(exact), "got %s", price.BilledModelRatio())
	assert.Equal(t, exact.InexactFloat64(), price.ModelRatio, "float field stays the compatibility view")

	want, err := common.QuotaFromDecimalStrict(decimal.NewFromFloat(1000).Mul(exact))
	require.NoError(t, err)
	assert.Equal(t, want, price.QuotaToPreConsume)
	assert.NotEqual(t, exact.String(), decimal.NewFromFloat(price.ModelRatio).String(),
		"financial math must keep the stored decimal, not NewFromFloat of the compatibility view")

	snap := service.AdtyFrozenPriceSnapshot(ctx)
	require.NotNil(t, snap)
	assert.Equal(t, exact.String(), snap.ModelRatio)
	assert.Equal(t, exact.String(), snap.ModelBasePrice)

	// Representable ratios must keep the same pre-consume as the old float path.
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"iss-r3-0004-exact2":2}`))
	ctx2, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx2.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	ctx2.Set("group", "default")
	info2 := &relaycommon.RelayInfo{OriginModelName: "iss-r3-0004-exact2", UserGroup: "default", UsingGroup: "default"}
	price2, err := ModelPriceHelper(ctx2, info2, 1000, &kittypes.TokenCountMeta{})
	require.NoError(t, err)
	want2, err := common.QuotaFromDecimalStrict(decimal.NewFromFloat(1000).Mul(decimal.NewFromInt(2)))
	require.NoError(t, err)
	assert.Equal(t, want2, price2.QuotaToPreConsume)
	assert.Equal(t, 2000, price2.QuotaToPreConsume)

	// Per-call / per-price path stores the decimal authority and keeps the
	// existing QuotaFromFloatStrict pre-consume (no rounding-policy change).
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"iss-r3-0004-price":0.1}`))
	ctx3, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx3.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	ctx3.Set("group", "default")
	info3 := &relaycommon.RelayInfo{OriginModelName: "iss-r3-0004-price", UserGroup: "default", UsingGroup: "default"}
	price3, err := ModelPriceHelper(ctx3, info3, 0, &kittypes.TokenCountMeta{})
	require.NoError(t, err)
	require.True(t, price3.UsePrice)
	require.True(t, price3.HasBilledModelPrice())
	assert.True(t, price3.BilledModelPrice().Equal(decimal.RequireFromString("0.1")))
	want3, err := common.QuotaFromFloatStrict(0.1 * common.QuotaPerUnit * 1)
	require.NoError(t, err)
	assert.Equal(t, want3, price3.QuotaToPreConsume)
	snap3 := service.AdtyFrozenPriceSnapshot(ctx3)
	require.NotNil(t, snap3)
	assert.Equal(t, "0.1", snap3.ModelPrice)
	assert.Equal(t, "0.1", snap3.ModelBasePrice)
}
