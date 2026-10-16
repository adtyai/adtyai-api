package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func srOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func operatorID(c *gin.Context) int { return c.GetInt("id") }

func requireRoutingRoot(c *gin.Context) bool {
	if c.GetInt("role") < common.RoleRootUser {
		adtyFail(c, http.StatusForbidden, "forbidden", "root only", nil)
		return false
	}
	return true
}

// GET /api/adty/routing/config
func AdtyRoutingGetConfig(c *gin.Context) {
	srOK(c, smartrouting.GetConfig())
}

// PUT /api/adty/routing/config
// 审查 P1-3：合并语义（JSON merge）——只更新请求体里出现的字段，未出现的字段保持当前值。
// 只有 weights / active_weight_scheme 实际发生变化时才需要 preview_token（P0 server gate）；
// 纯启停 {"enabled":false} 等非权重字段无需 token。契约见 prep/contract-smart-routing-toggle-config.md。
func AdtyRoutingPutConfig(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(body, &patch); err != nil || patch == nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json: body must be a JSON object", nil)
		return
	}
	var previewToken string
	if raw, ok := patch["preview_token"]; ok {
		if err := json.Unmarshal(raw, &previewToken); err != nil {
			adtyFail(c, http.StatusBadRequest, "bad_request", "preview_token must be a string", nil)
			return
		}
		delete(patch, "preview_token")
	}
	// 审查 N-P1-3：前端可显式声明 kind（仅校验取值，不参与合并；指纹只看目标方案名+权重）
	if raw, ok := patch["kind"]; ok {
		var kind string
		_ = json.Unmarshal(raw, &kind)
		switch kind {
		case "", "put_config", "activate_scheme", "update_weights":
		default:
			adtyFail(c, http.StatusBadRequest, "invalid_kind", "kind must be put_config, activate_scheme or update_weights", nil)
			return
		}
		delete(patch, "kind")
	}
	if len(patch) == 0 {
		adtyFail(c, http.StatusBadRequest, "empty_patch", "no config field to update", nil)
		return
	}
	old := smartrouting.GetConfig()
	// 审查 R-P2-2：未知字段不再静默忽略
	if unknown := unknownRoutingConfigKeys(old, patch); len(unknown) > 0 {
		adtyFail(c, http.StatusBadRequest, "unknown_field", "unknown config field: "+strings.Join(unknown, ","), map[string]any{"fields": unknown})
		return
	}
	neu, err := mergeRoutingConfig(old, patch)
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid field: "+err.Error(), nil)
		return
	}
	// 审查 R-P2-2：active_weight_scheme 必须存在；只切方案名时权重取该方案的值，同时给出时必须一致
	if neu.ActiveWeightScheme != old.ActiveWeightScheme {
		var row model.RoutingWeightScheme
		if err := model.DB.Where("name = ?", neu.ActiveWeightScheme).First(&row).Error; err != nil {
			adtyFail(c, http.StatusBadRequest, "scheme_not_found", "active_weight_scheme does not exist: "+neu.ActiveWeightScheme, nil)
			return
		}
		sw, err := smartrouting.ParseWeightsJSON(row.Weights)
		if err != nil {
			adtyFail(c, http.StatusBadRequest, "bad_scheme", "scheme weights invalid: "+err.Error(), nil)
			return
		}
		if _, hasW := patch["weights"]; !hasW {
			neu.Weights = sw
		} else if neu.Weights != sw {
			adtyFail(c, http.StatusBadRequest, "scheme_weights_mismatch", "weights do not match active_weight_scheme", nil)
			return
		}
	}
	// 审查 R-P2-1：与方案 CRUD 同一套规则（各项 >=0，总和 >0）
	if err := smartrouting.ValidateWeights(neu.Weights); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_weights", err.Error(), nil)
		return
	}
	if err := smartrouting.RequirePreviewForConfigChange(old, neu, operatorID(c), previewToken); err != nil {
		adtyFail(c, http.StatusBadRequest, "preview_required", err.Error(), map[string]any{
			"hint": "POST /api/adty/routing/weight-schemes/preview then retry with preview_token",
		})
		return
	}
	if err := smartrouting.SaveConfig(neu, operatorID(c)); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	srOK(c, smartrouting.GetConfig())
}

