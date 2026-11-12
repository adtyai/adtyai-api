package service

import (
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 分站定价：站长倍率 k 乘进扣费（任务 11.4，执行层决定 25）。
// k 只乘进分组倍率：按倍率、按次、阶梯计价、图片、任务提交、工具调用附加费、违规扣费都从分组倍率走，
// 不经过 HandleGroupRatio 的 Realtime 预扣和任务按 token 重算各自乘一次。
// k 不进 PriceData.OtherRatios（会出现在客户日志的计价参数里，等于把加价幅度告诉客户）。

// gin 上下文里记下的 k 和 d：同一个请求后面的消费日志都从这里取（11.5 的记账）。
const (
	AdtySiteMarkupContextKey     = "adty_site_markup"
	AdtyTeamDiscountContextKey   = "adty_team_discount"
	AdtyTeamPriceBlockedKey      = "adty_team_price_blocked"
	AdtySiteCostFactorContextKey = "adty_site_cost_factor"
	AdtySiteIdContextKey         = "adty_site_id"
	AdtySiteOwnerContextKey      = "adty_site_owner_user_id"
)

var (
	ErrAdtySitePricingConflict = common.NewMessage("The prices were changed elsewhere. Reload and try again")
	ErrAdtySitePaused          = common.NewMessage("Prices cannot be changed while the site is paused")
)

// adtySitePriceCache 是按站点分组的定价，变更时清掉；多实例下按 SYNC_FREQUENCY 过期。
var (
	adtySitePriceMu       sync.Mutex
	adtySitePriceCache    map[int][]model.AdtySitePrice
	adtySitePriceLoadedAt time.Time
)

// InvalidateAdtySitePriceCache 在定价变更后调用。
func InvalidateAdtySitePriceCache() {
	adtySitePriceMu.Lock()
	adtySitePriceCache = nil
	adtySitePriceMu.Unlock()
}

func adtySitePrices(siteId int) []model.AdtySitePrice {
	adtySitePriceMu.Lock()
	defer adtySitePriceMu.Unlock()
	if adtySitePriceCache == nil || time.Since(adtySitePriceLoadedAt) >= adtySiteCacheTTL() {
		var rows []model.AdtySitePrice
		if err := model.DB.Find(&rows).Error; err != nil {
			common.SysError(common.LogText("load adty site prices failed: %s", err.Error()))
			return nil
		}
		grouped := map[int][]model.AdtySitePrice{}
		for _, row := range rows {
			grouped[row.SiteId] = append(grouped[row.SiteId], row)
		}
		adtySitePriceCache, adtySitePriceLoadedAt = grouped, time.Now()
	}
	return adtySitePriceCache[siteId]
}

// AdtyBillingSiteForUser 是用户所属、并且按分站价扣费的站点：active、paused、closing 都按分站价，
// 其余（pending、closed、查不到）返回 nil。充值下单也用它决定加赠档位和谁出加赠。
func AdtyBillingSiteForUser(userId int) *model.AdtySite {
	if userId <= 0 || !resellerSetting().Enabled {
		return nil
	}
	site := AdtySiteForUser(userId)
	if !AdtySiteServesCustomers(site) {
		return nil
	}
	return site
}

// AdtySiteMarkup 返回分站客户在这个分组的 k；主站客户、站点没有开着时返回 1。只读缓存。
// 已经保存的价如果高于当前上限，按当前上限返回。
func AdtySiteMarkup(userId int, group string) decimal.Decimal {
	site := AdtyBillingSiteForUser(userId)
	if site == nil {
		return decimal.NewFromInt(1)
	}
	return adtyClampMarkup(site, ResolveAdtySiteMarkup(adtySitePrices(site.Id), userId, group), userId, group)
}

// AdtySiteVisitorMarkup 是分站访客（没登录）看价格页时的 k：全站 + 分组 → 全站 → 1。
func AdtySiteVisitorMarkup(site *model.AdtySite, group string) decimal.Decimal {
	if !AdtySiteServesCustomers(site) || !resellerSetting().Enabled {
		return decimal.NewFromInt(1)
	}
	return adtyClampMarkup(site, ResolveAdtySiteMarkup(adtySitePrices(site.Id), 0, group), 0, group)
}

// AdtySiteCostFactor 是站点当前的 d：单站设的 d → 业绩档位的 d → 全局 d；低于 min_cost_factor 时用下限。
// 全局 d 没设时返回 false（这时分站功能本来就打不开）。
func AdtySiteCostFactor(site *model.AdtySite) (decimal.Decimal, bool) {
	cfg := resellerSetting()
	var d decimal.Decimal
	switch {
	case site != nil && site.CostFactor != nil:
		d = *site.CostFactor
	case site != nil && site.TierCostFactor != nil && (len(cfg.CostFactorTiers) > 0 || site.TierCode != ""):
		d = *site.TierCostFactor
	case cfg.CostFactor != nil:
		d = decimal.NewFromFloat(*cfg.CostFactor)
	default:
		return decimal.Zero, false
	}
	if cfg.MinCostFactor != nil {
		d = decimal.Max(d, decimal.NewFromFloat(*cfg.MinCostFactor))
	}
	if !d.IsPositive() || d.GreaterThan(decimal.NewFromInt(1)) {
		return decimal.Zero, false
	}
	return d, true
}

// adtyMarkupFloat 把 k 转成 float 乘进分组倍率。结果不是有限正数时按 1 处理并记日志（billing.md 的安全不变式）。
func adtyMarkupFloat(k decimal.Decimal, userId int, group string) float64 {
	value := k.InexactFloat64()
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 1 {
		common.SysError(common.LogText("invalid site markup %s for user %d group %s, using 1", k.String(), userId, group))
		return 1
	}
	return value
}

// AdtyApplySiteMarkup 在算出分组倍率之后调用：分站客户把 k 乘进去，并把 k、d 写进 gin 上下文。
// 返回乘过 k 的分组倍率和 k（1 表示没乘）。上游的 HandleGroupRatio、Realtime 预扣都调它。
func AdtyApplySiteMarkup(c *gin.Context, userId int, group string, groupRatio float64) (float64, float64) {
	marked, k := adtyApplySiteMarkupOnly(c, userId, group, groupRatio)
	return adtyApplyTeamProtocolPrice(c, userId, groupRatio, marked, k)
}

func adtyApplySiteMarkupOnly(c *gin.Context, userId int, group string, groupRatio float64) (float64, float64) {
	site := AdtyBillingSiteForUser(userId)
	if site == nil {
		return groupRatio, 1
	}
	k := adtyMarkupFloat(adtyClampMarkup(site, ResolveAdtySiteMarkup(adtySitePrices(site.Id), userId, group), userId, group), userId, group)
	if c != nil {
		c.Set(AdtySiteMarkupContextKey, k)
		c.Set(AdtySiteIdContextKey, site.Id)
		ownerID, err := model.CurrentAdtySiteFinancialOwnerTx(model.DB, site.Id)
		if err != nil {
			common.SysError("Request earning owner snapshot unavailable")
		}
		c.Set(AdtySiteOwnerContextKey, ownerID)
		if d, ok := AdtySiteCostFactor(site); ok {
			c.Set(AdtySiteCostFactorContextKey, d.InexactFloat64())
		}
	}
	if k == 1 {
		return groupRatio, 1
	}
	marked := groupRatio * k
	if math.IsNaN(marked) || math.IsInf(marked, 0) || marked < 0 {
		common.SysError(common.LogText("site markup overflow for user %d group %s, using 1", userId, group))
		return groupRatio, 1
	}
	return marked, k
}

// adtyApplyTeamProtocolPrice multiplies the site selling price by the team discount.
// A subsite result below the supply factor is refused instead of being raised.
func adtyApplyTeamProtocolPrice(c *gin.Context, userId int, groupRatio, marked, k float64) (float64, float64) {
	if c == nil || userId <= 0 {
		return marked, k
	}
	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if tokenId <= 0 {
		return marked, k
	}
	var binding model.AdtyTeamKey
	lookup := model.DB.Where("token_id = ? AND user_id = ? AND status = ?", tokenId, userId, model.AdtyTeamMemberActive).Limit(1).Find(&binding)
	if lookup.Error != nil || lookup.RowsAffected == 0 {
		return marked, k
	}
	var team model.AdtyTeam
	if err := model.DB.First(&team, binding.TeamId).Error; err != nil || team.Status != model.AdtyTeamStatusActive || !team.Discount.IsPositive() {
		return marked, k
	}
	factor := team.Discount.InexactFloat64()
	if math.IsNaN(factor) || math.IsInf(factor, 0) || factor <= 0 || factor > 1 {
		c.Set(AdtyTeamPriceBlockedKey, true)
		return marked, k
	}
	if d := c.GetFloat64(AdtySiteCostFactorContextKey); d > 0 && groupRatio > 0 && marked*factor < groupRatio*d {
		c.Set(AdtyTeamPriceBlockedKey, true)
		return marked, k
	}
	next := marked * factor
	if math.IsNaN(next) || math.IsInf(next, 0) || next < 0 {
		c.Set(AdtyTeamPriceBlockedKey, true)
		return marked, k
	}
	c.Set(AdtyTeamDiscountContextKey, factor)
	return next, k
}

// AdtyTeamPriceBlocked reports a team price that would undercut the site supply price.
func AdtyTeamPriceBlocked(c *gin.Context) bool {
	return c != nil && c.GetBool(AdtyTeamPriceBlockedKey)
}

// AdtyApplySiteMarkupHook 是 HandleGroupRatio 调用的入口（relay/helper/price.go），测试可以替换。
var AdtyApplySiteMarkupHook = AdtyApplySiteMarkup

// AdtySiteMarkupFromContext 取同一个请求里记下的 k 和 d；没有时 ok 为 false。
func AdtySiteMarkupFromContext(c *gin.Context) (k, d float64, ok bool) {
	if c == nil {
		return 0, 0, false
	}
	k = c.GetFloat64(AdtySiteMarkupContextKey)
	d = c.GetFloat64(AdtySiteCostFactorContextKey)
	return k, d, k >= 1 && d > 0
}

// AdtyAppendSiteMarkupAdminInfo 把 k、d 记进日志的 admin_info，后台核对用；普通用户的日志投影看不到 admin_info。
func AdtyAppendSiteMarkupAdminInfo(c *gin.Context, other *model.LogOther) {
	k, d, ok := AdtySiteMarkupFromContext(c)
	if !ok || other == nil {
		return
	}
	other.SetAdmin("site_markup", k)
	other.SetAdmin("site_cost_factor", d)
	if siteId := c.GetInt(AdtySiteIdContextKey); siteId > 0 {
		other.SetAdmin("site_id", siteId)
		other.SetAdmin("site_owner_user_id", c.GetInt(AdtySiteOwnerContextKey))
	}
}

// AdtySitePricingInput 是站长保存定价的请求：默认 k、按分组的 k，带乐观锁版本。
type AdtySitePricingInput struct {
	SiteId          int
	UserId          int // 0 = 全站定价；否则是这个客户的单独定价
	DefaultMarkup   *decimal.Decimal
	GroupMarkups    map[string]decimal.Decimal // 值为 0 表示删掉这个分组的单独价
	ExpectedVersion int64
	Now             int64
}

// AdtySitePricingResult 说明保存后哪些项是立即生效、哪些在等涨价延迟。
type AdtySitePricingResult struct {
	Version   int64                 `json:"version"`
	Immediate []string              `json:"immediate"`
	Pending   map[string]int64      `json:"pending"` // 分组（空串是默认）→ 生效时间
	Prices    []model.AdtySitePrice `json:"prices"`
	Raised    []model.AdtySitePrice `json:"-"`
	Site      *model.AdtySite       `json:"-"`
}

// adtySiteMaxMarkup 是这个站的 k 上限：单站和全局都设了就取较小的那个。
func adtySiteMaxMarkup(site *model.AdtySite) (decimal.Decimal, bool) {
	cfg := resellerSetting()
	var limit decimal.Decimal
	set := false
	if site != nil && site.MaxMarkup != nil {
		limit = *site.MaxMarkup
		set = true
	}
	if site != nil && site.TierMaxMarkup != nil {
		tierLimit := *site.TierMaxMarkup
		if !set || tierLimit.LessThan(limit) {
			limit = tierLimit
		}
		set = true
	}
	if cfg.MaxMarkup != nil {
		global := decimal.NewFromFloat(*cfg.MaxMarkup)
		if !set || global.LessThan(limit) {
			limit = global
		}
		set = true
	}
	if !set || !limit.IsPositive() {
		return decimal.Zero, false
	}
	return limit, true
}

// adtyClampMarkup 读取时再按当前上限截一次。管理员调低上限后，已经保存的更高倍率按新上限扣费。
func adtyClampMarkup(site *model.AdtySite, k decimal.Decimal, userId int, group string) decimal.Decimal {
	limit, ok := adtySiteMaxMarkup(site)
	if !ok || !k.GreaterThan(limit) {
		return k
	}
	common.SysError(common.LogText("site markup %s for user %d group %s is above the current limit %s", k.String(), userId, group, limit.String()))
	return limit
}

// SaveAdtySitePricing 保存站长定价。规则（11.4）：
//   - 1 ≤ k ≤ 上限，否则拒绝；站点 paused 时不能改。
//   - 第一次设置、或者新值 ≤ 现值：立即生效，同时清掉待生效的涨价。
//   - 新值 > 现值：写进 PendingMarkup，markup_increase_delay_hours 之后由主节点任务生效。
//   - 版本号是这个站（或这个客户）所有定价行的最大 Version，保存时带 expected_version，不一致就拒绝。
func SaveAdtySitePricing(input AdtySitePricingInput) (*AdtySitePricingResult, error) {
	now := input.Now
	if now == 0 {
		now = time.Now().Unix()
	}
	delay := int64(max(resellerSetting().MarkupIncreaseDelayHours, 0)) * 3600
	result := &AdtySitePricingResult{Pending: map[string]int64{}}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var site model.AdtySite
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", input.SiteId).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdtySiteNotFound
			}
			return err
		}
		switch site.Status {
		case model.AdtySiteStatusActive:
		case model.AdtySiteStatusPaused:
			return ErrAdtySitePaused
		default:
			return ErrAdtySiteStatus
		}
		limit, ok := adtySiteMaxMarkup(&site)
		if !ok {
			return ErrAdtySiteDisabled
		}
		changes := map[string]decimal.Decimal{}
		if input.DefaultMarkup != nil {
			changes[""] = *input.DefaultMarkup
		}
		for group, markup := range input.GroupMarkups {
			group = strings.TrimSpace(group)
			if group == "" || len(group) > 64 {
				return common.NewMessage("Invalid parameters")
			}
			changes[group] = markup
		}
		for group, markup := range changes {
			if markup.IsZero() && group != "" {
				continue
			}
			if markup.LessThan(decimal.NewFromInt(1)) || markup.GreaterThan(limit) {
				return common.NewMessage("The markup must be between 1 and {{max}}", map[string]any{"max": limit.String()})
			}
		}
		var rows []model.AdtySitePrice
		if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND user_id = ?", site.Id, input.UserId).Find(&rows).Error; err != nil {
			return err
		}
		var version int64
		existing := map[string]model.AdtySitePrice{}
		for _, row := range rows {
			version = max(version, row.Version)
			existing[row.Group] = row
		}
		if version != input.ExpectedVersion {
			return ErrAdtySitePricingConflict
		}
		var customerCount int64
		if err := tx.Model(&model.AdtySiteCustomer{}).Where("site_id = ?", site.Id).Count(&customerCount).Error; err != nil {
			return err
		}
		next := version + 1
		for group, markup := range changes {
			row, found := existing[group]
			if markup.IsZero() {
				if !found {
					continue
				}
				// 删掉分组单独价等于改用上一级。上一级不更高，或站上还没有客户，就马上删。
				// 否则先留着低价，到期再删这一行，之后跟着上一级变。
				var others []model.AdtySitePrice
				if err := tx.Where("site_id = ? AND id <> ?", site.Id, row.Id).Find(&others).Error; err != nil {
					return err
				}
				fallback := ResolveAdtySiteMarkup(others, input.UserId, group)
				if !customerCountPositive(customerCount) || fallback.LessThanOrEqual(row.Markup) {
					if err := tx.Where("id = ?", row.Id).Delete(&model.AdtySitePrice{}).Error; err != nil {
						return err
					}
					result.Immediate = append(result.Immediate, group)
					continue
				}
				// 到期后删掉这一行，之后跟着上一级变，不把删除当时的高价冻住。
				row.PendingMarkup, row.PendingAt = decimal.Zero, now+delay
				if err := tx.Model(&model.AdtySitePrice{}).Where("id = ?", row.Id).Updates(map[string]any{
					"pending_markup": decimal.Zero, "pending_at": now + delay, "version": next, "updated_at": now,
				}).Error; err != nil {
					return err
				}
				result.Pending[group] = now + delay
				notice := row
				notice.PendingMarkup = fallback
				result.Raised = append(result.Raised, notice)
				continue
			}
			if !found {
				current := decimal.NewFromInt(1)
				if customerCountPositive(customerCount) {
					var others []model.AdtySitePrice
					if err := tx.Where("site_id = ? AND user_id = ?", site.Id, input.UserId).Find(&others).Error; err != nil {
						return err
					}
					current = ResolveAdtySiteMarkup(others, input.UserId, group)
				}
				row = model.AdtySitePrice{SiteId: site.Id, UserId: input.UserId, Group: group, Version: next, UpdatedAt: now}
				if customerCountPositive(customerCount) && markup.GreaterThan(current) {
					row.Markup, row.PendingMarkup, row.PendingAt = current, markup, now+delay
					if err := tx.Create(&row).Error; err != nil {
						return err
					}
					result.Pending[group] = now + delay
					result.Raised = append(result.Raised, row)
					continue
				}
				row.Markup = markup
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
				result.Immediate = append(result.Immediate, group)
				continue
			}
			updates := map[string]any{"version": next, "updated_at": now}
			if markup.LessThanOrEqual(row.Markup) {
				updates["markup"], updates["pending_markup"], updates["pending_at"] = markup, decimal.Zero, 0
				result.Immediate = append(result.Immediate, group)
			} else {
				updates["pending_markup"], updates["pending_at"] = markup, now+delay
				result.Pending[group] = now + delay
				row.PendingMarkup, row.PendingAt = markup, now+delay
				result.Raised = append(result.Raised, row)
			}
			if err := tx.Model(&model.AdtySitePrice{}).Where("id = ?", row.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		// 没有变更的行也要把版本推上去，这样下次带旧版本保存会冲突。
		if err := tx.Model(&model.AdtySitePrice{}).Where("site_id = ? AND user_id = ?", site.Id, input.UserId).Update("version", next).Error; err != nil {
			return err
		}
		result.Version, result.Site = next, &site
		return tx.Where("site_id = ? AND user_id = ?", site.Id, input.UserId).Order("id").Find(&result.Prices).Error
	})
	if err != nil {
		return nil, err
	}
	InvalidateAdtySitePriceCache()
	if len(result.Raised) > 0 && AdtyOnSiteMarkupIncrease != nil {
		AdtyOnSiteMarkupIncrease(result.Site, input.UserId, result.Raised)
	}
	return result, nil
}

