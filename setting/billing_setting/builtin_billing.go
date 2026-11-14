package billing_setting

import "github.com/QuantumNous/new-api/constant"

// deepSeekPeak is DeepSeek's peak window: Monday to Friday, 09:00-12:00 and
// 14:00-18:00 Beijing time. The Chinese public holidays DeepSeek bills
// off-peak are not known in advance and bill at peak here.
const deepSeekPeak = `weekday("Asia/Shanghai") >= 1 && weekday("Asia/Shanghai") <= 5 && ((hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) || (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18))`

const deepSeekFlashExpr = deepSeekPeak + ` ? tier("peak", p * 0.3 + cr * 0.006 + c * 1.2) : tier("off_peak", p * 0.15 + cr * 0.003 + c * 0.6)`

// Built-in token prices use actual USD per million tokens. Keep new model
// defaults here instead of splitting them across the legacy ratio tables.
var builtinBillingExpr = map[string]string{
	// https://developers.openai.com/api/docs/pricing (Standard, 2026-09-09).
	// The Images API reports image output in output_tokens, normalized to c.
	"gpt-image-2":            `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	"gpt-image-2.5-sunburst": `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	"gpt-image-2.5-flare":    `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	// https://developers.openai.com/api/docs/pricing (Standard, 2026-11-15).
	// Same token-family rates as gpt-image-2.5.
	"gpt-image-2.6-sunburst": `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	"gpt-image-2.6-flare":    `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	// https://developers.openai.com/api/docs/models/gpt-6-astra
	// https://developers.openai.com/api/docs/pricing (Standard, 2026-10-22):
	// cc is the 30m cache write and cc12h the 12h write. The long-context
	// rates apply to the whole request.
	// Do not infer service-tier discounts from incoming request parameters:
	// channels filter service_tier by default, so it may not reach the upstream.
	"gpt-6-astra": `len <= 272000 ? tier("standard", p * 10 + c * 50 + cr * 1 + cc * 12.5 + cc12h * 25) : tier("long_context", p * 20 + c * 75 + cr * 2 + cc * 25 + cc12h * 50)`,
	// https://developers.openai.com/api/docs/pricing (Standard short/long, 2026-11-15).
	// Prompts over 272K input tokens bill the whole request at long-context rates.
	"gpt-6.1-sol":   `len <= 272000 ? tier("standard", p * 2 + c * 10 + cr * 0.1 + cc * 2.5 + cc12h * 5) : tier("long_context", p * 4 + c * 15 + cr * 0.2 + cc * 5 + cc12h * 10)`,
	"gpt-6.1-terra": `len <= 272000 ? tier("standard", p * 1 + c * 5 + cr * 0.05 + cc * 1.25 + cc12h * 2.5) : tier("long_context", p * 2 + c * 7.5 + cr * 0.1 + cc * 2.5 + cc12h * 5)`,
	"gpt-6-sol":     `len <= 272000 ? tier("standard", p * 2 + c * 10 + cr * 0.2 + cc * 2.5 + cc12h * 5) : tier("long_context", p * 4 + c * 15 + cr * 0.4 + cc * 5 + cc12h * 10)`,
	"gpt-6-luna":    `len <= 272000 ? tier("standard", p * 0.1 + c * 0.5 + cr * 0.01 + cc * 0.125 + cc12h * 0.25) : tier("long_context", p * 0.2 + c * 0.75 + cr * 0.02 + cc * 0.25 + cc12h * 0.5)`,
	// https://developers.openai.com/api/docs/pricing (Realtime & Audio / Moderation, 2026-11-15).
	"gpt-realtime-2.1": `tier("standard", p * 4 + c * 24 + cr * 0.4 + ai * 32 + ao * 64 + img * 5 + img_cr * 0.5)`,
	"gpt-audio-1.5":    `tier("standard", p * 2.5 + c * 10 + ai * 32 + ao * 64)`,
	"omni-moderation":  `tier("free", fixed(0))`,
	// https://platform.claude.com/docs/en/about-claude/pricing (2026-11-15).
	// Haiku 5.5 has an official 100k input ladder; cache hits are 10% of the
	// matching input rate. 5m/1h cache writes stay on create-cache defaults.
	"claude-haiku-5-5": `len <= 100000 ? tier("standard", p * 0.1 + cr * 0.01 + c * 0.5) : tier("long_context", p * 0.5 + cr * 0.05 + c * 2.5)`,
	// https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya (updated 2026-11-11).
	// Catalog (原价) converted at the project's USD2RMB=7.3. Peak is 08:00-22:00
	// Beijing time every day, as the official table states.
	"ernie-5.5-flash-preview": `(hour("Asia/Shanghai") >= 8 && hour("Asia/Shanghai") < 22) ? tier("peak", p * 0.2739726027 + cr * 0.0273972603 + c * 1.095890411) : tier("off_peak", p * 0.1369863014 + cr * 0.0136986301 + c * 0.5479452055)`,
	"ernie-5.1":               `len <= 32000 ? tier("standard", p * 0.5479452055 + c * 2.4657534247) : tier("long_context", p * 0.8219178082 + c * 3.0136986301)`,
	// https://help.aliyun.com/zh/model-studio/model-pricing (Beijing list, 2026-11-15).
	// RMB/1M converted at USD2RMB=7.3. Cache hit 10%, explicit cache create 125%.
	"qwen3.7-plus": `len <= 256000 ? tier("standard", p * 0.2739726027 + c * 1.095890411 + cr * 0.0273972603 + cc * 0.3424657534) : tier("long_context", p * 0.8219178082 + c * 3.2876712329 + cr * 0.0821917808 + cc * 1.0273972603)`,
	// https://docs.x.ai/developers/models and https://docs.x.ai/developers/pricing (2026-11-15).
	// Official long-context rates apply to the whole request at >= 200k prompt tokens.
	"grok-4.7":       `len < 200000 ? tier("standard", p * 2 + c * 6 + cr * 0.5) : tier("long_context", p * 4 + c * 12 + cr * 1)`,
	"grok-4.6":       `len < 200000 ? tier("standard", p * 2 + c * 6 + cr * 0.5) : tier("long_context", p * 4 + c * 12 + cr * 1)`,
	"grok-4.5":       `len < 200000 ? tier("standard", p * 2 + c * 6 + cr * 0.3) : tier("long_context", p * 4 + c * 12 + cr * 0.6)`,
	"grok-4.3":       `len < 200000 ? tier("standard", p * 1.25 + c * 2.5 + cr * 0.2) : tier("long_context", p * 2.5 + c * 5 + cr * 0.4)`,
	"grok-build-0.1": `len < 200000 ? tier("standard", p * 1 + c * 2 + cr * 0.2) : tier("long_context", p * 2 + c * 4 + cr * 0.4)`,
	// https://api-docs.deepseek.com/quick_start/pricing (2026-10-17). Off-peak
	// is half the peak price. deepseek-v4-flash and deepseek-v4-flash-vision-exp
	// run on V4.1 Flash and bill at the Flash price. Images bill as input
	// tokens, and there is no cache-write price.
	"deepseek-flash":               deepSeekFlashExpr,
	"deepseek-v4-flash":            deepSeekFlashExpr,
	"deepseek-v4-flash-vision-exp": deepSeekFlashExpr,
	"deepseek-v4-pro":              deepSeekPeak + ` ? tier("peak", p * 1.32 + cr * 0.044 + c * 3.96) : tier("off_peak", p * 0.66 + cr * 0.022 + c * 1.98)`,
	// The Search and Fetch API routing names: $0.75 per 1,000 successful
	// requests each, the same pricing page. The calls carry no tokens.
	constant.DeepSeekSearchModel: `tier("request", fixed(0.00075))`,
	constant.DeepSeekFetchModel:  `tier("request", fixed(0.00075))`,
}
