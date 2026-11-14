package ratio_setting

import (
	"github.com/QuantumNous/new-api/types"
)

const (
	DefaultCacheRatio       = 1.0
	DefaultCreateCacheRatio = 1.25
)

var defaultCacheRatio = map[string]float64{
	"gpt-6.1-sol":                         0.05,       // $0.10 / $2
	"gpt-6.1-terra":                       0.05,       // $0.05 / $1
	"gpt-6-sol":                           0.10,       // $0.20 / $2
	"gpt-6-luna":                          0.10,       // $0.01 / $0.10
	"claude-fable-5-2":                    0.025,      // $0.20 / $8
	"claude-opus-5-5":                     0.05,       // $0.20 / $4
	"claude-sonnet-5-5":                   0.05,       // $0.10 / $2
	"claude-haiku-5-5":                    0.10,       // $0.01 / $0.10 and $0.05 / $0.50
	"claude-opus-5":                       0.10,       // $0.50 / $5
	"claude-sonnet-5":                     0.10,       // $0.20 / $2
	"claude-sonnet-4-6":                   0.10,       // $0.30 / $3
	"gemini-4-hydrogen":                   0.05,       // $0.075 / $1.50
	"gemini-4-carbon":                     0.05,       // $0.10 / $2 (intro)
	"gemini-4-argon":                      0.05,       // $0.10 / $2 (intro)
	"gemini-3.8-flash":                    0.10,       // $0.075 / $0.75 (intro)
	"gemini-3.6-flash":                    0.10,       // $0.075 / $0.75 (intro)
	"gemini-3.5-flash-lite":               0.10,       // $0.03 / $0.30
	"gemini-robotics-er-2-preview":        0.10,       // $0.10 / $1 (intro)
	"ernie-5.5-flash-preview":             0.10,       // 原价 cache/input, peak and off-peak
	"ernie-light-dt-1.0":                  0.10,       // ￥0.0008 / ￥0.008 per 1k
	"glm-5.3":                             0.25,       // ￥2 / ￥8
	"glm-5.3-flash":                       0.23 / 0.8, // ￥0.23 / ￥0.8
	"glm-5.3-flashx":                      0.57 / 2,   // ￥0.57 / ￥2
	"glm-5.2":                             0.25,       // ￥2 / ￥8
	"qwen3.8-flash":                       0.10,       // official cache hit 10% of input
	"qwen3.7-plus":                        0.10,
	"grok-4.7":                            0.25, // $0.50 / $2
	"grok-4.6":                            0.25, // $0.50 / $2
	"grok-4.5":                            0.15, // $0.30 / $2
	"grok-4.3":                            0.16, // $0.20 / $1.25
	"grok-build-0.1":                      0.20, // $0.20 / $1
	"hy4":                                 0.05, // ￥0.3 / ￥6
	"hy4-preview":                         0.05, // ￥0.3 / ￥6
	"hy3":                                 0.25, // ￥0.25 / ￥1
	"gemini-3-flash-preview":              0.1,
	"gemini-3-pro-preview":                0.1,
	"gemini-3.1-pro-preview":              0.1,
	"gpt-4o-2024-11-20":                   0.5,
	"gpt-4o-2024-08-06":                   0.5,
	"gpt-4o":                              0.5,
	"gpt-4o-mini-2024-07-18":              0.5,
	"gpt-4o-mini":                         0.5,
	"gpt-4.1":                             0.25,
	"gpt-4.1-mini":                        0.25,
	"gpt-5":                               0.1,
	"gpt-5-2025-08-07":                    0.1,
	"gpt-5-mini":                          0.1,
	"gpt-5-mini-2025-08-07":               0.1,
	"gpt-5-nano":                          0.1,
	"gpt-5-nano-2025-08-07":               0.1,
	"claude-haiku-4-5-20251001":           0.1,
	"claude-sonnet-4-20250514-thinking":   0.1,
	"claude-opus-4-20250514-thinking":     0.1,
	"claude-opus-4-1-20250805-thinking":   0.1,
	"claude-sonnet-4-5-20250929":          0.1,
	"claude-sonnet-4-5-20250929-thinking": 0.1,
	"claude-opus-4-5-20251101":            0.1,
	"claude-opus-4-5-20251101-thinking":   0.1,
	"claude-opus-4-6":                     0.1,
	"claude-opus-4-6-thinking":            0.1,
	"claude-opus-4-7":                     0.1,
	"claude-opus-4-7-thinking":            0.1,
	"claude-opus-4-8":                     0.1,
	"claude-opus-4-8-thinking":            0.1,
}

