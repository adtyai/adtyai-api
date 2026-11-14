package ratio_setting

import (
	"maps"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hostreasoning "github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

// from songquanpeng/one-api
const (
	USD2RMB = 7.3 // 暂定 1 USD = 7.3 RMB
	USD     = 500 // $0.002 = 1 -> $1 = 500
	RMB     = USD / USD2RMB
)

// modelRatio
// https://platform.openai.com/docs/models/model-endpoint-compatibility
// https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9dlf
// https://openai.com/pricing
// TODO: when a new api is enabled, check the pricing here
// 1 === $0.002 / 1K tokens
// 1 === ￥0.014 / 1k tokens

var defaultModelRatio = map[string]float64{
	"gpt-4o":                              1.25, // $2.5 / 1M tokens
	"gpt-4o-2024-08-06":                   1.25, // $2.5 / 1M tokens
	"gpt-4o-2024-11-20":                   1.25, // $2.5 / 1M tokens
	"gpt-4.1":                             1.0,  // $2 / 1M tokens
	"gpt-4.1-2025-04-14":                  1.0,  // $2 / 1M tokens
	"gpt-4.1-mini":                        0.2,  // $0.4 / 1M tokens
	"gpt-4.1-mini-2025-04-14":             0.2,  // $0.4 / 1M tokens
	"o3":                                  1.0,  // $2 / 1M tokens
	"o3-2025-04-16":                       1.0,  // $2 / 1M tokens
	"o3-pro":                              10.0, // $20 / 1M tokens
	"o3-pro-2025-06-10":                   10.0, // $20 / 1M tokens
	"gpt-4o-mini":                         0.075,
	"gpt-4o-mini-2024-07-18":              0.075,
	"gpt-5":                               0.625,
	"gpt-5-2025-08-07":                    0.625,
	"gpt-5-mini":                          0.125,
	"gpt-5-mini-2025-08-07":               0.125,
	"gpt-5-nano":                          0.025,
	"gpt-5-nano-2025-08-07":               0.025,
	"gpt-5.5":                             2.5, // $5 / 1M tokens
	"gpt-5.6-sol":                         2.5,
	"gpt-5.6-terra":                       1.25,
	"gpt-5.6-luna":                        0.5,
	"whisper-1":                           15,  // $0.006 / minute -> $0.006 / 150 words -> $0.006 / 200 tokens -> $0.03 / 1k tokens
	"tts-1":                               7.5, // 1k characters -> $0.015
	"tts-1-hd":                            15,  // 1k characters -> $0.03
	"text-embedding-3-small":              0.01,
	"text-embedding-3-large":              0.065,
	"text-embedding-ada-002":              0.05,
	"claude-fable-5-2":                    4.0, // $8 / 1M tokens
	"claude-opus-5-5":                     2.0, // $4 / 1M tokens
	"claude-sonnet-5-5":                   1.0, // $2 / 1M tokens
	"claude-opus-5":                       2.5, // $5 / 1M tokens
	"claude-sonnet-5":                     1.0, // $2 / 1M tokens
	"claude-sonnet-4-6":                   1.5, // $3 / 1M tokens
	"claude-haiku-4-5-20251001":           0.5, // $1 / 1M tokens
	"claude-sonnet-4-5-20250929":          1.5,
	"claude-opus-4-5-20251101":            2.5,
	"claude-opus-4-6":                     2.5,
	"claude-opus-4-7":                     2.5,
	"claude-opus-4-8":                     2.5,
	"ERNIE-3.5-8K-0205":                   0.024 * RMB,
	"ERNIE-Bot-8K":                        0.024 * RMB,
	"ernie-light-dt-1.0":                  0.008 * RMB, // ￥0.008 / 1k tokens
	"ERNIE-Lite-8K-0308":                  0.003 * RMB,
	"Embedding-V1":                        0.002 * RMB,
	"bge-large-zh":                        0.002 * RMB,
	"bge-large-en":                        0.002 * RMB,
	"tao-8k":                              0.002 * RMB,
	"gemini-4-hydrogen":                   0.75,  // $1.50 / 1M tokens (paid, 2026-11-15)
	"gemini-4-carbon":                     1.0,   // $2 / 1M tokens intro through 2026-11-30
	"gemini-4-argon":                      1.0,   // $2 / 1M tokens intro through 2026-11-30
	"gemini-3.8-flash":                    0.375, // $0.75 / 1M tokens intro through 2026-12-31
	"gemini-3.6-flash":                    0.375, // $0.75 / 1M tokens intro through 2026-12-31
	"gemini-3.5-flash-lite":               0.15,  // $0.30 / 1M tokens
	"gemini-robotics-er-2-preview":        0.5,   // $1 / 1M tokens intro through 2026-12-31
	"gemini-2.5-pro-exp-03-25":            0.625,
	"gemini-2.5-pro":                      0.625,
	"gemini-2.5-flash-preview-04-17":      0.075,
	"gemini-2.5-flash-lite-preview-06-17": 0.05,
	"gemini-2.5-flash":                    0.15,
	"gemini-embedding-001":                0.075,
	"text-embedding-004":                  0.001,
	"chatglm_turbo":                       0.3572,       // ￥0.005 / 1k tokens
	"chatglm_pro":                         0.7143,       // ￥0.01 / 1k tokens
	"chatglm_std":                         0.3572,       // ￥0.005 / 1k tokens
	"chatglm_lite":                        0.1429,       // ￥0.002 / 1k tokens
	"glm-5.3":                             0.008 * RMB,  // ￥8 / 1M tokens
	"glm-5.3-flash":                       0.0008 * RMB, // ￥0.8 / 1M tokens
	"glm-5.3-flashx":                      0.002 * RMB,  // ￥2 / 1M tokens
	"glm-5.2":                             0.008 * RMB,  // ￥8 / 1M tokens
	"glm-4":                               7.143,        // ￥0.1 / 1k tokens
	"glm-4v":                              0.05 * RMB,   // ￥0.05 / 1k tokens
	"glm-4-alltools":                      0.1 * RMB,    // ￥0.1 / 1k tokens
	"glm-3-turbo":                         0.3572,
	"glm-4-plus":                          0.05 * RMB,
	"glm-4-0520":                          0.1 * RMB,
	"glm-4-air":                           0.001 * RMB,
	"glm-4-airx":                          0.01 * RMB,
	"glm-4-long":                          0.001 * RMB,
	"glm-4-flash":                         0,
	"glm-4v-plus":                         0.01 * RMB,
	"qwen-turbo":                          0.8572,         // ￥0.012 / 1k tokens
	"qwen-plus":                           10,             // ￥0.14 / 1k tokens
	"qwen3.8-flash":                       0.0008 * RMB,   // ￥0.8 / 1M tokens (Beijing list)
	"qwen3.7-text-embedding":              0.0005 * RMB,   // ￥0.5 / 1M tokens (Beijing list)
	"qwen3.7-text-embedding-flash":        0.000125 * RMB, // ￥0.125 / 1M tokens (Beijing list)
	"hy4":                                 0.006 * RMB,    // ￥6 / 1M tokens catalog
	"hy4-preview":                         0.006 * RMB,    // ￥6 / 1M tokens catalog
	"hy3":                                 0.001 * RMB,    // ￥1 / 1M tokens catalog
	"text-embedding-v1":                   0.05,           // ￥0.0007 / 1k tokens
	// https://platform.lingyiwanwu.com/docs#-计费单元
	// 已经按照 7.2 来换算美元价格
	"yi-34b-chat-0205":       0.18,
	"yi-34b-chat-200k":       0.864,
	"yi-vl-plus":             0.432,
	"yi-large":               20.0 / 1000 * RMB,
	"yi-medium":              2.5 / 1000 * RMB,
	"yi-vision":              6.0 / 1000 * RMB,
	"yi-medium-200k":         12.0 / 1000 * RMB,
	"yi-spark":               1.0 / 1000 * RMB,
	"yi-large-rag":           25.0 / 1000 * RMB,
	"yi-large-turbo":         12.0 / 1000 * RMB,
	"yi-large-preview":       20.0 / 1000 * RMB,
	"yi-large-rag-preview":   25.0 / 1000 * RMB,
	"command":                0.5,
	"command-nightly":        0.5,
	"command-light":          0.5,
	"command-light-nightly":  0.5,
	"command-r":              0.25,
	"command-r-plus":         1.5,
	"command-r-08-2024":      0.075,
	"command-r-plus-08-2024": 1.25,
	// Perplexity online 模型对搜索额外收费，有需要应自行调整，此处不计入搜索费用
	// grok
	"grok-3-beta":           1.5,
	"grok-3-mini-beta":      0.15,
	"grok-2":                1,
	"grok-2-vision":         1,
	"grok-beta":             2.5,
	"grok-vision-beta":      2.5,
	"grok-3-fast-beta":      2.5,
	"grok-3-mini-fast-beta": 0.3,
	// submodel
	"NousResearch/Hermes-4-405B-FP8": 0.8,
}

var defaultModelPrice = map[string]float64{
	"gpt-4o-mini-tts":        0.3,
	"grok-imagine-image-2.0": 0.04, // $0.04 / image
}

var defaultAudioRatio = map[string]float64{
	"gpt-4o-mini-tts": 25,
}

var defaultAudioCompletionRatio = map[string]float64{
	"gpt-4o-realtime":      2,
	"gpt-4o-mini-realtime": 2,
	"gpt-4o-mini-tts":      1,
	"tts-1":                0,
	"tts-1-hd":             0,
}

var modelPriceMap = types.NewRWMap[string, float64]()
var modelPriceDecimals = types.NewRWMap[string, decimal.Decimal]()
var modelPriceStore = newRatioJSONMap(modelPriceMap, modelPriceDecimals)

var modelRatioMap = types.NewRWMap[string, float64]()
var modelRatioDecimals = types.NewRWMap[string, decimal.Decimal]()
var modelRatioStore = newRatioJSONMap(modelRatioMap, modelRatioDecimals)

var completionRatioMap = types.NewRWMap[string, float64]()
var completionRatioDecimals = types.NewRWMap[string, decimal.Decimal]()
var completionRatioStore = newRatioJSONMap(completionRatioMap, completionRatioDecimals)

var defaultCompletionRatio = map[string]float64{
	// Official output/input unit-price ratios. Required so new models do not
	// inherit the gpt-4 completion=2 fallback or the generic gemini=4 fallback.
	"claude-fable-5-2":             5,             // $40 / $8
	"claude-opus-5-5":              5,             // $20 / $4
	"claude-sonnet-5-5":            5,             // $10 / $2
	"claude-haiku-5-5":             5,             // $0.50 / $0.10 and $2.50 / $0.50
	"claude-opus-5":                5,             // $25 / $5
	"claude-sonnet-5":              5,             // $10 / $2
	"claude-sonnet-4-6":            5,             // $15 / $3
	"gemini-4-hydrogen":            5,             // $7.50 / $1.50
	"gemini-4-carbon":              5,             // $10 / $2 (intro)
	"gemini-4-argon":               5,             // $10 / $2 (intro)
	"gemini-3.8-flash":             5,             // $3.75 / $0.75 (intro)
	"gemini-3.6-flash":             5,             // $3.75 / $0.75 (intro)
	"gemini-3.5-flash-lite":        2.5 / 0.3,     // $2.50 / $0.30
	"gemini-robotics-er-2-preview": 5,             // $5 / $1 (intro)
	"ernie-5.1":                    0.018 / 0.004, // ￥0.018 / ￥0.004 per 1k (<=32k)
	"ernie-light-dt-1.0":           3.5,           // ￥0.028 / ￥0.008 per 1k
	"glm-5.3":                      3.5,           // ￥28 / ￥8 per 1M
	"glm-5.3-flash":                3.5,           // ￥2.8 / ￥0.8 per 1M
	"glm-5.3-flashx":               3.5,           // ￥7 / ￥2 per 1M
	"glm-5.2":                      3.5,           // ￥28 / ￥8 per 1M
	"qwen3.8-flash":                2.7 / 0.8,     // ￥2.7 / ￥0.8 per 1M
	"qwen3.7-plus":                 4,             // ￥8 / ￥2 and ￥24 / ￥6
	"hy4":                          3,             // ￥18 / ￥6 per 1M
	"hy4-preview":                  3,             // ￥18 / ￥6 per 1M
	"hy3":                          4,             // ￥4 / ￥1 per 1M
}

// InitRatioSettings initializes all model related settings maps
func InitRatioSettings() {
	modelPriceMap.AddAll(defaultModelPrice)
	modelRatioMap.AddAll(defaultModelRatio)
	completionRatioMap.AddAll(defaultCompletionRatio)
	cacheRatioMap.AddAll(defaultCacheRatio)
	createCacheRatioMap.AddAll(defaultCreateCacheRatio)
	imageRatioMap.AddAll(defaultImageRatio)
	audioRatioMap.AddAll(defaultAudioRatio)
	audioCompletionRatioMap.AddAll(defaultAudioCompletionRatio)
	modelPriceStore.syncFromFloats()
	modelRatioStore.syncFromFloats()
	completionRatioStore.syncFromFloats()
	audioRatioStore.syncFromFloats()
	audioCompletionRatioStore.syncFromFloats()
}

func GetModelPriceMap() map[string]float64 {
	return modelPriceMap.ReadAll()
}

func ModelPrice2JSONString() string {
	return modelPriceMap.MarshalJSONString()
}

func UpdateModelPriceByJSONString(jsonStr string) error {
	if err := modelPriceStore.loadJSON(jsonStr); err != nil {
		return err
	}
	InvalidateExposedDataCache()
	return nil
}

// GetModelPrice 返回模型的价格，如果模型不存在则返回-1，false
func GetModelPrice(name string, printErr bool) (float64, bool) {
	name = FormatMatchingModelName(name)

	if price, ok := modelPriceMap.Get(name); ok {
		return price, true
	}

	if printErr {
		common.SysError(common.LogText("model price not found: %s", name))
	}
	return -1, false
}

func UpdateModelRatioByJSONString(jsonStr string) error {
	if err := modelRatioStore.loadJSON(jsonStr); err != nil {
		return err
	}
	InvalidateExposedDataCache()
	return nil
}

// 处理带有思考预算的模型名称，方便统一定价
func handleThinkingBudgetModel(name, prefix, wildcard string) string {
	if strings.HasPrefix(name, prefix) && strings.Contains(name, "-thinking-") {
		return wildcard
	}
	return name
}

func GetModelRatio(name string) (float64, bool, string) {
	name = FormatMatchingModelName(name)

	ratio, ok := modelRatioMap.Get(name)
	if !ok {
		return 37.5, operation_setting.SelfUseModeEnabled, name
	}
	return ratio, true, name
}

func DefaultModelRatio2JSONString() string {
	jsonBytes, err := common.Marshal(defaultModelRatio)
	if err != nil {
		common.SysError(common.LogText("error marshalling model ratio: %s", err.Error()))
	}
	return string(jsonBytes)
}

func GetDefaultModelRatioMap() map[string]float64 {
	return defaultModelRatio
}

func GetDefaultModelPriceMap() map[string]float64 {
	return defaultModelPrice
}

// GetDefaultPricingMaps returns independent copies for model-level reset and
// first-write initialization; callers cannot mutate the built-in defaults.
func GetDefaultPricingMaps() map[string]map[string]float64 {
	defaults := map[string]map[string]float64{
		"ModelPrice": defaultModelPrice, "ModelRatio": defaultModelRatio,
		"CompletionRatio": defaultCompletionRatio, "CacheRatio": defaultCacheRatio,
		"CreateCacheRatio": defaultCreateCacheRatio, "ImageRatio": defaultImageRatio,
		"AudioRatio": defaultAudioRatio, "AudioCompletionRatio": defaultAudioCompletionRatio,
	}
	result := make(map[string]map[string]float64, len(defaults))
	for key, values := range defaults {
		result[key] = make(map[string]float64, len(values))
		maps.Copy(result[key], values)
	}
	return result
}

func CompletionRatio2JSONString() string {
	return completionRatioMap.MarshalJSONString()
}

func UpdateCompletionRatioByJSONString(jsonStr string) error {
	if err := completionRatioStore.loadJSON(jsonStr); err != nil {
		return err
	}
	InvalidateExposedDataCache()
	return nil
}

func GetCompletionRatio(name string) float64 {
	return GetCompletionRatioInfo(name).Ratio
}

type CompletionRatioInfo struct {
	Ratio  float64 `json:"ratio"`
	Locked bool    `json:"locked"`
}

func GetCompletionRatioInfo(name string) CompletionRatioInfo {
	name = FormatMatchingModelName(name)
	var configured *float64
	if ratio, ok := completionRatioMap.Get(name); ok {
		configured = &ratio
	}
	return ResolveCompletionRatio(name, configured)
}

// ResolveCompletionRatio applies relay's enforced and fallback ratios to a
// configuration snapshot or draft without consulting mutable saved settings.
func ResolveCompletionRatio(name string, configured *float64) CompletionRatioInfo {
	name = FormatMatchingModelName(name)
	if strings.Contains(name, "/") && configured != nil {
		return CompletionRatioInfo{Ratio: *configured}
	}

	hardCodedRatio, locked := getHardcodedCompletionModelRatio(name)
	if locked {
		return CompletionRatioInfo{
			Ratio:  hardCodedRatio,
			Locked: true,
		}
	}

	if configured != nil {
		return CompletionRatioInfo{
			Ratio:  *configured,
			Locked: false,
		}
	}

	return CompletionRatioInfo{
		Ratio:  hardCodedRatio,
		Locked: false,
	}
}

func getHardcodedCompletionModelRatio(name string) (float64, bool) {

	isReservedModel := strings.HasSuffix(name, "-all") || strings.HasSuffix(name, "-gizmo-*")
	if isReservedModel {
		return 2, false
	}

	if strings.HasPrefix(name, "gpt-") {
		if strings.HasPrefix(name, "gpt-4o") {
			if name == "gpt-4o-2024-05-13" {
				return 3, true
			}
			if strings.HasPrefix(name, "gpt-4o-mini-tts") {
				return 20, false
			}
			return 4, false
		}
		// gpt-5 匹配
		if strings.HasPrefix(name, "gpt-5") {
			if !strings.Contains(name, ".") {
				return 8, true
			}
			if strings.HasPrefix(name, "gpt-5.4") {
				if strings.HasPrefix(name, "gpt-5.4-nano") {
					return 6.25, true
				}
				return 6, true
			}
			// gpt-5.5 and later models are unlocked
			return 6, false
		}
		// gpt-4.5-preview匹配
		if strings.HasPrefix(name, "gpt-4.5-preview") {
			return 2, true
		}
		if strings.HasPrefix(name, "gpt-4-turbo") || strings.HasSuffix(name, "gpt-4-1106") || strings.HasSuffix(name, "gpt-4-1105") {
			return 3, true
		}
		// 没有特殊标记的 gpt-4 模型默认倍率为 2
		return 2, false
	}
	if strings.HasPrefix(name, "o1") || strings.HasPrefix(name, "o3") {
		return 4, true
	}
	if name == "chatgpt-4o-latest" {
		return 3, true
	}

	if strings.Contains(name, "claude-3") {
		return 5, true
	} else if strings.Contains(name, "claude-sonnet-4") || strings.Contains(name, "claude-opus-4") || strings.Contains(name, "claude-haiku-4") {
		return 5, true
	}

	if strings.HasPrefix(name, "gpt-3.5") {
		if name == "gpt-3.5-turbo" || strings.HasSuffix(name, "0125") {
			// https://openai.com/blog/new-embedding-models-and-api-updates
			// Updated GPT-3.5 Turbo model and lower pricing
			return 3, true
		}
		if strings.HasSuffix(name, "1106") {
			return 2, true
		}
		return 4.0 / 3.0, true
	}
	if strings.HasPrefix(name, "mistral-") {
		return 3, true
	}
	if strings.HasPrefix(name, "gemini-") {
		if strings.HasPrefix(name, "gemini-1.5") {
			return 4, true
		} else if strings.HasPrefix(name, "gemini-2.0") {
			return 4, true
		} else if strings.HasPrefix(name, "gemini-2.5-pro") { // 移除preview来增加兼容性，这里假设正式版的倍率和preview一致
			return 8, false
		} else if strings.HasPrefix(name, "gemini-2.5-flash") { // 处理不同的flash模型倍率
			if strings.HasPrefix(name, "gemini-2.5-flash-preview") {
				return 3.5 / 0.15, false
			}
			if strings.HasPrefix(name, "gemini-2.5-flash-lite") {
				return 4, false
			}
			return 2.5 / 0.3, false
		} else if strings.HasPrefix(name, "gemini-robotics-er-1.5") {
			return 2.5 / 0.3, false
		} else if strings.HasPrefix(name, "gemini-3-pro") {
			if strings.HasPrefix(name, "gemini-3-pro-image") {
				return 60, false
			}
			return 6, false
		}
		return 4, false
	}
	if strings.HasPrefix(name, "command") {
		switch name {
		case "command-r":
			return 3, true
		case "command-r-plus":
			return 5, true
		case "command-r-08-2024":
			return 4, true
		case "command-r-plus-08-2024":
			return 4, true
		default:
			return 4, false
		}
	}
	// hint 只给官方上4倍率，由于开源模型供应商自行定价，不对其进行补全倍率进行强制对齐
	if strings.HasPrefix(name, "ERNIE-Speed-") {
		return 2, true
	} else if strings.HasPrefix(name, "ERNIE-Lite-") {
		return 2, true
	} else if strings.HasPrefix(name, "ERNIE-Character") {
		return 2, true
	} else if strings.HasPrefix(name, "ERNIE-Functions") {
		return 2, true
	}
	switch name {
	case "llama2-70b-4096":
		return 0.8 / 0.64, true
	case "llama3-8b-8192":
		return 2, true
	case "llama3-70b-8192":
		return 0.79 / 0.59, true
	}
	return 1, false
}

func GetAudioRatio(name string) float64 {
	name = FormatMatchingModelName(name)
	if ratio, ok := audioRatioMap.Get(name); ok {
		return ratio
	}
	return 1
}

func GetAudioCompletionRatio(name string) float64 {
	name = FormatMatchingModelName(name)
	if ratio, ok := audioCompletionRatioMap.Get(name); ok {
		return ratio
	}
	return 1
}

func ContainsAudioRatio(name string) bool {
	name = FormatMatchingModelName(name)
	_, ok := audioRatioMap.Get(name)
	return ok
}

func ContainsAudioCompletionRatio(name string) bool {
	name = FormatMatchingModelName(name)
	_, ok := audioCompletionRatioMap.Get(name)
	return ok
}

func ModelRatio2JSONString() string {
	return modelRatioMap.MarshalJSONString()
}

var defaultImageRatio = map[string]float64{}
var imageRatioMap = types.NewRWMap[string, float64]()
var audioRatioMap = types.NewRWMap[string, float64]()
var audioRatioDecimals = types.NewRWMap[string, decimal.Decimal]()
var audioRatioStore = newRatioJSONMap(audioRatioMap, audioRatioDecimals)
var audioCompletionRatioMap = types.NewRWMap[string, float64]()
var audioCompletionRatioDecimals = types.NewRWMap[string, decimal.Decimal]()
var audioCompletionRatioStore = newRatioJSONMap(audioCompletionRatioMap, audioCompletionRatioDecimals)

func ImageRatio2JSONString() string {
	return imageRatioMap.MarshalJSONString()
}

func UpdateImageRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(imageRatioMap, jsonStr)
}