// unknownRoutingConfigKeys 返回 patch 中 Config 没有的 JSON 字段名。
func unknownRoutingConfigKeys(cur smartrouting.Config, patch map[string]json.RawMessage) []string {
	known := map[string]json.RawMessage{}
	if b, err := json.Marshal(cur); err == nil {
		_ = json.Unmarshal(b, &known)
	}
	var out []string
	for k := range patch {
		if _, ok := known[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// mergeRoutingConfig 以当前配置的深拷贝为底，把 patch 中出现的字段覆盖上去。
func mergeRoutingConfig(cur smartrouting.Config, patch map[string]json.RawMessage) (smartrouting.Config, error) {
	var neu smartrouting.Config
	base, err := json.Marshal(cur)
	if err != nil {
		return neu, err
	}
	if err := json.Unmarshal(base, &neu); err != nil {
		return neu, err
	}
	p, err := json.Marshal(patch)
	if err != nil {
		return neu, err
	}
	if err := json.Unmarshal(p, &neu); err != nil {
		return neu, err
	}
	return neu, nil
}

// GET /api/adty/routing/scores
func AdtyRoutingGetScores(c *gin.Context) {
	modelName := c.Query("model")
	all := smartrouting.GetAllScores()
	rows := make([]map[string]any, 0)
	for _, s := range all {
		if s == nil {
			continue
		}
		if modelName != "" && s.Model != modelName {
			continue
		}
		rows = append(rows, map[string]any{
			"channel_id": s.ChannelID, "model": s.Model, "score": s.Score,
			"coef": float64(s.RouteWeight) / 100.0, "state": s.State,
			"samples": s.Samples, "updated_at": s.UpdatedAt,
			"s_conn": s.SConn, "s_price": s.SPrice, "s_speed": s.SSpeed,
			"s_stab": s.SStab, "s_auth": s.SAuth, "route_weight": s.RouteWeight,
		})
	}
	srOK(c, rows)
}

// GET /api/adty/routing/scores/:channel_id
func AdtyRoutingGetScoreDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("channel_id"))
	modelName := c.Query("model")
	all := smartrouting.GetAllScores()
	rows := make([]map[string]any, 0)
	for _, s := range all {
		if s == nil || s.ChannelID != id {
			continue
		}
		if modelName != "" && s.Model != modelName {
			continue
		}
		rows = append(rows, map[string]any{
			"channel_id": s.ChannelID, "model": s.Model, "score": s.Score,
			"s_conn": s.SConn, "s_price": s.SPrice, "s_speed": s.SSpeed,
			"s_stab": s.SStab, "s_auth": s.SAuth, "samples": s.Samples,
			"state": s.State, "route_weight": s.RouteWeight, "updated_at": s.UpdatedAt,
		})
	}
	srOK(c, rows)
}

// GET /api/adty/routing/rankings
func AdtyRoutingGetRankings(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		adtyFail(c, http.StatusBadRequest, "bad_request", "model required", nil)
		return
	}
	c.Request.URL.RawQuery = "model=" + modelName
	AdtyRoutingGetScores(c)
}

// GET /api/adty/routing/health/:channel_id — snapshots as coarse series
func AdtyRoutingGetHealth(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("channel_id"))
	var rows []model.ChannelScoreSnapshot
	q := model.DB.Where("channel_id = ?", id)
	if m := c.Query("model"); m != "" {
		q = q.Where("model = ?", m)
	}
	_ = q.Find(&rows).Error
	series := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		series = append(series, map[string]any{"ts": r.UpdatedAt.Unix(), "value": r.Score, "model": r.Model})
	}
	srOK(c, series)
}

// weight schemes CRUD
func AdtyRoutingListSchemes(c *gin.Context) {
	var rows []model.RoutingWeightScheme
	_ = model.DB.Order("id asc").Find(&rows).Error
	srOK(c, rows)
}

