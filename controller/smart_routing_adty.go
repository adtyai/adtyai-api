package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/gin-gonic/gin"
)

func srOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func operatorID(c *gin.Context) int { return c.GetInt("id") }

// GET /api/adty/routing/config
func AdtyRoutingGetConfig(c *gin.Context) {
	srOK(c, smartrouting.GetConfig())
}

// PUT /api/adty/routing/config
// Changing active_weight_scheme or weights requires preview_token from POST /weight-schemes/preview (P0 server gate).
func AdtyRoutingPutConfig(c *gin.Context) {
	var req struct {
		smartrouting.Config
		PreviewToken string `json:"preview_token"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	old := smartrouting.GetConfig()
	if err := smartrouting.RequirePreviewForConfigChange(old, req.Config, operatorID(c), req.PreviewToken); err != nil {
		adtyFail(c, http.StatusBadRequest, "preview_required", err.Error(), map[string]any{
			"hint": "POST /api/adty/routing/weight-schemes/preview then retry with preview_token",
		})
		return
	}
	if err := smartrouting.SaveConfig(req.Config, operatorID(c)); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	srOK(c, smartrouting.GetConfig())
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
	row := model.RoutingWeightScheme{Name: req.Name, Weights: w, Note: req.Note, CreatedBy: operatorID(c)}
	if err := model.DB.Create(&row).Error; err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	_ = model.SaveWeightSnapshot("scheme", operatorID(c), map[string]any{"op": "create", "name": req.Name}, row)
	srOK(c, row)
}

func AdtyRoutingUpdateScheme(c *gin.Context) {
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
		adtyFail(c, http.StatusBadRequest, "bad_request", parseErr.Error(), nil)
		return
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
	_ = model.DB.Save(&row).Error
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
	_ = model.DB.Delete(&row).Error
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
	w := req.Weights
	if w.Sum() <= 0 {
		w = smartrouting.GetConfig().Weights
	}
	if req.SchemeName == "" && req.SchemeID > 0 {
		var row model.RoutingWeightScheme
		if err := model.DB.First(&row, req.SchemeID).Error; err == nil {
			req.SchemeName = row.Name
			if req.Weights.Sum() <= 0 {
				if pw, err := smartrouting.ParseWeightsJSON(row.Weights); err == nil {
					w = pw
				}
			}
		}
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
		_ = smartrouting.SaveConfig(cfg, operatorID(c))
		_ = model.SaveWeightSnapshot("rollback", operatorID(c), map[string]any{"to": "official"}, cfg)
		srOK(c, gin.H{"rolled_back": "official"})
		return
	}
	var snap model.RoutingWeightSnapshot
	if err := model.DB.First(&snap, req.SnapshotID).Error; err != nil {
		adtyFail(c, http.StatusNotFound, "not_found", "snapshot not found", nil)
		return
	}
	var cfg smartrouting.Config
	if err := json.Unmarshal([]byte(snap.Snapshot), &cfg); err == nil && (cfg.ActiveWeightScheme != "" || cfg.Weights.Sum() > 0) {
		old := smartrouting.GetConfig()
		if err := smartrouting.RequirePreviewForConfigChange(old, cfg, operatorID(c), req.PreviewToken); err != nil {
			adtyFail(c, http.StatusBadRequest, "preview_required", err.Error(), map[string]any{
				"hint": "preview the target scheme/weights then pass preview_token",
			})
			return
		}
		_ = smartrouting.SaveConfig(cfg, operatorID(c))
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
	var req model.ChannelRouteTag
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ChannelID == 0 || req.Tag == "" {
		adtyFail(c, http.StatusBadRequest, "bad_request", "channel_id and tag required", nil)
		return
	}
	if req.Model == "" {
		req.Model = "*"
	}
	req.CreatedBy = operatorID(c)
	if err := model.DB.Create(&req).Error; err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	_ = model.SaveWeightSnapshot("tag", operatorID(c), map[string]any{"op": "create_tag"}, req)
	srOK(c, req)
}

func AdtyRoutingDeleteTag(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	if id == 0 {
		adtyFail(c, http.StatusBadRequest, "bad_request", "id required", nil)
		return
	}
	_ = model.DB.Delete(&model.ChannelRouteTag{}, id).Error
	srOK(c, gin.H{"deleted": id})
}

func AdtyRoutingClearFakeModel(c *gin.Context) {
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
	_ = q.Delete(&model.ChannelRouteTag{}).Error
	srOK(c, gin.H{"cleared": true})
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
	var rows []model.ChannelCostPrice
	if err := common.DecodeJson(c.Request.Body, &rows); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	for i := range rows {
		rows[i].UpdatedBy = operatorID(c)
		rows[i].UpdatedAt = time.Now()
		_ = model.DB.Save(&rows[i]).Error
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

func AdtyRoutingCostBackfill(c *gin.Context) {
	cfg := smartrouting.GetConfig()
	if !cfg.HistoryBackfillEnabled {
		adtyFail(c, http.StatusBadRequest, "bad_request", "history_backfill_enabled=false", nil)
		return
	}
	// skeleton: mark accepted; actual log→bucket backfill can be expanded
	srOK(c, gin.H{"accepted": true, "note": "backfill skeleton; enable and extend from local logs"})
}

func AdtyRoutingBalanceRefresh(c *gin.Context) {
	// reuse official balance update path via channel id if provided
	srOK(c, gin.H{"accepted": true, "note": "trigger official AutomaticallyUpdateChannels / per-channel balance API from admin UI"})
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
	_ = common.DecodeJson(c.Request.Body, &req)
	_ = model.DB.Model(&model.ChannelAuthFlag{}).Where("id = ?", id).Update("false_positive", req.FalsePositive).Error
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