const DefaultImageRatio = 1.0

func GetImageRatio(name string) (float64, bool) {
	ratio, ok := imageRatioMap.Get(name)
	if !ok {
		return DefaultImageRatio, false
	}
	return ratio, true
}

func AudioRatio2JSONString() string {
	return audioRatioMap.MarshalJSONString()
}

func UpdateAudioRatioByJSONString(jsonStr string) error {
	if err := audioRatioStore.loadJSON(jsonStr); err != nil {
		return err
	}
	InvalidateExposedDataCache()
	return nil
}

func AudioCompletionRatio2JSONString() string {
	return audioCompletionRatioMap.MarshalJSONString()
}

func UpdateAudioCompletionRatioByJSONString(jsonStr string) error {
	if err := audioCompletionRatioStore.loadJSON(jsonStr); err != nil {
		return err
	}
	InvalidateExposedDataCache()
	return nil
}

func GetModelRatioCopy() map[string]float64 {
	return modelRatioMap.ReadAll()
}

func GetModelPriceCopy() map[string]float64 {
	return modelPriceMap.ReadAll()
}

func GetCompletionRatioCopy() map[string]float64 {
	return completionRatioMap.ReadAll()
}

func GetImageRatioCopy() map[string]float64 {
	return imageRatioMap.ReadAll()
}