// AdtyOnSiteMarkupIncrease 在保存了待生效的涨价后调用（11.3 给受影响的客户发邮件，按客户所属站点的品牌）。
// userId 为 0 表示全站定价，影响这个站所有没有单独定价的客户。
var AdtyOnSiteMarkupIncrease func(site *model.AdtySite, userId int, raised []model.AdtySitePrice)

// ApplyDueAdtySiteMarkups 把到期的涨价改成生效（主节点任务调用）。站点 paused 时也照常生效：暂停前定下的涨价已经通知过客户。
func customerCountPositive(count int64) bool { return count > 0 }

func ApplyDueAdtySiteMarkups(now int64) (int64, error) {
	result := model.DB.Model(&model.AdtySitePrice{}).
		Where("pending_markup > ? AND pending_at > 0 AND pending_at <= ?", 0, now).
		Updates(map[string]any{
			"markup": gorm.Expr("pending_markup"), "pending_markup": 0, "pending_at": 0,
			"version": gorm.Expr("version + 1"), "updated_at": now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	removed := model.DB.Where("pending_markup = ? AND pending_at > 0 AND pending_at <= ?", 0, now).Delete(&model.AdtySitePrice{})
	if removed.Error != nil {
		return 0, removed.Error
	}
	changed := result.RowsAffected + removed.RowsAffected
	if changed > 0 {
		InvalidateAdtySitePriceCache()
	}
	return changed, nil
}

// AssignAdtySiteCostTiers 按上个月每个站的客户消费额取档（cost_factor_tiers 设了才启用），写进 TierCostFactor。
// revenue 是「站点 id → 上月客户消费额（美元）」，由 11.5 的日汇总算出；month 形如 2026-10。
// 返回 d 变了的站，由调用方通知站长。
func AssignAdtySiteCostTiers(month string, revenue map[int]decimal.Decimal) ([]model.AdtySite, error) {
	tiers := resellerSetting().CostFactorTiers
	if len(tiers) == 0 {
		return nil, nil
	}
	var sites []model.AdtySite
	if err := model.DB.Where("status IN ?", []string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused, model.AdtySiteStatusClosing}).
		Where("(tier_mode = ? OR tier_mode IS NULL) AND (tier_code = ? OR tier_code IS NULL)", "", "").Find(&sites).Error; err != nil {
		return nil, err
	}
	var changed []model.AdtySite
	for _, site := range sites {
		amount := revenue[site.Id]
		var picked *decimal.Decimal
		best := decimal.NewFromInt(-1)
		for _, tier := range tiers {
			threshold := decimal.NewFromFloat(tier.MinMonthlyUSD)
			if threshold.LessThanOrEqual(amount) && threshold.GreaterThan(best) {
				best = threshold
				factor := decimal.NewFromFloat(tier.CostFactor)
				picked = &factor
			}
		}
		current, hasCurrent := AdtySiteCostFactor(&site)
		raise := picked != nil && hasCurrent && picked.GreaterThan(current)
		updates := map[string]any{"tier_month": month}
		if raise {
			delay := resellerSetting().MarkupIncreaseDelayHours
			if delay < 1 {
				delay = 24
			}
			pendingAt := time.Now().Unix() + int64(delay)*3600
			updates["pending_tier_cost_factor"] = *picked
			updates["pending_tier_cost_factor_at"] = pendingAt
		} else {
			if picked == nil {
				updates["tier_cost_factor"] = nil
			} else {
				updates["tier_cost_factor"] = *picked
			}
			updates["pending_tier_cost_factor"] = gorm.Expr("NULL")
			updates["pending_tier_cost_factor_at"] = 0
		}
		if err := model.DB.Model(&model.AdtySite{}).Where("id = ?", site.Id).
			Where("(tier_mode = ? OR tier_mode IS NULL) AND (tier_code = ? OR tier_code IS NULL)", "", "").Updates(updates).Error; err != nil {
			return nil, err
		}
		if raise {
			notice := site
			notice.PendingCostFactor = picked
			notice.PendingCostFactorAt = updates["pending_tier_cost_factor_at"].(int64)
			adtyNotifySiteCostFactorPending(&notice)
			continue
		}
		before := site.TierCostFactor
		if (before == nil) != (picked == nil) || (before != nil && picked != nil && !before.Equal(*picked)) {
			site.TierCostFactor, site.TierMonth = picked, month
			changed = append(changed, site)
		}
	}
	InvalidateAdtySiteCache()
	return changed, nil
}

// AdtyStampTaskSiteBilling 在任务提交时把这次请求的 k、d 快照进 BillingContext（controller/relay.go）。
// 轮询结算和退款沿用快照，不受之后改 k、d 的影响。
func AdtyStampTaskSiteBilling(c *gin.Context, bc *model.TaskBillingContext) {
	if bc == nil {
		return
	}
	if k, d, ok := AdtySiteMarkupFromContext(c); ok {
		bc.SiteMarkup, bc.SiteCostFactor, bc.SiteId = k, d, c.GetInt(AdtySiteIdContextKey)
		bc.SiteOwnerUserId = c.GetInt(AdtySiteOwnerContextKey)
	}
}

// adtyTaskRecalcGroupRatio 是任务按 token 重算时的分组倍率（service/task_billing.go）：
// 提交时乘过 k 的任务（快照里 SiteMarkup ≥ 1）用快照里的分组倍率，已经含 k，不再乘；
// 主站客户的任务照上游用当前分组倍率；没有快照的分站任务按当前的 k 乘一次。
func adtyTaskRecalcGroupRatio(task *model.Task, group string, groupRatio float64) float64 {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.SiteMarkup >= 1 && bc.GroupRatio > 0 {
		return bc.GroupRatio
	}
	marked, _ := AdtyApplySiteMarkup(nil, task.UserId, group, groupRatio)
	return marked
}

func init() {
	model.AdtyOnConsumeLog = adtyOnConsumeLog
	adtyMasterTasks = append(adtyMasterTasks, startAdtySiteMarkupTask, startAdtySiteCostTierTask)
}

// adtyOnConsumeLog 在每条消费日志之前调用：记下 k、d（admin_info），再交给 11.5 的收益记账。
func adtyOnConsumeLog(c *gin.Context, userId int, params *model.RecordConsumeLogParams) {
	if params == nil {
		return
	}
	// 只有乘过 k 的请求才补 admin_info；Other 为 nil 的日志保持原样（上游按 nil 存空串）。
	if _, _, ok := AdtySiteMarkupFromContext(c); ok {
		if params.Other == nil {
			params.Other = model.NewLogOther()
		}
		AdtyAppendSiteMarkupAdminInfo(c, params.Other)
	}
	if c != nil {
		if discount := c.GetFloat64(AdtyTeamDiscountContextKey); discount > 0 {
			if params.Other == nil {
				params.Other = model.NewLogOther()
			}
			params.Other.SetAdmin("team_discount", discount)
		}
	}
	adtyTakeBillingShortfall(c, params) // 余额下限：结算没扣到的差额记 admin_info
	if c.GetBool(adtyDurableBillingContextKey) {
		if params.Other == nil {
			params.Other = model.NewLogOther()
		}
		params.Other.SetAdmin("billing_operation_key", c.GetString(adtyBillingOperationContextKey))
	}
	if AdtyRecordConsumeEarning != nil {
		AdtyRecordConsumeEarning(c, userId, params)
	}
}

// AdtyRecordConsumeEarning 由 11.5 的收益记账注册。
var AdtyRecordConsumeEarning func(c *gin.Context, userId int, params *model.RecordConsumeLogParams)

// adtySiteMonthRevenue 是上个月每个站的客户消费额（美元）：usage 和 refund 行的 Revenue 合计，不算作废行。
func adtySiteMonthRevenue(month string) (map[int]decimal.Decimal, error) {
	start, err := time.ParseInLocation("2006-01", month, time.Local)
	if err != nil {
		return nil, err
	}
	var rows []model.AdtySiteEarningDaily
	err = model.DB.Select("site_id", "revenue").
		Where("kind IN ? AND status <> ? AND day >= ? AND day < ?",
			[]string{model.AdtySiteEarningUsage, model.AdtySiteEarningRefund}, model.AdtySiteEarningVoid,
			start.Format("2006-01-02"), start.AddDate(0, 1, 0).Format("2006-01-02")).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	revenue := map[int]decimal.Decimal{}
	for _, row := range rows {
		revenue[row.SiteId] = revenue[row.SiteId].Add(row.Revenue)
	}
	return revenue, nil
}

// RunAdtySiteCostTierAssignment 按上个月已记账消费给还没算过那个月的站分档。没有档位时什么都不写。
func RunAdtySiteCostTierAssignment(now time.Time) error {
	if len(resellerSetting().CostFactorTiers) == 0 {
		return nil
	}
	month := now.AddDate(0, -1, 0).Format("2006-01")
	var pending int64
	err := model.DB.Model(&model.AdtySite{}).
		Where("status IN ? AND tier_month <> ?", []string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused, model.AdtySiteStatusClosing}, month).
		Count(&pending).Error
	if err != nil || pending == 0 {
		return err
	}
	revenue, err := adtySiteMonthRevenue(month)
	if err != nil {
		return err
	}
	_, err = AssignAdtySiteCostTiers(month, revenue)
	return err
}