func AdtyRoutingCreateScheme(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	var req struct {
		Name    string          `json:"name"`
		Weights json.RawMessage `json:"weights"`
		Note    string          `json:"note"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Name == "" {
		adtyFail(c, http.StatusBadRequest, "bad_request", "name and weights required", nil)
		return
	}
	w := string(req.Weights)
	if w == "" {
		w = model.DefaultWeightJSON
	}
	// 审查 N-P2-4：权重校验 + 重名 409
	if _, err := smartrouting.ParseWeightsJSON(w); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_weights", err.Error(), nil)
		return
	}
	if routingSchemeNameTaken(req.Name, 0) {
		adtyFail(c, http.StatusConflict, "name_exists", "scheme name already exists", nil)
		return
	}
	row := model.RoutingWeightScheme{Name: req.Name, Weights: w, Note: req.Note, CreatedBy: operatorID(c)}
	if err := model.DB.Create(&row).Error; err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	_ = model.SaveWeightSnapshot("scheme", operatorID(c), map[string]any{"op": "create", "name": req.Name}, row)
	srOK(c, row)
}

func AdtyRoutingUpdateScheme(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var row model.RoutingWeightScheme
	if err := model.DB.First(&row, id).Error; err != nil {
		adtyFail(c, http.StatusNotFound, "not_found", "scheme not found", nil)
		return
	}
	var req struct {
		Name         string          `json:"name"`
		Weights      json.RawMessage `json:"weights"`
		Note         string          `json:"note"`
		PreviewToken string          `json:"preview_token"`
		Activate     bool            `json:"activate"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	weightsChanging := len(req.Weights) > 0 && string(req.Weights) != row.Weights
	name := row.Name
	if req.Name != "" {
		name = req.Name
	}
	var wp smartrouting.WeightParts
	var parseErr error
	if weightsChanging {
		wp, parseErr = smartrouting.ParseWeightsJSON(string(req.Weights))
	} else {
		wp, parseErr = smartrouting.ParseWeightsJSON(row.Weights)
	}
	if parseErr != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_weights", parseErr.Error(), nil)
		return
	}
	if req.Name != "" && req.Name != row.Name {
		if routingSchemeNameTaken(req.Name, row.ID) {
			adtyFail(c, http.StatusConflict, "name_exists", "scheme name already exists", nil)
			return
		}
		if row.Name == smartrouting.GetConfig().ActiveWeightScheme && !req.Activate {
			adtyFail(c, http.StatusConflict, "scheme_active", "cannot rename the active scheme", nil)
			return
		}
	}

	// P0: any activate or weight change MUST present a matching preview_token.
	if req.Activate {
		neu := smartrouting.GetConfig()
		neu.ActiveWeightScheme = name
		neu.Weights = wp
		if err := smartrouting.RequirePreviewForConfigChange(smartrouting.GetConfig(), neu, operatorID(c), req.PreviewToken); err != nil {
			adtyFail(c, http.StatusBadRequest, "preview_required", err.Error(), map[string]any{
				"hint": "POST /weight-schemes/preview with kind=put_config|activate_scheme then retry",
			})
			return
		}
	} else if weightsChanging {
		if err := smartrouting.RequirePreviewForSchemeUpdate(id, name, wp, operatorID(c), req.PreviewToken); err != nil {
			adtyFail(c, http.StatusBadRequest, "preview_required", err.Error(), map[string]any{
				"hint": "POST /weight-schemes/preview with kind=update_weights then retry",
			})
			return
		}
	}

	if weightsChanging {
		row.Weights = string(req.Weights)
	}
	if req.Name != "" {
		row.Name = req.Name
	}
	if req.Note != "" {
		row.Note = req.Note
	}
	if err := model.DB.Save(&row).Error; err != nil {
		adtyFail(c, http.StatusInternalServerError, "save_failed", err.Error(), nil)
		return
	}
	_ = model.SaveWeightSnapshot("scheme", operatorID(c), map[string]any{"op": "update", "id": id, "activate": req.Activate}, row)
	if req.Activate {
		cfg := smartrouting.GetConfig()
		cfg.ActiveWeightScheme = row.Name
		cfg.Weights = wp
		if err := smartrouting.SaveConfig(cfg, operatorID(c)); err != nil {
			adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
			return
		}
	}
	srOK(c, row)
}

func AdtyRoutingDeleteScheme(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var row model.RoutingWeightScheme
	if err := model.DB.First(&row, id).Error; err != nil {
		adtyFail(c, http.StatusNotFound, "not_found", "scheme not found", nil)
		return
	}
	if row.Name == "balanced_default" {
		adtyFail(c, http.StatusBadRequest, "bad_request", "cannot delete balanced_default", nil)
		return
	}
	// 审查 N-P2-4：不能删除正在使用的方案
	if row.Name == smartrouting.GetConfig().ActiveWeightScheme {
		adtyFail(c, http.StatusConflict, "scheme_active", "cannot delete the active scheme", nil)
		return
	}
	if err := model.DB.Delete(&row).Error; err != nil {
		adtyFail(c, http.StatusInternalServerError, "delete_failed", err.Error(), nil)
		return
	}
	srOK(c, gin.H{"deleted": id})
}