func GetAudioRatioCopy() map[string]float64 {
	return audioRatioMap.ReadAll()
}

func GetAudioCompletionRatioCopy() map[string]float64 {
	return audioCompletionRatioMap.ReadAll()
}

// RoutingMatchModelName returns the name used for channel-ability and token-limit
// fallback matching: strip @ modifiers and legacy aliases first, then apply
// wildcard normalization.
func RoutingMatchModelName(name string) string {
	return FormatMatchingModelName(hostreasoning.BaseModelName(name))
}

// HasConfiguredModelRatio reports whether name has an explicit ratio entry
// after wildcard normalization. Self-use fallback does not count.
func HasConfiguredModelRatio(name string) bool {
	name = FormatMatchingModelName(name)
	_, ok := modelRatioMap.Get(name)
	return ok
}

// 转换模型名，减少渠道必须配置各种带参数模型
func FormatMatchingModelName(name string) string {
	if strings.HasPrefix(name, "gemini-2.5-flash-lite") {
		name = handleThinkingBudgetModel(name, "gemini-2.5-flash-lite", "gemini-2.5-flash-lite-thinking-*")
	} else if strings.HasPrefix(name, "gemini-2.5-flash") {
		name = handleThinkingBudgetModel(name, "gemini-2.5-flash", "gemini-2.5-flash-thinking-*")
	} else if strings.HasPrefix(name, "gemini-2.5-pro") {
		name = handleThinkingBudgetModel(name, "gemini-2.5-pro", "gemini-2.5-pro-thinking-*")
	}

	if strings.HasPrefix(name, "gpt-4-gizmo") {
		name = "gpt-4-gizmo-*"
	}
	if strings.HasPrefix(name, "gpt-4o-gizmo") {
		name = "gpt-4o-gizmo-*"
	}
	return name
}

// result: 倍率or价格， usePrice， exist
func GetModelRatioOrPrice(model string) (float64, bool, bool) { // price or ratio
	price, usePrice := GetModelPrice(model, false)
	if usePrice {
		return price, true, true
	}
	modelRatio, success, _ := GetModelRatio(model)
	if success {
		return modelRatio, false, true
	}
	return 37.5, false, false
}