var defaultCreateCacheRatio = map[string]float64{
	"gpt-6.1-sol":                         1.25, // 30m write $2.50 / $2
	"gpt-6.1-terra":                       1.25, // 30m write $1.25 / $1
	"gpt-6-sol":                           1.25, // 30m write $2.50 / $2
	"gpt-6-luna":                          1.25, // 30m write $0.125 / $0.10
	"claude-fable-5-2":                    1.25, // 5m write $10 / $8
	"claude-opus-5-5":                     1.25, // 5m write $5 / $4
	"claude-sonnet-5-5":                   1.25, // 5m write $2.50 / $2
	"claude-haiku-5-5":                    1.25, // 5m write $0.125 / $0.10
	"claude-opus-5":                       1.25, // 5m write $6.25 / $5
	"claude-sonnet-5":                     1.25, // 5m write $2.50 / $2
	"claude-sonnet-4-6":                   1.25, // 5m write $3.75 / $3
	"qwen3.8-flash":                       1.25, // official explicit cache create 125% of input
	"qwen3.7-plus":                        1.25,
	"gpt-5.6-sol":                         1.25,
	"gpt-5.6-terra":                       1.25,
	"gpt-5.6-luna":                        1.25,
	"claude-haiku-4-5-20251001":           1.25,
	"claude-sonnet-4-20250514-thinking":   1.25,
	"claude-opus-4-20250514-thinking":     1.25,
	"claude-opus-4-1-20250805-thinking":   1.25,
	"claude-sonnet-4-5-20250929":          1.25,
	"claude-sonnet-4-5-20250929-thinking": 1.25,
	"claude-opus-4-5-20251101":            1.25,
	"claude-opus-4-5-20251101-thinking":   1.25,
	"claude-opus-4-6":                     1.25,
	"claude-opus-4-6-thinking":            1.25,
	"claude-opus-4-7":                     1.25,
	"claude-opus-4-7-thinking":            1.25,
	"claude-opus-4-8":                     1.25,
	"claude-opus-4-8-thinking":            1.25,
}

//var defaultCreateCacheRatio = map[string]float64{}

var cacheRatioMap = types.NewRWMap[string, float64]()
var createCacheRatioMap = types.NewRWMap[string, float64]()

// GetCacheRatioMap returns a copy of the cache ratio map
func GetCacheRatioMap() map[string]float64 {
	return cacheRatioMap.ReadAll()
}

// CacheRatio2JSONString converts the cache ratio map to a JSON string
func CacheRatio2JSONString() string {
	return cacheRatioMap.MarshalJSONString()
}

// CreateCacheRatio2JSONString converts the create cache ratio map to a JSON string
func CreateCacheRatio2JSONString() string {
	return createCacheRatioMap.MarshalJSONString()
}

// UpdateCacheRatioByJSONString updates the cache ratio map from a JSON string
func UpdateCacheRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(cacheRatioMap, jsonStr, InvalidateExposedDataCache)
}

// UpdateCreateCacheRatioByJSONString updates the create cache ratio map from a JSON string
func UpdateCreateCacheRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(createCacheRatioMap, jsonStr, InvalidateExposedDataCache)
}

// GetCacheRatio returns the cache ratio for a model
func GetCacheRatio(name string) (float64, bool) {
	ratio, ok := cacheRatioMap.Get(name)
	if !ok {
		return DefaultCacheRatio, false
	}
	return ratio, true
}

func GetCreateCacheRatio(name string) (float64, bool) {
	ratio, ok := createCacheRatioMap.Get(name)
	if !ok {
		return DefaultCreateCacheRatio, false
	}
	return ratio, true
}

func GetCacheRatioCopy() map[string]float64 {
	return cacheRatioMap.ReadAll()
}

func GetCreateCacheRatioCopy() map[string]float64 {
	return createCacheRatioMap.ReadAll()
}
