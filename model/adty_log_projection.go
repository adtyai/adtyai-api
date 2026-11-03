package model

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// 客户看到的日志投影（任务 7.1）：清掉渠道和上游请求 ID，other 只留用量类字段（白名单），
// 倍率、价格和上游信息一律不给。按 json.RawMessage 处理，超过 JS 安全范围的大整数原样保留。

// adtyUserLogOtherKeys 是普通用户能看到的 other 键。
var adtyUserLogOtherKeys = map[string]bool{
	"cache_tokens": true, "cache_creation_tokens": true, "cache_creation_tokens_5m": true,
	"cache_creation_tokens_1h": true, "cache_creation_tokens_12h": true, "cache_write_tokens": true,
	"input_tokens_total": true, "audio_input": true, "audio_output": true, "text_input": true,
	"text_output": true, "audio_input_token_count": true, "image": true, "image_output": true,
	"image_count": true, "image_cache_tokens": true, "billing_tokens": true, "tool_surcharges": true,
	"billing_mode": true, "billing_unit": true, "billing_source": true, "usage_facts": true, "frt": true,
	"reasoning_effort": true, "request_path": true, "ws": true, "audio": true, "is_task": true,
	"task_sync": true, "task_id": true, "result_discarded": true, "reason": true,
	"pre_consumed_quota": true, "actual_quota": true, "error_type": true, "error_code": true,
	"status_code": true, "violation_fee": true, "violation_fee_code": true,
	logOtherContentPartsKey: true, "op": true, "stream_status": true,
}

// 投影时补的计费方式标记：去掉 model_price 后前端靠它区分按次和按 token。
const (
	AdtyBillingModePerCall  = "per_call"
	AdtyBillingModePerToken = "per_token"
)

// AdtyProjectUserLog 把一条日志改成客户能看的样子，列表和导出都调它。
func AdtyProjectUserLog(log *Log) {
	log.ChannelId = 0
	log.ChannelName = ""
	log.UpstreamRequestId = ""
	log.Other = adtyProjectUserLogOther(log.Other)
}

func adtyProjectUserLogOther(value string) string {
	if value == "" {
		return ""
	}
	var values map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(value, &values); err != nil {
		return "{}"
	}
	out := make(map[string]json.RawMessage, len(values))
	for key, raw := range values {
		if !adtyUserLogOtherKeys[key] {
			continue
		}
		switch key {
		case "tool_surcharges":
			raw = adtyKeepFields(raw, true, "name", "count")
		case "stream_status":
			raw = adtyKeepFields(raw, false, "status", "end_reason")
		}
		if raw != nil {
			out[key] = raw
		}
	}
	if _, ok := out["billing_mode"]; !ok {
		mode := AdtyBillingModePerToken
		var price float64
		if raw, ok := values["model_price"]; ok && common.Unmarshal(raw, &price) == nil && price > 0 {
			mode = AdtyBillingModePerCall
		}
		if encoded, err := common.Marshal(mode); err == nil {
			out["billing_mode"] = encoded
		}
	}
	formatted, err := common.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(formatted)
}

// adtyKeepFields 只保留对象（list 为真时是对象数组里每一项）的几个字段；格式不对时整项去掉。
func adtyKeepFields(raw json.RawMessage, list bool, fields ...string) json.RawMessage {
	var encoded []byte
	var err error
	if list {
		var items []map[string]json.RawMessage
		if common.Unmarshal(raw, &items) != nil {
			return nil
		}
		for i := range items {
			items[i] = adtyPickFields(items[i], fields)
		}
		encoded, err = common.Marshal(items)
	} else {
		var item map[string]json.RawMessage
		if common.Unmarshal(raw, &item) != nil {
			return nil
		}
		encoded, err = common.Marshal(adtyPickFields(item, fields))
	}
	if err != nil {
		return nil
	}
	return encoded
}

func adtyPickFields(item map[string]json.RawMessage, fields []string) map[string]json.RawMessage {
	kept := make(map[string]json.RawMessage, len(fields))
	for _, field := range fields {
		if value, ok := item[field]; ok {
			kept[field] = value
		}
	}
	return kept
}
