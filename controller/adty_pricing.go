package controller

import (
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// 官方价（任务 8.4）。公开接口给价格页算节省；后台三个接口只有超级管理员，放在系统设置里。

// adtyPublicOfficialPrice 是公开接口的一项，没有的价格返回 null。
type adtyPublicOfficialPrice struct {
	Input      *decimal.Decimal `json:"input"`
	Output     *decimal.Decimal `json:"output"`
	CacheRead  *decimal.Decimal `json:"cache_read"`
	CacheWrite *decimal.Decimal `json:"cache_write"`
	PerCall    *decimal.Decimal `json:"per_call"`
}

// GetAdtyOfficialPrices 返回有官方价的模型，updated_at 取各行最后一次更新的时间。
func GetAdtyOfficialPrices(c *gin.Context) {
	rows, err := model.ListAdtyOfficialPrices()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	prices := make(map[string]adtyPublicOfficialPrice, len(rows))
	var updatedAt int64
	for _, row := range rows {
		if !row.HasAnyPrice() {
			continue
		}
		prices[row.ModelName] = adtyPublicOfficialPrice{
			Input:      row.InputPerMTok,
			Output:     row.OutputPerMTok,
			CacheRead:  row.CacheReadPerMTok,
			CacheWrite: row.CacheWritePerMTok,
			PerCall:    row.PerCall,
		}
		updatedAt = max(updatedAt, row.UpdatedAt)
	}
	common.ApiSuccess(c, gin.H{"prices": prices, "updated_at": updatedAt})
}

// GetAdtyAdminOfficialPrices 返回表里的行，加上已启用但还没有行的模型（映射为空），按模型名排序。
func GetAdtyAdminOfficialPrices(c *gin.Context) {
	rows, err := model.ListAdtyOfficialPrices()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	known := make(map[string]bool, len(rows))
	for _, row := range rows {
		known[row.ModelName] = true
	}
	enabledModels, err := model.GetEnabledModels()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, name := range enabledModels {
		if name != "" && !known[name] {
			known[name] = true
			rows = append(rows, model.AdtyOfficialPrice{ModelName: name})
		}
	}
	slices.SortFunc(rows, func(a, b model.AdtyOfficialPrice) int { return strings.Compare(a.ModelName, b.ModelName) })
	common.ApiSuccess(c, rows)
}

type adtyOfficialPriceRequest struct {
	SourceId          string           `json:"source_id"`
	InputPerMTok      *decimal.Decimal `json:"input_per_mtok"`
	OutputPerMTok     *decimal.Decimal `json:"output_per_mtok"`
	CacheReadPerMTok  *decimal.Decimal `json:"cache_read_per_mtok"`
	CacheWritePerMTok *decimal.Decimal `json:"cache_write_per_mtok"`
	PerCall           *decimal.Decimal `json:"per_call"`
	Override          bool             `json:"override"`
}

// PutAdtyOfficialPrice 保存一个模型的映射、价格和覆盖开关，整行替换。
// 路由用 *model，模型名里可以有斜杠。没打开覆盖时，下次同步按映射改写四项 token 价格。
func PutAdtyOfficialPrice(c *gin.Context) {
	name := strings.TrimSpace(strings.TrimPrefix(c.Param("model"), "/"))
	if name == "" || len(name) > 191 {
		common.ApiErrorT(c, "Model name is required")
		return
	}
	var req adtyOfficialPriceRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	sourceID := strings.TrimSpace(req.SourceId)
	if sourceID != "" {
		provider, modelID, ok := strings.Cut(sourceID, "/")
		if !ok || provider == "" || modelID == "" || len(sourceID) > 255 {
			common.ApiErrorT(c, "The models.dev ID must look like provider/model")
			return
		}
	}
	for _, price := range []*decimal.Decimal{req.InputPerMTok, req.OutputPerMTok, req.CacheReadPerMTok, req.CacheWritePerMTok, req.PerCall} {
		if price != nil && price.IsNegative() {
			common.ApiErrorT(c, "Prices cannot be negative")
			return
		}
	}
	row := &model.AdtyOfficialPrice{
		ModelName:         name,
		SourceId:          sourceID,
		InputPerMTok:      req.InputPerMTok,
		OutputPerMTok:     req.OutputPerMTok,
		CacheReadPerMTok:  req.CacheReadPerMTok,
		CacheWritePerMTok: req.CacheWritePerMTok,
		PerCall:           req.PerCall,
		Source:            model.AdtyOfficialPriceSourceManual,
		Override:          req.Override,
	}
	if err := model.SaveAdtyOfficialPrice(row); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.official_price.update", map[string]any{"model": name, "source_id": sourceID, "override": req.Override})
	common.ApiSuccess(c, row)
}

// SyncAdtyOfficialPrices 立即同步一次。失败时旧数据保留，把原因告诉管理员。
func SyncAdtyOfficialPrices(c *gin.Context) {
	updated, err := service.SyncOfficialPrices(c.Request.Context())
	if err != nil {
		common.SysError(common.LogText("official price sync failed: %s", err.Error()))
		common.ApiErrorT(c, "Official price sync failed: {{error}}", map[string]any{"error": err.Error()})
		return
	}
	recordManageAudit(c, "adty.official_price.sync", map[string]any{"updated": updated})
	common.ApiSuccess(c, gin.H{"updated": updated, "synced_at": time.Now().Unix()})
}