func AdtyRoutingPreviewScheme(c *gin.Context) {
	var req struct {
		Weights    smartrouting.WeightParts `json:"weights"`
		Model      string                   `json:"model"`
		Group      string                   `json:"group"`
		Kind       string                   `json:"kind"` // activate_scheme|update_weights|put_config
		SchemeID   int64                    `json:"scheme_id"`
		SchemeName string                   `json:"scheme_name"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	// 审查 R-P2-1：显式给了权重就必须合法（不再接受负数）
	if req.Weights != (smartrouting.WeightParts{}) {
		if err := smartrouting.ValidateWeights(req.Weights); err != nil {
			adtyFail(c, http.StatusBadRequest, "invalid_weights", err.Error(), nil)
			return
		}
	}
	w := req.Weights
	if w.Sum() <= 0 {
		w = smartrouting.GetConfig().Weights
	}
	if req.SchemeName == "" && req.SchemeID > 0 {
		var row model.RoutingWeightScheme
		if err := model.DB.First(&row, req.SchemeID).Error; err != nil {
			adtyFail(c, http.StatusNotFound, "scheme_not_found", "scheme not found", nil)
			return
		} else {
			req.SchemeName = row.Name
			if req.Weights.Sum() <= 0 {
				if pw, err := smartrouting.ParseWeightsJSON(row.Weights); err == nil {
					w = pw
				}
			}
		}
	}
	// 审查 P1-3：未指定方案时按当前生效方案出指纹，与 PUT /config 合并后的 active_weight_scheme 一致。
	if req.SchemeName == "" && req.SchemeID == 0 {
		req.SchemeName = smartrouting.GetConfig().ActiveWeightScheme
	}
	kind := req.Kind
	if kind == "" {
		if req.SchemeName != "" && req.SchemeName != smartrouting.GetConfig().ActiveWeightScheme {
			kind = "activate_scheme"
		} else if req.Weights.Sum() > 0 {
			kind = "update_weights"
		} else {
			kind = "put_config"
		}
	}
	all := smartrouting.GetAllScores()
	rows := make([]map[string]any, 0)
	sumW := w.Sum()
	if sumW <= 0 {
		sumW = 1
	}
	for _, s := range all {
		if s == nil {
			continue
		}
		if req.Model != "" && s.Model != req.Model {
			continue
		}
		score := (w.Conn*s.SConn + w.Price*s.SPrice + w.Speed*s.SSpeed + w.Stab*s.SStab + w.Auth*s.SAuth) / sumW
		rw := smartrouting.RouteWeightFromScore(score, s.State)
		rows = append(rows, map[string]any{
			"channel_id": s.ChannelID, "model": s.Model,
			"score_now": s.Score, "score_new": score,
			"rw_now": s.RouteWeight, "rw_new": rw,
		})
	}
	intent := smartrouting.PreviewIntent{
		Kind: kind, SchemeID: req.SchemeID, SchemeName: req.SchemeName,
		Weights: w, OperatorID: operatorID(c),
	}
	token, exp := smartrouting.IssuePreviewToken(intent)
	srOK(c, gin.H{
		"preview":       rows,
		"preview_token": token,
		"expires_at":    exp.Unix(),
		"intent":        intent,
	})
}

func AdtyRoutingExportScheme(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var row model.RoutingWeightScheme
	if err := model.DB.First(&row, id).Error; err != nil {
		adtyFail(c, http.StatusNotFound, "not_found", "scheme not found", nil)
		return
	}
	srOK(c, row)
}

func AdtyRoutingImportScheme(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	var req model.RoutingWeightScheme
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Name == "" {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid scheme", nil)
		return
	}
	req.ID = 0
	req.CreatedBy = operatorID(c)
	if req.Weights == "" {
		req.Weights = model.DefaultWeightJSON
	}
	// 审查 N-P2-4：导入校验权重；重名 409（导出→改名→导入可往返）
	if _, err := smartrouting.ParseWeightsJSON(req.Weights); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_weights", err.Error(), nil)
		return
	}
	if routingSchemeNameTaken(req.Name, 0) {
		adtyFail(c, http.StatusConflict, "name_exists", "scheme name already exists", nil)
		return
	}
	req.CreatedAt, req.UpdatedAt = time.Time{}, time.Time{}
	if err := model.DB.Create(&req).Error; err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	_ = model.SaveWeightSnapshot("scheme", operatorID(c), map[string]any{"op": "import"}, req)
	srOK(c, req)
}

func AdtyRoutingSchemeHistory(c *gin.Context) {
	var rows []model.RoutingWeightSnapshot
	_ = model.DB.Order("id desc").Limit(100).Find(&rows).Error
	srOK(c, rows)
}

func AdtyRoutingSchemeRollback(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	var req struct {
		SnapshotID   int64  `json:"snapshot_id"`
		To           string `json:"to"`
		PreviewToken string `json:"preview_token"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	if req.To == "official" {
		cfg := smartrouting.GetConfig()
		cfg.Enabled = false
		if err := smartrouting.SaveConfig(cfg, operatorID(c)); err != nil {
			adtyFail(c, http.StatusInternalServerError, "save_failed", err.Error(), nil)
			return
		}
		_ = model.SaveWeightSnapshot("rollback", operatorID(c), map[string]any{"to": "official"}, cfg)
		srOK(c, gin.H{"rolled_back": "official"})
		return
	}
	var snap model.RoutingWeightSnapshot
	if err := model.DB.First(&snap, req.SnapshotID).Error; err != nil {
		adtyFail(c, http.StatusNotFound, "not_found", "snapshot not found", nil)
		return
	}
	// 审查 P2-3：快照解析失败 / 不是可回滚的配置 → 400 bad_snapshot；保存失败 → 500，不再误报成功。
	var cfg smartrouting.Config
	if err := json.Unmarshal([]byte(snap.Snapshot), &cfg); err != nil || (cfg.ActiveWeightScheme == "" && cfg.Weights.Sum() <= 0) {
		adtyFail(c, http.StatusBadRequest, "bad_snapshot", "snapshot is not a restorable routing config", nil)
		return
	}
	old := smartrouting.GetConfig()
	if err := smartrouting.RequirePreviewForConfigChange(old, cfg, operatorID(c), req.PreviewToken); err != nil {
		adtyFail(c, http.StatusBadRequest, "preview_required", err.Error(), map[string]any{
			"hint": "preview the target scheme/weights then pass preview_token",
		})
		return
	}
	if err := smartrouting.SaveConfig(cfg, operatorID(c)); err != nil {
		adtyFail(c, http.StatusInternalServerError, "save_failed", err.Error(), nil)
		return
	}
	srOK(c, gin.H{"rolled_back": req.SnapshotID})
}

func AdtyRoutingSimulate(c *gin.Context) {
	var req struct {
		Model string `json:"model"`
		Group string `json:"group"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Model == "" {
		adtyFail(c, http.StatusBadRequest, "bad_request", "model required", nil)
		return
	}
	if req.Group == "" {
		req.Group = "default"
	}
	var channels []*model.Channel
	_ = model.DB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error
	filtered := make([]*model.Channel, 0)
	for _, ch := range channels {
		okM, okG := false, false
		for _, m := range ch.GetModels() {
			if m == req.Model {
				okM = true
				break
			}
		}
		for _, g := range splitGroup(ch.Group) {
			if g == req.Group {
				okG = true
				break
			}
		}
		if okM && okG {
			filtered = append(filtered, ch)
		}
	}
	srOK(c, smartrouting.SimulateSelection(req.Model, req.Group, filtered))
}

func splitGroup(s string) []string {
	var out []string
	cur := ""
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			p := cur
			for len(p) > 0 && p[0] == ' ' {
				p = p[1:]
			}
			if p != "" {
				out = append(out, p)
			}
			cur = ""
			continue
		}
		cur += string(s[i])
	}
	return out
}

// tags
func AdtyRoutingListTags(c *gin.Context) {
	cid, _ := strconv.Atoi(c.Query("channel_id"))
	rows, err := model.ListRouteTags(cid)
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	srOK(c, rows)
}

func AdtyRoutingCreateTag(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	var req model.ChannelRouteTag
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ChannelID == 0 || req.Tag == "" {
		adtyFail(c, http.StatusBadRequest, "bad_request", "channel_id and tag required", nil)
		return
	}
	if req.Model == "" {
		req.Model = "*"
	}
	if !routingChannelExists(c, req.ChannelID) {
		return
	}
	switch req.Tag {
	case "excluded", "fake_model", "fallback_only", "locked":
	default:
		adtyFail(c, http.StatusBadRequest, "invalid_tag", "tag must be excluded, fake_model, fallback_only or locked", nil)
		return
	}
	req.ID = 0
	req.CreatedBy = operatorID(c)
	if err := model.DB.Create(&req).Error; err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	_ = model.SaveWeightSnapshot("tag", operatorID(c), map[string]any{"op": "create_tag"}, req)
	srOK(c, req)
}

func AdtyRoutingDeleteTag(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	if id == 0 {
		adtyFail(c, http.StatusBadRequest, "bad_request", "id required", nil)
		return
	}
	res := model.DB.Delete(&model.ChannelRouteTag{}, id)
	if res.Error != nil {
		adtyFail(c, http.StatusInternalServerError, "delete_failed", res.Error.Error(), nil)
		return
	}
	if res.RowsAffected == 0 {
		adtyFail(c, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	srOK(c, gin.H{"deleted": id})
}

func AdtyRoutingClearFakeModel(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	var req struct {
		ChannelID int    `json:"channel_id"`
		Model     string `json:"model"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ChannelID == 0 {
		adtyFail(c, http.StatusBadRequest, "bad_request", "channel_id required", nil)
		return
	}
	q := model.DB.Where("channel_id = ? AND tag = ?", req.ChannelID, "fake_model")
	if req.Model != "" {
		q = q.Where("model = ?", req.Model)
	}
	res := q.Delete(&model.ChannelRouteTag{})
	if res.Error != nil {
		adtyFail(c, http.StatusInternalServerError, "delete_failed", res.Error.Error(), nil)
		return
	}
	srOK(c, gin.H{"cleared": true, "deleted": res.RowsAffected})
}

// breaker
func AdtyRoutingBreakerEvents(c *gin.Context) {
	var rows []model.ChannelBreakerEvent
	q := model.DB.Order("id desc").Limit(200)
	if id, _ := strconv.Atoi(c.Query("channel_id")); id > 0 {
		q = q.Where("channel_id = ?", id)
	}
	_ = q.Find(&rows).Error
	srOK(c, rows)
}

func AdtyRoutingBreakerStages(c *gin.Context) {
	var rows []model.ChannelBreakerState
	_ = model.DB.Find(&rows).Error
	srOK(c, rows)
}

// prices / cost
func AdtyRoutingGetPrices(c *gin.Context) {
	var rows []model.ChannelCostPrice
	q := model.DB
	if id, _ := strconv.Atoi(c.Query("channel_id")); id > 0 {
		q = q.Where("channel_id = ?", id)
	}
	_ = q.Find(&rows).Error
	srOK(c, rows)
}

func AdtyRoutingPutPrices(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	var rows []model.ChannelCostPrice
	if err := common.DecodeJson(c.Request.Body, &rows); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	// 审查 N-P2-5：先全部校验（渠道存在、数值非负），再在一个事务里写，失败不吞
	for _, r := range rows {
		if r.ChannelID <= 0 || r.Model == "" {
			adtyFail(c, http.StatusBadRequest, "bad_request", "channel_id and model required", nil)
			return
		}
		if r.RechargeCreditsPerCNY < 0 || r.ChannelRatio < 0 || r.ModelRefUSDPerMTok < 0 || r.InPriceUSD < 0 || r.OutPriceUSD < 0 {
			adtyFail(c, http.StatusBadRequest, "invalid_price", "prices must be >= 0", map[string]any{"channel_id": r.ChannelID, "model": r.Model})
			return
		}
		if !routingChannelExists(c, r.ChannelID) {
			return
		}
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		for i := range rows {
			rows[i].UpdatedBy = operatorID(c)
			rows[i].UpdatedAt = time.Now()
			if err := tx.Save(&rows[i]).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		adtyFail(c, http.StatusInternalServerError, "save_failed", err.Error(), nil)
		return
	}
	_ = model.SaveWeightSnapshot("price", operatorID(c), map[string]any{"op": "put_prices", "n": len(rows)}, rows)
	srOK(c, rows)
}

func AdtyRoutingCostSummary(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		adtyFail(c, http.StatusForbidden, "forbidden", "root only", nil)
		return
	}
	from := c.Query("from")
	to := c.Query("to")
	type agg struct {
		ChannelID int
		CostCNY   float64
		Requests  int64
	}
	var rows []agg
	q := model.DB.Model(&model.ChannelCostLedger{}).
		Select("channel_id, SUM(cost_cny) as cost_cny, COUNT(*) as requests").
		Where("attempt_failed = ?", false)
	if from != "" {
		q = q.Where("created_at >= ?", from)
	}
	if to != "" {
		q = q.Where("created_at < ?", to)
	}
	_ = q.Group("channel_id").Scan(&rows).Error
	srOK(c, rows)
}

func AdtyRoutingCostRankings(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		adtyFail(c, http.StatusForbidden, "forbidden", "root only", nil)
		return
	}
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	rows, err := smartrouting.CostRankingsByMargin(from, to)
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	srOK(c, rows)
}

func AdtyRoutingCostLive(c *gin.Context) {
	cid, _ := strconv.Atoi(c.Query("channel_id"))
	m := c.Query("model")
	unit := smartrouting.LiveUnitCNY(cid, m, smartrouting.GetConfig())
	if unit <= 0 {
		unit = smartrouting.ConfiguredUnitCNY(cid, m)
	}
	srOK(c, gin.H{"channel_id": cid, "model": m, "unit_cny_per_mtok": unit})
}

func AdtyRoutingCostGuards(c *gin.Context) {
	day := time.Now().Format("2006-01-02")
	var rows []model.ChannelCostGuard
	_ = model.DB.Where("day = ?", day).Find(&rows).Error
	srOK(c, rows)
}

// AdtyRoutingCostBackfill POST /api/adty/routing/cost/backfill（root）
// 审查 N-P1-5：真实从本地消费日志回填成本流水。body: {"from":unix,"to":unix,"limit":n}，默认最近 24h。
func AdtyRoutingCostBackfill(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	cfg := smartrouting.GetConfig()
	if !cfg.HistoryBackfillEnabled {
		adtyFail(c, http.StatusBadRequest, "backfill_disabled", "history_backfill_enabled=false", nil)
		return
	}
	var req struct {
		From  int64 `json:"from"`
		To    int64 `json:"to"`
		Limit int   `json:"limit"`
	}
	_ = c.ShouldBindJSON(&req)
	to := time.Now()
	if req.To > 0 {
		to = time.Unix(req.To, 0)
	}
	from := to.Add(-24 * time.Hour)
	if req.From > 0 {
		from = time.Unix(req.From, 0)
	}
	scanned, inserted, err := smartrouting.BackfillCostFromLogs(from, to, req.Limit)
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "backfill_failed", err.Error(), nil)
		return
	}
	srOK(c, gin.H{"from": from.Unix(), "to": to.Unix(), "scanned": scanned, "inserted": inserted})
}

