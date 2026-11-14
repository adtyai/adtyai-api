package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialBillingUsesPriceDataDecimalAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const literal = "0.1234567890123456789"
	exact := decimal.RequireFromString(literal)
	require.False(t, exact.Equal(decimal.NewFromFloat(exact.InexactFloat64())))

	price := hosttypes.PriceData{
		ModelRatio: exact.InexactFloat64(),
		GroupRatioInfo: hosttypes.GroupRatioInfo{
			GroupRatio: 1,
		},
	}
	price.SetBilledModelRatio(exact)
	price.GroupRatioInfo.SetBilledGroupRatio(decimal.NewFromInt(1))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "iss-r3-0004-ratio",
		UsingGroup:      "default",
		PriceData:       price,
	}

	AdtyFreezeRequestPriceSnapshot(ctx, info)
	snap := AdtyFrozenPriceSnapshot(ctx)
	require.NotNil(t, snap)
	assert.Equal(t, exact.String(), snap.ModelRatio)
	assert.Equal(t, exact.String(), snap.ModelBasePrice)
	assert.NotEqual(t, exact.String(), decimal.NewFromFloat(price.ModelRatio).String())

	quotaInfo := quotaInfoFromPriceData(QuotaInfo{
		InputDetails:  TokenDetails{TextTokens: 1000},
		OutputDetails: TokenDetails{},
		ModelName:     "iss-r3-0004-ratio",
		ModelRatio:    price.ModelRatio,
		GroupRatio:    1,
	}, info)
	require.True(t, quotaInfo.hasModelRatioDecimal)
	assert.True(t, quotaInfo.ModelRatioDecimal.Equal(exact))
	got, clamp := calculateAudioQuota(quotaInfo)
	assert.Nil(t, clamp)
	want, err := common.QuotaFromDecimalStrict(decimal.NewFromInt(1000).Mul(exact))
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.NotEqual(t, exact.String(), decimal.NewFromFloat(price.ModelRatio).String())

	summary := calculateTextQuotaSummary(ctx, info, &dto.Usage{PromptTokens: 1000, TotalTokens: 1000})
	assert.Equal(t, want, summary.Quota)
}

func TestPriceDataFloatFallbackKeepsLegacyAmounts(t *testing.T) {
	// Callers that only set the float fields must still bill as before.
	info := &relaycommon.RelayInfo{
		OriginModelName: "legacy-float-only",
		UsingGroup:      "default",
		PriceData: hosttypes.PriceData{
			ModelRatio:     2,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
	}
	info.PriceData.GroupRatioInfo.SetBilledGroupRatio(decimal.NewFromInt(1))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	summary := calculateTextQuotaSummary(ctx, info, &dto.Usage{PromptTokens: 1000, TotalTokens: 1000})
	assert.Equal(t, 2000, summary.Quota)
}