// startAdtySiteCostTierTask 在主节点每小时检查一次：上个月还没分档的站，按已记账消费补算。
func startAdtySiteCostTierTask() {
	go func() {
		run := func() {
			if err := RunAdtySiteCostTierAssignment(time.Now()); err != nil {
				common.SysError(common.LogText("assign site cost tiers: %s", err.Error()))
			}
		}
		run()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()
}

// startAdtySiteMarkupTask 在主节点每 5 分钟把到期的涨价改成生效。
func startAdtySiteMarkupTask() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := ApplyDueAdtySiteMarkups(time.Now().Unix()); err != nil {
				common.SysError(common.LogText("apply site markups: %s", err.Error()))
			}
			if _, err := ApplyDueAdtySiteCostFactors(time.Now().Unix()); err != nil {
				common.SysError(common.LogText("apply site cost factors: %s", err.Error()))
			}
			if err := ApplyDueGlobalCostFactor(time.Now().Unix()); err != nil {
				common.SysError(common.LogText("apply global cost factor: %s", err.Error()))
			}
			if _, err := ApplyDueAdtySiteTierCostFactors(time.Now().Unix()); err != nil {
				common.SysError(common.LogText("apply site tier cost factors: %s", err.Error()))
			}
			if err := ApplyDueMinCostFactor(time.Now().Unix()); err != nil {
				common.SysError(common.LogText("apply minimum cost factor: %s", err.Error()))
			}
		}
	}()
}

// AdtyUserGroupRatio 是 /api/user/self/groups 显示的分组倍率：分站客户乘上自己的 k。
func AdtyUserGroupRatio(userId int, userGroup, group string) float64 {
	ratio := GetUserGroupRatio(userGroup, group)
	marked, _ := AdtyApplySiteMarkup(nil, userId, group, ratio)
	return marked
}

// AdtyApplySitePricing 在 /api/pricing 返回前调用：分站客户按自己的 k、分站访客按全站默认的 k 改分组倍率。
func AdtyApplySitePricing(c *gin.Context, groupRatio map[string]float64) {
	userId := c.GetInt("id")
	if userId > 0 {
		for group, ratio := range groupRatio {
			groupRatio[group], _ = AdtyApplySiteMarkup(nil, userId, group, ratio)
		}
		return
	}
	site := AdtySiteForRequest(c)
	if site == nil {
		return
	}
	for group, ratio := range groupRatio {
		groupRatio[group] = ratio * adtyMarkupFloat(AdtySiteVisitorMarkup(site, group), 0, group)
	}
}