var routingBalanceRefreshRunning atomic.Bool

// AdtyRoutingBalanceRefresh POST /api/adty/routing/balance/refresh（root）
// 审查 N-P1-5：真实刷新渠道余额。带 channel_id 同步刷新该渠道并返回余额；不带则后台刷新全部启用渠道（同一时间只跑一个）。
func AdtyRoutingBalanceRefresh(c *gin.Context) {
	if !requireRoutingRoot(c) {
		return
	}
	var req struct {
		ChannelID int `json:"channel_id"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.ChannelID == 0 {
		req.ChannelID, _ = strconv.Atoi(c.Query("channel_id"))
	}
	if req.ChannelID > 0 {
		ch, err := model.GetChannelById(req.ChannelID, true)
		if err != nil || ch == nil {
			adtyFail(c, http.StatusNotFound, "not_found", "channel not found", nil)
			return
		}
		r := routingRefreshOneBalance(ch)
		if r.Status == "failed" {
			adtyFail(c, http.StatusBadGateway, "balance_query_failed", r.Error, nil)
			return
		}
		srOK(c, r)
		return
	}
	if !routingBalanceRefreshRunning.CompareAndSwap(false, true) {
		adtyFail(c, http.StatusConflict, "refresh_running", "a full balance refresh is already running", nil)
		return
	}
	go func() {
		defer routingBalanceRefreshRunning.Store(false)
		defer func() {
			if r := recover(); r != nil {
				common.SysError(common.LogText("routing balance refresh panic: %v", r))
			}
		}()
		if err := routingRefreshAllBalances(); err != nil {
			common.SysError(common.LogText("routing balance refresh failed: %s", err.Error()))
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"started": true}})
}

// auth flags
func AdtyRoutingAuthFlags(c *gin.Context) {
	var rows []model.ChannelAuthFlag
	q := model.DB.Order("id desc").Limit(200)
	if id, _ := strconv.Atoi(c.Query("channel_id")); id > 0 {
		q = q.Where("channel_id = ?", id)
	}
	_ = q.Find(&rows).Error
	srOK(c, rows)
}

func AdtyRoutingMarkAuthFlag(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		FalsePositive bool `json:"false_positive"`
	}
	if !requireRoutingRoot(c) {
		return
	}
	_ = common.DecodeJson(c.Request.Body, &req)
	res := model.DB.Model(&model.ChannelAuthFlag{}).Where("id = ?", id).Update("false_positive", req.FalsePositive)
	if res.Error != nil {
		adtyFail(c, http.StatusInternalServerError, "save_failed", res.Error.Error(), nil)
		return
	}
	if res.RowsAffected == 0 {
		adtyFail(c, http.StatusNotFound, "not_found", "auth flag not found", nil)
		return
	}
	srOK(c, gin.H{"id": id, "false_positive": req.FalsePositive})
}

// decision replay
func AdtyRoutingDecision(c *gin.Context) {
	logID, _ := strconv.Atoi(c.Query("log_id"))
	reqID := c.Query("request_id")
	var log model.Log
	q := model.LOG_DB
	if logID > 0 {
		q = q.Where("id = ?", logID)
	} else if reqID != "" {
		q = q.Where("request_id = ?", reqID)
	} else {
		adtyFail(c, http.StatusBadRequest, "bad_request", "log_id or request_id required", nil)
		return
	}
	if err := q.First(&log).Error; err != nil {
		adtyFail(c, http.StatusNotFound, "not_found", "log not found", nil)
		return
	}
	srOK(c, gin.H{"log_id": log.Id, "request_id": log.RequestId, "other": log.Other})
}

func AdtyRoutingDecisions(c *gin.Context) {
	var rows []model.Log
	_ = model.LOG_DB.Order("id desc").Limit(50).Find(&rows).Error
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{"log_id": r.Id, "request_id": r.RequestId, "other": r.Other})
	}
	srOK(c, out)
}

// POST /weights/rollback alias
func AdtyRoutingWeightsRollback(c *gin.Context) {
	AdtyRoutingSchemeRollback(c)
}

func routingSchemeNameTaken(name string, exceptID int64) bool {
	var n int64
	q := model.DB.Model(&model.RoutingWeightScheme{}).Where("name = ?", name)
	if exceptID > 0 {
		q = q.Where("id <> ?", exceptID)
	}
	_ = q.Count(&n).Error
	return n > 0
}

// routingChannelExists 审查 N-P2-5：渠道不存在 → 404，不再静默成功。
func routingChannelExists(c *gin.Context, channelID int) bool {
	var n int64
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Count(&n).Error; err != nil || n == 0 {
		adtyFail(c, http.StatusNotFound, "channel_not_found", "channel not found", map[string]any{"channel_id": channelID})
		return false
	}
	return true
}

// routingBalanceResult 审查 R-P1-2：路由页刷新余额的单渠道结果。
// status: ok（已写余额）/ unsupported（上游无余额接口，未写、未当 0）/ failed（查询出错）。
type routingBalanceResult struct {
	ChannelID   int     `json:"channel_id"`
	Status      string  `json:"status"`
	Balance     float64 `json:"balance"`
	RawResponse string  `json:"raw_response,omitempty"`
	Error       string  `json:"error,omitempty"`
	UpdatedAt   int64   `json:"updated_at"`
}

func isBalanceUnsupportedErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errBalanceUnsupported) {
		return true
	}
	return strings.Contains(err.Error(), "Not implemented")
}

// routingRefreshOneBalance 只查询并写入余额（由 updateChannelBalance 内部写库），
// 绝不禁用渠道；低余额交给 balance_patrol 告警/熔断统一处理。
func routingRefreshOneBalance(ch *model.Channel) routingBalanceResult {
	r := routingBalanceResult{ChannelID: ch.Id, UpdatedAt: time.Now().Unix()}
	if ch.Type == constant.ChannelTypeTaskPlugin || ch.ChannelInfo.IsMultiKey {
		r.Status = "unsupported"
		return r
	}
	res, err := updateChannelBalance(ch)
	switch {
	case isBalanceUnsupportedErr(err):
		r.Status = "unsupported"
	case err != nil:
		r.Status = "failed"
		r.Error = err.Error()
	default:
		r.Status = "ok"
		r.Balance = res.Balance
		r.RawResponse = res.RawResponse
	}
	return r
}

// routingRefreshAllBalances 审查 R-P1-2：替代官方 updateAllChannelsBalance（余额<=0 会禁用渠道）。
func routingRefreshAllBalances() error {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, ch := range channels {
		if ch.Status != common.ChannelStatusEnabled {
			continue
		}
		r := routingRefreshOneBalance(ch)
		counts[r.Status]++
		time.Sleep(common.RequestInterval)
	}
	common.SysLog(common.LogText("routing balance refresh done: ok=%d unsupported=%d failed=%d", counts["ok"], counts["unsupported"], counts["failed"]))
	return nil
}
