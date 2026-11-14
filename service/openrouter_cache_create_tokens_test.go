package service

import (
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func openRouterCacheCreateCost(modelRatio, cacheCreationRatio, cacheRatio, completionRatio float64, prompt, cacheRead, cacheCreate, completion int) float64 {
	quotaPrice := decimal.NewFromFloat(modelRatio).Div(common.QuotaPerUnitDecimal())
	cost := decimal.NewFromInt(int64(prompt - cacheRead - cacheCreate)).Mul(quotaPrice).
		Add(decimal.NewFromInt(int64(cacheRead)).Mul(quotaPrice.Mul(decimal.NewFromFloat(cacheRatio)))).
		Add(decimal.NewFromInt(int64(cacheCreate)).Mul(quotaPrice.Mul(decimal.NewFromFloat(cacheCreationRatio)))).
		Add(decimal.NewFromInt(int64(completion)).Mul(quotaPrice.Mul(decimal.NewFromFloat(completionRatio))))
	f, _ := cost.Float64()
	return f
}

func TestCalcOpenRouterCacheCreateTokensInexactPriceDoesNotContaminateTokens(t *testing.T) {
	const (
		modelRatio         = 0.1
		cacheCreationRatio = 2.0
		cacheRatio         = 0.1
		completionRatio    = 1.0
		promptTokens       = 1000
		cacheReadTokens    = 100
		cacheCreateTokens  = 200
		completionTokens   = 50
	)
	priceData := hosttypes.PriceData{
		ModelRatio:         modelRatio,
		CacheCreationRatio: cacheCreationRatio,
		CacheRatio:         cacheRatio,
		CompletionRatio:    completionRatio,
	}
	got := CalcOpenRouterCacheCreateTokens(dto.Usage{
		Cost:             openRouterCacheCreateCost(modelRatio, cacheCreationRatio, cacheRatio, completionRatio, promptTokens, cacheReadTokens, cacheCreateTokens, completionTokens),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: cacheReadTokens,
		},
	}, priceData)
	require.Equal(t, cacheCreateTokens, got)
}

func TestCalcOpenRouterCacheCreateTokensInexactPriceDoesNotFollowFloatRounding(t *testing.T) {
	// 0.1 is not binary-exact. The same inversion in float64 rounds this
	// case to 6006790; exact decimal rounding (half away from zero / half up)
	// yields 6006791. That token count is the input to later settlement.
	priceData := hosttypes.PriceData{
		ModelRatio:         0.1,
		CacheCreationRatio: 2,
		CacheRatio:         0.1,
		CompletionRatio:    1,
	}
	got := CalcOpenRouterCacheCreateTokens(dto.Usage{
		Cost:             1.23,
		PromptTokens:     123456,
		CompletionTokens: 30864,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12345,
		},
	}, priceData)
	require.Equal(t, 6006791, got)
	require.NotEqual(t, 6006790, got)
}

func TestCalcOpenRouterCacheCreateTokensRatioOneReturnsZero(t *testing.T) {
	got := CalcOpenRouterCacheCreateTokens(dto.Usage{Cost: 1.23}, hosttypes.PriceData{
		ModelRatio:         0.1,
		CacheCreationRatio: 1,
		CacheRatio:         0.1,
		CompletionRatio:    1,
	})
	require.Equal(t, 0, got)
}

func TestCalcOpenRouterCacheCreateTokensNaNReturnsSentinel(t *testing.T) {
	got := CalcOpenRouterCacheCreateTokens(dto.Usage{Cost: math.NaN()}, hosttypes.PriceData{
		ModelRatio:         1,
		CacheCreationRatio: 2,
		CacheRatio:         1,
		CompletionRatio:    1,
	})
	require.Equal(t, -1, got)
}

func TestCalculateTextQuotaSummaryOpenRouterCacheCreateInexactPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	const (
		modelRatio         = 1.5
		completionRatio    = 5.0
		cacheRatio         = 0.1
		cacheCreationRatio = 1.25
		promptTokens       = 1000
		cacheReadTokens    = 100
		cacheCreateTokens  = 200
		completionTokens   = 50
	)

	relayInfo := &relaycommon.RelayInfo{
		FinalRequestRelayFormat: types.RelayFormatClaude,
		OriginModelName:         "claude-3-5-sonnet-20241022",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenRouter,
		},
		PriceData: hosttypes.PriceData{
			ModelRatio:         modelRatio,
			CompletionRatio:    completionRatio,
			CacheRatio:         cacheRatio,
			CacheCreationRatio: cacheCreationRatio,
			GroupRatioInfo:     hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
		StartTime: time.Now(),
	}
	usage := &dto.Usage{
		Cost:             openRouterCacheCreateCost(modelRatio, cacheCreationRatio, cacheRatio, completionRatio, promptTokens, cacheReadTokens, cacheCreateTokens, completionTokens),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: cacheReadTokens,
		},
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)
	require.True(t, summary.IsClaudeUsageSemantic)
	require.Equal(t, cacheCreateTokens, summary.CacheCreationTokens)
	require.Equal(t, promptTokens-cacheReadTokens-cacheCreateTokens, summary.PromptTokens)
	require.Equal(t, cacheReadTokens, summary.CacheTokens)

	// Settlement input is the inverted integer token count, billed in decimal.
	// (700 + 100*0.1 + 200*1.25 + 50*5) * 1.5 = 1815
	require.Equal(t, 1815, summary.Quota)
}
