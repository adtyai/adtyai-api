package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type businessParamPutItem struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type businessParamPutReq struct {
	Confirm bool                   `json:"confirm"`
	Key     string                 `json:"key"`
	Value   any                    `json:"value"`
	Items   []businessParamPutItem `json:"items"`
}

// AdminGetBusinessParams GET /api/admin/business-params
func AdminGetBusinessParams(c *gin.Context) {
	rows := model.ListBusinessParamsWithValues()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rows,
		"meta": gin.H{
			"count":    len(rows),
			"excluded": []string{"EpayInstanceDailyLimit"},
			"note":     "EpayInstanceDailyLimit is per-instance, not in global options. Official RateLimit* keep official paths; TopupOrderRateLimit* are adtyai-only.",
		},
	})
}

// AdminPutBusinessParams PUT /api/admin/business-params
// Body: {key, value} or {items:[{key,value},...]}（confirm 字段忽略，不再校验）
func AdminPutBusinessParams(c *gin.Context) {
	var req businessParamPutReq
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	items := req.Items
	if len(items) == 0 && req.Key != "" {
		items = []businessParamPutItem{{Key: req.Key, Value: req.Value}}
	}
	if len(items) == 0 {
		adtyFail(c, http.StatusBadRequest, "bad_request", "key/value or items required", nil)
		return
	}
	applyBusinessParamUpdates(c, items)
}

// AdminPutBusinessParamByKey PUT /api/admin/business-params/:key
func AdminPutBusinessParamByKey(c *gin.Context) {
	key := c.Param("key")
	var req struct {
		Confirm bool `json:"confirm"`
		Value   any  `json:"value"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	applyBusinessParamUpdates(c, []businessParamPutItem{{Key: key, Value: req.Value}})
}

func stringifyBPValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return common.Interface2String(t)
	case bool:
		return common.Interface2String(t)
	case int:
		return common.Interface2String(t)
	default:
		// JSON arrays/objects arrived decoded — re-marshal
		b, err := common.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

func businessParamRangeParams(key, value string) map[string]any {
	params := map[string]any{"key": key, "value": value}
	if def, ok := model.BusinessParamDefByKey(key); ok {
		if def.Min != nil {
			params["min"] = *def.Min
		}
		if def.Max != nil {
			params["max"] = *def.Max
		}
	}
	return params
}

// businessParamErrorPayload 业务参数校验错误 → code/message/params；业务参数接口和通用配置保存共用。
func businessParamErrorPayload(err error) (string, string, map[string]any) {
	var coded *model.BusinessParamCodedError
	if errors.As(err, &coded) {
		return coded.Code, coded.Message, coded.Params
	}
	var bpErr *model.BusinessParamError
	hasKey := errors.As(err, &bpErr)
	switch {
	case errors.Is(err, model.ErrUnknownBusinessParam):
		if hasKey {
			return "unknown_key", err.Error(), map[string]any{"key": bpErr.Key}
		}
		return "unknown_key", err.Error(), nil
	case errors.Is(err, model.ErrBusinessParamRange):
		if hasKey {
			return "out_of_range", "value out of range", businessParamRangeParams(bpErr.Key, bpErr.Value)
		}
		return "out_of_range", "value out of range", nil
	case errors.Is(err, model.ErrBusinessParamFormat):
		if hasKey {
			return "invalid_value", "invalid value", map[string]any{"key": bpErr.Key, "value": bpErr.Value}
		}
		return "invalid_value", "invalid value", nil
	}
	return "validation_failed", err.Error(), nil
}

func applyBusinessParamUpdates(c *gin.Context, items []businessParamPutItem) {
	normalized := make(map[string]string, len(items))
	oldValues := make(map[string]string, len(items))

	for _, it := range items {
		key := strings.TrimSpace(it.Key)
		if key == "" {
			adtyFail(c, http.StatusBadRequest, "bad_request", "empty key", nil)
			return
		}
		if !model.IsBusinessParamKey(key) {
			adtyFail(c, http.StatusBadRequest, "unknown_key", "unknown key", map[string]any{"key": key})
			return
		}
		// 审查 N-P1-4：资金类参数仅 root 可改
		if businessParamFundKeys[key] && c.GetInt("role") < common.RoleRootUser {
			adtyFail(c, http.StatusForbidden, "root_only", "fund parameter requires root", map[string]any{"key": key})
			return
		}
		raw := stringifyBPValue(it.Value)
		nv, err := model.NormalizeBusinessParamValue(key, raw)
		if err != nil {
			if errors.Is(err, model.ErrBusinessParamRange) {
				adtyFail(c, http.StatusBadRequest, "out_of_range", "value out of range", businessParamRangeParams(key, raw))
				return
			}
			adtyFail(c, http.StatusBadRequest, "invalid_value", "invalid value", map[string]any{"key": key, "value": raw})
			return
		}
		normalized[key] = nv
		oldValues[key] = model.BusinessParamCurrentValue(key)
	}

	// Atomic: validate-all + single DB tx + OptionMap only after commit.
	if err := model.ApplyBusinessParamUpdates(normalized); err != nil {
		code, msg, params := businessParamErrorPayload(err)
		adtyFail(c, http.StatusBadRequest, code, msg, params)
		return
	}

	changed := make([]gin.H, 0, len(normalized))
	for key, nv := range normalized {
		changed = append(changed, gin.H{
			"key":       key,
			"old_value": oldValues[key],
			"new_value": nv,
			"value":     model.BusinessParamCurrentValue(key),
		})
	}

	for key, nv := range normalized {
		if oldValues[key] != nv {
			common.SysLog(common.LogText("business param changed: operator=%d key=%s old=%q new=%q", c.GetInt("id"), key, oldValues[key], nv))
			model.RecordLog(c.GetInt("id"), model.LogTypeManage, common.NewMessage("Business parameter {{key}} changed from {{old}} to {{new}}",
				map[string]any{"key": key, "old": oldValues[key], "new": nv}))
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": changed})
}

// businessParamFundKeys 审查 N-P1-4：涉及资金（加赠、汇率、返佣、提现、订单超时、新用户额度、调额上限）的参数。
var businessParamFundKeys = map[string]bool{
	"TopupBonusTiers": true, "FirstTopupBonusRate": true, "Price": true,
	"BepusdtOrderTimeoutMinutes": true, "UsdtUnderpayTolerance": true, "TopupOrderTimeoutMinutes": true,
	"AffTopupCommissionRate": true, "AffSubscriptionCommissionRate": true, "AffFreezeDays": true,
	"AffMinWithdrawAmount": true, "AffWithdrawFeeRate": true, "BalanceAdjustMaxAmount": true,
	"SelfApproveAlertAmount": true, "QuotaForNewUser": true, "RefundAlertAmount": true,
}
