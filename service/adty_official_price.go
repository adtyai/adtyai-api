package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
)

// 官方价同步（任务 8.4）：从 models.dev 拉原价，写进 adty_official_prices。

// adtyModelsDevURL 和 adtyModelsDevClient 留给测试替换。整份数据约 6 MB，超时 30 秒。
var (
	adtyModelsDevURL    = "https://models.dev/api.json"
	adtyModelsDevClient = &http.Client{Timeout: 30 * time.Second}
)

// 读响应的上限，防止对方返回异常大的内容。
const adtyModelsDevMaxBytes = 64 << 20

// adtyOfficialPriceSyncMu 让定时同步和后台「立即同步」不同时跑。
var adtyOfficialPriceSyncMu sync.Mutex

// modelsDevCost 是 models.dev 里一个模型的价格，单位每百万 tokens 美元。
// tiers、context_over_200k、reasoning、音频价格都不用，解码时忽略。
type modelsDevCost struct {
	Input      *decimal.Decimal `json:"input"`
	Output     *decimal.Decimal `json:"output"`
	CacheRead  *decimal.Decimal `json:"cache_read"`
	CacheWrite *decimal.Decimal `json:"cache_write"`
}

// parseModelsDevCosts 把 api.json 解析成 "provider/model" → 价格。
// 单个模型解析不了或者一项价格都没有就跳过；负数价格当作没有。一条可用的都没有算失败。
func parseModelsDevCosts(raw []byte) (map[string]modelsDevCost, error) {
	var providers map[string]struct {
		Models map[string]common.RawMessage `json:"models"`
	}
	if err := common.Unmarshal(raw, &providers); err != nil {
		return nil, fmt.Errorf("decode models.dev response: %w", err)
	}
	costs := map[string]modelsDevCost{}
	for providerID, provider := range providers {
		for modelID, rawModel := range provider.Models {
			var entry struct {
				Cost *modelsDevCost `json:"cost"`
			}
			if err := common.Unmarshal(rawModel, &entry); err != nil || entry.Cost == nil {
				continue
			}
			cost := *entry.Cost
			for _, price := range []**decimal.Decimal{&cost.Input, &cost.Output, &cost.CacheRead, &cost.CacheWrite} {
				if *price != nil && (*price).IsNegative() {
					*price = nil
				}
			}
			if cost.Input == nil && cost.Output == nil && cost.CacheRead == nil && cost.CacheWrite == nil {
				continue
			}
			costs[providerID+"/"+modelID] = cost
		}
	}
	if len(costs) == 0 {
		return nil, errors.New("no models.dev prices found")
	}
	return costs, nil
}

func fetchModelsDevCosts(ctx context.Context) (map[string]modelsDevCost, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, adtyModelsDevURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := adtyModelsDevClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, adtyModelsDevMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > adtyModelsDevMaxBytes {
		return nil, errors.New("models.dev response is too large")
	}
	return parseModelsDevCosts(raw)
}

// SyncOfficialPrices 拉一次 models.dev 并更新官方价，返回改动的行数。
// 已映射、没手动覆盖的行更新四项 token 价格；没有行的已启用模型按 ID 完全相同自动映射。
// 拉取或解析失败时不写库，旧数据保留。映射的条目在 models.dev 里找不到时那一行也不动。
func SyncOfficialPrices(ctx context.Context) (updated int, err error) {
	adtyOfficialPriceSyncMu.Lock()
	defer adtyOfficialPriceSyncMu.Unlock()

	costs, err := fetchModelsDevCosts(ctx)
	if err != nil {
		return 0, err
	}
	rows, err := model.ListAdtyOfficialPrices()
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	known := make(map[string]bool, len(rows))
	var items []model.AdtyOfficialPriceSync
	for _, row := range rows {
		known[row.ModelName] = true
		if row.Override || row.SourceId == "" {
			continue
		}
		cost, ok := costs[row.SourceId]
		if !ok {
			continue
		}
		items = append(items, model.AdtyOfficialPriceSync{Row: officialPriceFromCost(row.ModelName, row.SourceId, cost, now)})
	}

	providers := slices.Clone(adty_setting.GetOfficialPriceSetting().AutoMapProviders)
	enabledModels, err := model.GetEnabledModels()
	if err != nil {
		return 0, err
	}
	for _, name := range enabledModels {
		if known[name] || strings.TrimSpace(name) == "" {
			continue
		}
		for _, provider := range providers {
			sourceID := strings.TrimSpace(provider) + "/" + name
			if cost, ok := costs[sourceID]; ok {
				items = append(items, model.AdtyOfficialPriceSync{Row: officialPriceFromCost(name, sourceID, cost, now), Create: true})
				break
			}
		}
	}
	return model.ApplyAdtyOfficialPriceSync(items)
}

func officialPriceFromCost(modelName, sourceID string, cost modelsDevCost, now int64) model.AdtyOfficialPrice {
	return model.AdtyOfficialPrice{
		ModelName:         modelName,
		SourceId:          sourceID,
		InputPerMTok:      cost.Input,
		OutputPerMTok:     cost.Output,
		CacheReadPerMTok:  cost.CacheRead,
		CacheWritePerMTok: cost.CacheWrite,
		Source:            model.AdtyOfficialPriceSourceModelsDev,
		SyncedAt:          now,
		UpdatedAt:         now,
	}
}

var adtyOfficialPriceSyncOnce sync.Once

// StartAdtyOfficialPriceSync 只在主节点启动：启动时同步一次，之后每 ADTY_OFFICIAL_PRICE_SYNC_HOURS 小时（默认 24）一次。
func StartAdtyOfficialPriceSync() {
	if !common.IsMasterNode {
		return
	}
	adtyOfficialPriceSyncOnce.Do(func() {
		hours := max(common.GetEnvOrDefault("ADTY_OFFICIAL_PRICE_SYNC_HOURS", 24), 1)
		go func() {
			ticker := time.NewTicker(time.Duration(hours) * time.Hour)
			defer ticker.Stop()
			for {
				if updated, err := SyncOfficialPrices(context.Background()); err != nil {
					common.SysError(common.LogText("official price sync failed: %s", err.Error()))
				} else {
					common.SysLog(common.LogText("official price sync updated %d models", updated))
				}
				<-ticker.C
			}
		}()
	})
}

func init() {
	adtyMasterTasks = append(adtyMasterTasks, StartAdtyOfficialPriceSync)
}
