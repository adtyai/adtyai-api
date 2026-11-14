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

// 分站定价按 G.2：供货价 = 模型基础价格 × 分站供货倍率 × 本次实际计费分组倍率，
// 普通售价 = 供货价 × (1 + 分站加价比例)。加价比例为 0 时售价等于供货价。
// 这些因子只乘进分组倍率，按倍率、按次、阶梯、图片、任务提交、工具附加费、违规扣费都从这条路径走，
// 不在 HandleGroupRatio 的 Realtime 预扣和任务按 token 重算里再乘一次。
// 加价幅度不进 PriceData.OtherRatios（会出现在客户日志的计价参数里）。
//
// 存储字段 Markup / MaxMarkup 仍是历史上的 k 倍率（库默认 1，界面也按倍率录入）。
// 1 表示不加价，因此计费时 加价比例 = k - 1；保存值 1.2 是 20% 加价，不能当成 120%。
// CostFactor 是供货倍率，小于 1 的值要原样使用，不再把 k<1 回退成 1，也不再要求供货倍率 ≤ 1。

// gin 上下文里记下的仍是存储口径的 k 和供货倍率 d：同一个请求后面的消费日志都从这里取（11.5 的记账）。
const (
	AdtySiteMarkupContextKey      = "adty_site_markup"
	AdtyTeamDiscountContextKey    = "adty_team_discount"
	AdtyTeamPriceBlockedKey       = "adty_team_price_blocked"
	AdtySiteCostFactorContextKey  = "adty_site_cost_factor"
	AdtySiteBilledRatioContextKey = "adty_site_billed_ratio"
	AdtySiteIdContextKey          = "adty_site_id"
	AdtySiteOwnerContextKey       = "adty_site_owner_user_id"
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

func adtyNormalizeSitePriceModel(name string) string {
	return strings.TrimSpace(name)
}

func adtySitePriceKey(group, modelName string) string {
	return group + "" + adtyNormalizeSitePriceModel(modelName)
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

// AdtySiteMarkup 返回分站客户在这个分组、这个模型上的存储 k（加价比例 = k-1）；主站客户、站点没有开着时返回 1。只读缓存。
// modelName 为空时使用该客户/分组层级的默认加价。已经保存的价如果高于当前上限，按当前上限返回。
func AdtySiteMarkup(userId int, group, modelName string) decimal.Decimal {
	site := AdtyBillingSiteForUser(userId)
	if site == nil {
		return decimal.NewFromInt(1)
	}
	return adtyClampMarkup(site, ResolveAdtySiteMarkup(adtySitePrices(site.Id), userId, group, modelName), userId, group)
}

// AdtySiteVisitorMarkup 是分站访客（没登录）看价格页时的存储 k：全站 + 分组 → 全站 → 1，并套用该模型的独立加价（若有）。
func AdtySiteVisitorMarkup(site *model.AdtySite, group, modelName string) decimal.Decimal {
	if !AdtySiteServesCustomers(site) || !resellerSetting().Enabled {
		return decimal.NewFromInt(1)
	}
	return adtyClampMarkup(site, ResolveAdtySiteMarkup(adtySitePrices(site.Id), 0, group, modelName), 0, group)
}

// AdtySiteCostFactor 是站点当前的供货倍率：单站设的值 → 业绩档位 → 全局；低于 min_cost_factor 时用下限。
// 供货倍率是正的乘数，可以小于或大于 1。全局值没设时返回 false（这时分站功能本来就打不开）。
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
	if !d.IsPositive() {
		return decimal.Zero, false
	}
	return d, true
}

// adtyStoredMarkupRatio 把存储的 k 换成已确认的加价比例。历史字段是倍率，默认 1 表示不加价，所以比例 = k - 1。
func adtyStoredMarkupRatio(k decimal.Decimal) decimal.Decimal {
	return k.Sub(decimal.NewFromInt(1))
}

// adtyMarkupFloat 把存储的 k 转成 float 给上下文和快照用。NaN、Inf 或非正数按 1（不加价）处理并记日志；0<k<1 原样返回。
func adtyMarkupFloat(k decimal.Decimal, userId int, group string) float64 {
	return adtyValidStoredMarkup(k, userId, group).InexactFloat64()
}

func adtyDecimalFloatInvalid(d decimal.Decimal) bool {
	value := d.InexactFloat64()
	return math.IsNaN(value) || math.IsInf(value, 0)
}

func adtyValidStoredMarkup(k decimal.Decimal, userId int, group string) decimal.Decimal {
	if !k.IsPositive() || adtyDecimalFloatInvalid(k) {
		common.SysError(common.LogText("invalid site markup %s for user %d group %s, using 1", k.String(), userId, group))
		return decimal.NewFromInt(1)
	}
	return k
}

func adtyDecimalFiniteNonNegative(d decimal.Decimal) bool {
	value := d.InexactFloat64()
	return !math.IsNaN(value) && !math.IsInf(value, 0) && !d.IsNegative()
}

// adtySiteOrdinarySellRatioDec 用精确十进制计算：供货价因子 = 分组倍率 × 供货倍率，
// 普通售价因子 = 供货价因子 × (1 + 加价比例)。计费结果不再经过 float64 往返。
func adtySiteOrdinarySellRatioDec(groupRatio, k, costFactor decimal.Decimal, userId int, group string) (marked, storedK decimal.Decimal, ok bool) {
	storedK = k
	if !storedK.IsPositive() || adtyDecimalFloatInvalid(storedK) {
		storedK = decimal.NewFromInt(1)
	}
	if !costFactor.IsPositive() {
		common.SysError(common.LogText("invalid site cost factor %s for user %d group %s, leaving group ratio unchanged", costFactor.String(), userId, group))
		return groupRatio, decimal.NewFromInt(1), false
	}
	marked = groupRatio.Mul(costFactor).Mul(decimal.NewFromInt(1).Add(adtyStoredMarkupRatio(storedK)))
	if !adtyDecimalFiniteNonNegative(marked) {
		common.SysError(common.LogText("site sell price overflow for user %d group %s, leaving group ratio unchanged", userId, group))
		return groupRatio, decimal.NewFromInt(1), false
	}
	return marked, storedK, true
}

func adtyRememberBilledRatio(c *gin.Context, billed decimal.Decimal) {
	if c != nil {
		c.Set(AdtySiteBilledRatioContextKey, billed)
	}
}

// AdtyBilledGroupRatioFromContext 取出本次请求按精确十进制算出的计费分组倍率。
func AdtyBilledGroupRatioFromContext(c *gin.Context) (decimal.Decimal, bool) {
	if c == nil {
		return decimal.Zero, false
	}
	value, ok := c.Get(AdtySiteBilledRatioContextKey)
	if !ok {
		return decimal.Zero, false
	}
	billed, ok := value.(decimal.Decimal)
	return billed, ok
}

// AdtyApplySiteMarkupDecimal 按已确认公式用精确十进制计算供货价、售价和团队下限。
// 返回最终计费分组倍率和存储 k（1 表示加价比例为 0）。
func AdtyApplySiteMarkupDecimal(c *gin.Context, userId int, group string, groupRatio decimal.Decimal, modelName string) (billed, storedK decimal.Decimal) {
	marked, storedK := adtyApplySiteMarkupOnlyDecimal(c, userId, group, groupRatio, modelName)
	billed, storedK = adtyApplyTeamProtocolPriceDecimal(c, userId, groupRatio, marked, storedK)
	adtyRememberBilledRatio(c, billed)
	return billed, storedK
}

// AdtyApplySiteMarkup 在算出分组倍率之后调用：按供货倍率和加价比例改分组倍率，并把 k、d 写进 gin 上下文。
// 返回普通售价对应的分组倍率和存储 k（1 表示加价比例为 0）。上游的 HandleGroupRatio、Realtime 预扣都调它。
// 财务权威结果走 AdtyApplySiteMarkupDecimal；这里的 float64 仅留给仍使用浮点的调用方和快照字段。
func AdtyApplySiteMarkup(c *gin.Context, userId int, group string, groupRatio float64, modelName string) (float64, float64) {
	billed, storedK := AdtyApplySiteMarkupDecimal(c, userId, group, decimal.NewFromFloat(groupRatio), modelName)
	return billed.InexactFloat64(), storedK.InexactFloat64()
}

func adtyApplySiteMarkupOnlyDecimal(c *gin.Context, userId int, group string, groupRatio decimal.Decimal, modelName string) (decimal.Decimal, decimal.Decimal) {
	one := decimal.NewFromInt(1)
	factors := adtyDefaultPriceFactors(group, groupRatio)
	site := AdtyBillingSiteForUser(userId)
	if site == nil {
		adtyRememberPriceFactors(c, factors)
		return groupRatio, one
	}
	factors.MainSite = false
	factors.SiteId = site.Id
	kDec := adtyClampMarkup(site, ResolveAdtySiteMarkup(adtySitePrices(site.Id), userId, group, modelName), userId, group)
	kUsed := adtyValidStoredMarkup(kDec, userId, group)
	k := kUsed.InexactFloat64()
	d, dOK := AdtySiteCostFactor(site)
	if c != nil {
		c.Set(AdtySiteMarkupContextKey, k)
		c.Set(AdtySiteIdContextKey, site.Id)
		ownerID, err := model.CurrentAdtySiteFinancialOwnerTx(model.DB, site.Id)
		if err != nil {
			common.SysError("Request earning owner snapshot unavailable")
		}
		c.Set(AdtySiteOwnerContextKey, ownerID)
		factors.SiteOwnerUserId = ownerID
		if dOK {
			c.Set(AdtySiteCostFactorContextKey, d.InexactFloat64())
		}
	}
	if !dOK {
		common.SysError(common.LogText("site cost factor missing for user %d group %s, leaving group ratio unchanged", userId, group))
		adtyRememberPriceFactors(c, factors)
		return groupRatio, one
	}
	marked, storedK, ok := adtySiteOrdinarySellRatioDec(groupRatio, kUsed, d, userId, group)
	if !ok {
		adtyRememberPriceFactors(c, factors)
		return marked, storedK
	}
	factors.StoredK = storedK
	factors.MarkupRatio = adtyStoredMarkupRatio(storedK)
	factors.CostFactor = d
	factors.SupplyPrice = groupRatio.Mul(d)
	factors.SellPrice = marked
	adtyRememberPriceFactors(c, factors)
	return marked, storedK
}

// adtyApplyTeamProtocolPriceDecimal 按已确认口径把团队折后支付比例乘进分站普通售价：
// 分站团队折扣计算价 = 分站普通售价 × 分站团队折后支付比例（0.8 表示支付普通售价的 80%），
// 分站团队最终价 = max(分站供货价, 分站团队折扣计算价)。
// 供货价沿用 ISS-R1-0009：分组倍率 × 供货倍率。主站团队没有供货价下限（G.2-5）。
// 支付比例无效或结果溢出时仍置 AdtyTeamPriceBlockedKey；折后价低于供货价时改为按供货价计费，不再拒绝。
func adtyApplyTeamProtocolPriceDecimal(c *gin.Context, userId int, groupRatio, marked, storedK decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	if c == nil || userId <= 0 {
		return marked, storedK
	}
	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if tokenId <= 0 || model.DB == nil || !adtyTeamTableReadyFor(model.DB, &model.AdtyTeamKey{}) {
		return marked, storedK
	}
	var binding model.AdtyTeamKey
	lookup := model.DB.Where("token_id = ? AND user_id = ? AND status = ?", tokenId, userId, model.AdtyTeamMemberActive).Limit(1).Find(&binding)
	if lookup.Error != nil || lookup.RowsAffected == 0 {
		return marked, storedK
	}
	var team model.AdtyTeam
	if err := model.DB.First(&team, binding.TeamId).Error; err != nil || team.Status != model.AdtyTeamStatusActive || !team.Discount.IsPositive() {
		return marked, storedK
	}
	factorDec := team.Discount
	if !factorDec.IsPositive() || factorDec.GreaterThan(decimal.NewFromInt(1)) || adtyDecimalFloatInvalid(factorDec) {
		c.Set(AdtyTeamPriceBlockedKey, true)
		return marked, storedK
	}
	discounted := marked.Mul(factorDec)
	nextDec := discounted
	var supply decimal.Decimal
	floorApplied := false
	// 只有分站团队才有供货价下限；主站团队即使有很深的折扣也不抬价。
	if team.SiteId > 0 {
		if site := AdtyBillingSiteForUser(userId); site != nil {
			if d, ok := AdtySiteCostFactor(site); ok && groupRatio.IsPositive() {
				supply = groupRatio.Mul(d)
				nextDec = decimal.Max(supply, discounted)
				floorApplied = true
			}
		}
	}
	if !adtyDecimalFiniteNonNegative(nextDec) {
		c.Set(AdtyTeamPriceBlockedKey, true)
		return marked, storedK
	}
	c.Set(AdtyTeamDiscountContextKey, factorDec.InexactFloat64())
	factors, ok := adtyPriceFactorsFromContext(c)
	if !ok {
		factors = adtyDefaultPriceFactors("", groupRatio)
		factors.SellPrice = marked
		factors.StoredK = storedK
		factors.MarkupRatio = adtyStoredMarkupRatio(storedK)
	}
	factors.TeamApplied = true
	factors.TeamPayRatio = factorDec
	if floorApplied {
		factors.FloorApplied = true
		factors.SupplyFloor = supply
	}
	adtyRememberPriceFactors(c, factors)
	return nextDec, storedK
}

// AdtyTeamPriceBlocked 表示本次请求的团队支付比例无效或折后价溢出，计费应拒绝。
// 折后价低于供货价不再走这条拒绝路径。
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
	return k, d, k > 0 && d > 0
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

// AdtySitePricingInput 是站长保存定价的请求：默认 k、按分组的 k、按模型的独立 k，带乐观锁版本。
type AdtySitePricingInput struct {
	SiteId          int
	UserId          int // 0 = 全站定价；否则是这个客户的单独定价
	DefaultMarkup   *decimal.Decimal
	GroupMarkups    map[string]decimal.Decimal // 值为 0 表示删掉这个分组的单独价
	ModelMarkups    map[string]decimal.Decimal // 值为 0 表示删掉这个模型的独立加价，恢复全局
	ExpectedVersion int64
	Now             int64
}

// AdtySitePricingResult 说明保存后哪些项是立即生效、哪些在等涨价延迟。
type AdtySitePricingResult struct {
	Version        int64                 `json:"version"`
	Immediate      []string              `json:"immediate"`
	Pending        map[string]int64      `json:"pending"` // 分组（空串是默认）→ 生效时间
	ModelImmediate []string              `json:"model_immediate,omitempty"`
	ModelPending   map[string]int64      `json:"model_pending,omitempty"`
	Prices         []model.AdtySitePrice `json:"prices"`
	Raised         []model.AdtySitePrice `json:"-"`
	Site           *model.AdtySite       `json:"-"`
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
//   - k 必须为正数且不超过上限（k 是存储倍率，加价比例 = k-1；0 仍表示删掉分组单独价或单模型独立加价）；站点 paused 时不能改。
//   - 删除单模型独立加价立即生效，该模型随后继承全局加价，并清掉进程内定价缓存（SPRC-005）。
//   - 第一次设置、或者新值 ≤ 现值：立即生效，同时清掉待生效的涨价。
//   - 新值 > 现值：写进 PendingMarkup，markup_increase_delay_hours 之后由主节点任务生效。
//   - 版本号是这个站（或这个客户）所有定价行的最大 Version，保存时带 expected_version，不一致就拒绝。
func SaveAdtySitePricing(input AdtySitePricingInput) (*AdtySitePricingResult, error) {
	now := input.Now
	if now == 0 {
		now = time.Now().Unix()
	}
	delay := int64(max(resellerSetting().MarkupIncreaseDelayHours, 0)) * 3600
	result := &AdtySitePricingResult{Pending: map[string]int64{}, ModelPending: map[string]int64{}}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var site model.AdtySite
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", input.SiteId).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdtySiteNotFound
			}
			return err
		}
		switch site.Status {
		case model.AdtySiteStatusActive, model.AdtySiteStatusPaused:
			// leftover paused rows are not a live pause mode; pricing stays editable
		default:
			return ErrAdtySiteStatus
		}
		limit, ok := adtySiteMaxMarkup(&site)
		if !ok {
			return ErrAdtySiteDisabled
		}
		type priceChange struct {
			group, model string
			markup       decimal.Decimal
		}
		var changes []priceChange
		if input.DefaultMarkup != nil {
			changes = append(changes, priceChange{markup: *input.DefaultMarkup})
		}
		for group, markup := range input.GroupMarkups {
			group = strings.TrimSpace(group)
			if group == "" || len(group) > 64 {
				return common.NewMessage("Invalid parameters")
			}
			changes = append(changes, priceChange{group: group, markup: markup})
		}
		for modelName, markup := range input.ModelMarkups {
			modelName = adtyNormalizeSitePriceModel(modelName)
			if modelName == "" || len(modelName) > 191 {
				return common.NewMessage("Invalid parameters")
			}
			changes = append(changes, priceChange{model: modelName, markup: markup})
		}
		for _, change := range changes {
			if change.markup.IsZero() && (change.group != "" || change.model != "") {
				continue
			}
			if !change.markup.IsPositive() || change.markup.GreaterThan(limit) {
				return common.NewMessage("The markup must be greater than 0 and at most {{max}}", map[string]any{"max": limit.String()})
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
			existing[adtySitePriceKey(row.Group, row.Model)] = row
		}
		if version != input.ExpectedVersion {
			return ErrAdtySitePricingConflict
		}
		var customerCount int64
		if err := tx.Model(&model.AdtySiteCustomer{}).Where("site_id = ?", site.Id).Count(&customerCount).Error; err != nil {
			return err
		}
		next := version + 1
		recordImmediate := func(change priceChange) {
			if change.model != "" {
				result.ModelImmediate = append(result.ModelImmediate, change.model)
				return
			}
			result.Immediate = append(result.Immediate, change.group)
		}
		recordPending := func(change priceChange, at int64) {
			if change.model != "" {
				result.ModelPending[change.model] = at
				return
			}
			result.Pending[change.group] = at
		}
		for _, change := range changes {
			group, modelName, markup := change.group, change.model, change.markup
			row, found := existing[adtySitePriceKey(group, modelName)]
			if markup.IsZero() {
				if !found {
					continue
				}
				// SPRC-005: 删除单模型独立加价必须立刻恢复全局加价，不能留下缓存价。
				if modelName != "" {
					if err := tx.Where("id = ?", row.Id).Delete(&model.AdtySitePrice{}).Error; err != nil {
						return err
					}
					recordImmediate(change)
					continue
				}
				// 删掉分组单独价等于改用上一级。上一级不更高，或站上还没有客户，就马上删。
				// 否则先留着低价，到期再删这一行，之后跟着上一级变。
				var others []model.AdtySitePrice
				if err := tx.Where("site_id = ? AND id <> ?", site.Id, row.Id).Find(&others).Error; err != nil {
					return err
				}
				fallback := ResolveAdtySiteMarkup(others, input.UserId, group, modelName)
				if !customerCountPositive(customerCount) || fallback.LessThanOrEqual(row.Markup) {
					if err := tx.Where("id = ?", row.Id).Delete(&model.AdtySitePrice{}).Error; err != nil {
						return err
					}
					recordImmediate(change)
					continue
				}
				// 到期后删掉这一行，之后跟着上一级变，不把删除当时的高价冻住。
				row.PendingMarkup, row.PendingAt = decimal.Zero, now+delay
				if err := tx.Model(&model.AdtySitePrice{}).Where("id = ?", row.Id).Updates(map[string]any{
					"pending_markup": decimal.Zero, "pending_at": now + delay, "version": next, "updated_at": now,
				}).Error; err != nil {
					return err
				}
				recordPending(change, now+delay)
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
					current = ResolveAdtySiteMarkup(others, input.UserId, group, modelName)
				}
				row = model.AdtySitePrice{SiteId: site.Id, UserId: input.UserId, Group: group, Model: modelName, Version: next, UpdatedAt: now}
				if customerCountPositive(customerCount) && markup.GreaterThan(current) {
					row.Markup, row.PendingMarkup, row.PendingAt = current, markup, now+delay
					if err := tx.Create(&row).Error; err != nil {
						return err
					}
					recordPending(change, now+delay)
					result.Raised = append(result.Raised, row)
					continue
				}
				row.Markup = markup
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
				recordImmediate(change)
				continue
			}
			updates := map[string]any{"version": next, "updated_at": now}
			if markup.LessThanOrEqual(row.Markup) {
				updates["markup"], updates["pending_markup"], updates["pending_at"] = markup, decimal.Zero, 0
				recordImmediate(change)
			} else {
				updates["pending_markup"], updates["pending_at"] = markup, now+delay
				recordPending(change, now+delay)
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

// AdtyStampTaskSiteBilling 在任务提交时把这次请求的完整 G.2-11 快照和兼容 k、d 写进 BillingContext。
// 轮询结算和退款沿用快照，不受之后改价的影响。
func AdtyStampTaskSiteBilling(c *gin.Context, bc *model.TaskBillingContext) {
	if bc == nil {
		return
	}
	if k, d, ok := AdtySiteMarkupFromContext(c); ok {
		bc.SiteMarkup, bc.SiteCostFactor, bc.SiteId = k, d, c.GetInt(AdtySiteIdContextKey)
		bc.SiteOwnerUserId = c.GetInt(AdtySiteOwnerContextKey)
	}
	if snap := AdtyFrozenPriceSnapshot(c); snap != nil {
		bc.PriceSnapshot = snap
		if bc.SiteId == 0 && snap.SiteId > 0 {
			bc.SiteId, bc.SiteOwnerUserId = snap.SiteId, snap.SiteOwnerUserId
		}
		if bc.SiteMarkup == 0 {
			bc.SiteMarkup = snap.LegacyMarkupFloat()
		}
		if bc.SiteCostFactor == 0 {
			bc.SiteCostFactor = snap.LegacyCostFactorFloat()
		}
	}
}

// adtyTaskRecalcGroupRatio 是任务按 token 重算时的分组倍率（service/task_billing.go）：
// 提交时已经按供货倍率和加价比例乘进快照的任务（SiteMarkup > 0）用快照里的分组倍率，不再乘；
// 主站客户的任务照上游用当前分组倍率；没有快照的分站任务按当前公式乘一次。
func adtyTaskRecalcGroupRatio(task *model.Task, group string, groupRatio float64) float64 {
	return adtyTaskRecalcGroupRatioDecimal(task, group, decimal.NewFromFloat(groupRatio)).InexactFloat64()
}

func adtyTaskRecalcGroupRatioDecimal(task *model.Task, group string, groupRatio decimal.Decimal) decimal.Decimal {
	if task != nil {
		if bc := task.PrivateData.BillingContext; bc != nil {
			if snap := bc.PriceSnapshot; snap != nil {
				if billed, ok := snap.BilledGroupRatioDecimal(); ok {
					return billed
				}
			}
			// Old rows: keep the previously stored billed ratio and do not
			// reprice from live site markup / cost factor / team pay.
			if bc.GroupRatio > 0 {
				return decimal.NewFromFloat(bc.GroupRatio)
			}
		}
	}
	modelName := ""
	if task != nil {
		modelName = adtyNormalizeSitePriceModel(task.Properties.OriginModelName)
	}
	billed, _ := AdtyApplySiteMarkupDecimal(nil, task.UserId, group, groupRatio, modelName)
	return billed
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

// AdtyUserGroupRatio 是 /api/user/self/groups 显示的分组倍率：分站客户按供货倍率和加价比例计算。
func AdtyUserGroupRatio(userId int, userGroup, group string) float64 {
	ratio := GetUserGroupRatio(userGroup, group)
	marked, _ := AdtyApplySiteMarkup(nil, userId, group, ratio, "")
	return marked
}

// AdtyApplySitePricing 在 /api/pricing 返回前调用：分站客户按自己的价、分站访客按全站默认价改分组倍率。
// 分组倍率按空模型（全局）加价计算；单模型独立加价由 AdtyAdjustPricingForSiteModels 乘进各模型的标价/倍率。
func AdtyApplySitePricing(c *gin.Context, groupRatio map[string]float64) {
	userId := c.GetInt("id")
	if userId > 0 {
		for group, ratio := range groupRatio {
			groupRatio[group], _ = AdtyApplySiteMarkup(nil, userId, group, ratio, "")
		}
		return
	}
	site := AdtySiteForRequest(c)
	if site == nil {
		return
	}
	d, ok := AdtySiteCostFactor(site)
	if !ok {
		return
	}
	for group, ratio := range groupRatio {
		marked, _, _ := adtySiteOrdinarySellRatioDec(decimal.NewFromFloat(ratio), AdtySiteVisitorMarkup(site, group, ""), d, 0, group)
		groupRatio[group] = marked.InexactFloat64()
	}
}

// AdtyAdjustPricingForSiteModels 返回价格列表的副本，把每个模型的标价和倍率按
// 该模型有效 k 相对同一分组空模型 k 的比值放大或缩小。group_ratio 已经乘过空模型加价，
// 这样公开价格页显示的是该模型的有效售价。不改动缓存中的原始切片。
func AdtyAdjustPricingForSiteModels(c *gin.Context, group string, pricing []model.Pricing) []model.Pricing {
	if c == nil || len(pricing) == 0 {
		return pricing
	}
	userId := c.GetInt("id")
	var site *model.AdtySite
	if userId > 0 {
		site = AdtyBillingSiteForUser(userId)
	} else {
		site = AdtySiteForRequest(c)
		userId = 0
	}
	if site == nil || !AdtySiteServesCustomers(site) || !resellerSetting().Enabled {
		return pricing
	}
	prices := adtySitePrices(site.Id)
	baseK := adtyClampMarkup(site, ResolveAdtySiteMarkup(prices, userId, group, ""), userId, group)
	if !baseK.IsPositive() {
		return pricing
	}
	out := make([]model.Pricing, len(pricing))
	copy(out, pricing)
	changed := false
	for i := range out {
		name := adtyNormalizeSitePriceModel(out[i].ModelName)
		if name == "" {
			continue
		}
		modelK := adtyClampMarkup(site, ResolveAdtySiteMarkup(prices, userId, group, name), userId, group)
		if !modelK.IsPositive() || modelK.Equal(baseK) {
			continue
		}
		factor := modelK.Div(baseK)
		out[i].ModelPrice = decimal.NewFromFloat(out[i].ModelPrice).Mul(factor).InexactFloat64()
		if out[i].ModelRatio != nil {
			scaled := decimal.NewFromFloat(*out[i].ModelRatio).Mul(factor).InexactFloat64()
			out[i].ModelRatio = &scaled
		}
		changed = true
	}
	if !changed {
		return pricing
	}
	return out
}
