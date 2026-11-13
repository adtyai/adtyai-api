package service

import (
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	adtyPriceFactorsContextKey  = "adty_price_factors"
	AdtyPriceSnapshotContextKey = "adty_price_snapshot"
)

// adtyPriceFactors holds the exact decimal ratios captured while applying the
// official G.2 formula, in group-ratio space (multipliers on the model base).
type adtyPriceFactors struct {
	BillingGroup    string
	GroupRatio      decimal.Decimal
	StoredK         decimal.Decimal
	MarkupRatio     decimal.Decimal
	CostFactor      decimal.Decimal
	SupplyPrice     decimal.Decimal
	SellPrice       decimal.Decimal
	TeamPayRatio    decimal.Decimal
	TeamApplied     bool
	SupplyFloor     decimal.Decimal
	FloorApplied    bool
	SiteId          int
	SiteOwnerUserId int
	MainSite        bool
}

func adtyRememberPriceFactors(c *gin.Context, factors adtyPriceFactors) {
	if c != nil {
		c.Set(adtyPriceFactorsContextKey, factors)
	}
}

func adtyPriceFactorsFromContext(c *gin.Context) (adtyPriceFactors, bool) {
	if c == nil {
		return adtyPriceFactors{}, false
	}
	value, ok := c.Get(adtyPriceFactorsContextKey)
	if !ok {
		return adtyPriceFactors{}, false
	}
	factors, ok := value.(adtyPriceFactors)
	return factors, ok
}

func adtyDefaultPriceFactors(group string, groupRatio decimal.Decimal) adtyPriceFactors {
	return adtyPriceFactors{
		BillingGroup: group,
		GroupRatio:   groupRatio,
		StoredK:      decimal.NewFromInt(1),
		MarkupRatio:  decimal.Zero,
		CostFactor:   decimal.NewFromInt(1),
		SupplyPrice:  groupRatio,
		SellPrice:    groupRatio,
		MainSite:     true,
	}
}

// AdtyFrozenPriceSnapshot returns the immutable G.2-11 snapshot recorded at
// accept / precharge, if the official field set is present.
func AdtyFrozenPriceSnapshot(c *gin.Context) *model.RequestPriceSnapshot {
	if c == nil {
		return nil
	}
	value, ok := c.Get(AdtyPriceSnapshotContextKey)
	if !ok {
		return nil
	}
	switch snap := value.(type) {
	case *model.RequestPriceSnapshot:
		if snap != nil && snap.HasOfficialFields() {
			return snap
		}
	case model.RequestPriceSnapshot:
		if snap.HasOfficialFields() {
			clone := snap
			return &clone
		}
	}
	return nil
}

// AdtyFreezeRequestPriceSnapshot records the official G.2-11 snapshot on the
// request context. It is a no-op when a complete snapshot is already present.
func AdtyFreezeRequestPriceSnapshot(c *gin.Context, info *relaycommon.RelayInfo) {
	if c == nil || info == nil {
		return
	}
	if AdtyFrozenPriceSnapshot(c) != nil {
		return
	}
	snap := buildRequestPriceSnapshot(c, info)
	if snap == nil || !snap.HasOfficialFields() {
		return
	}
	c.Set(AdtyPriceSnapshotContextKey, snap)
}

func buildRequestPriceSnapshot(c *gin.Context, info *relaycommon.RelayInfo) *model.RequestPriceSnapshot {
	price := info.PriceData
	factors, ok := adtyPriceFactorsFromContext(c)
	if !ok {
		group := info.UsingGroup
		groupRatio := price.GroupRatioInfo.BilledGroupRatio()
		if !price.GroupRatioInfo.HasBilledGroupRatio() && price.GroupRatioInfo.GroupRatio != 0 {
			groupRatio = decimal.NewFromFloat(price.GroupRatioInfo.GroupRatio)
		}
		factors = adtyDefaultPriceFactors(group, groupRatio)
	}
	if factors.BillingGroup == "" {
		factors.BillingGroup = info.UsingGroup
	}

	unit := model.BillingUnitPerToken
	base := decimal.NewFromFloat(price.ModelRatio)
	if price.UsePrice {
		unit = model.BillingUnitPerCall
		base = decimal.NewFromFloat(price.ModelPrice)
	}
	if info.TieredBillingSnapshot != nil && info.TieredBillingSnapshot.BillingMode == "tiered_expr" {
		unit = model.BillingUnitTieredExpr
	}

	finalRatio := price.GroupRatioInfo.BilledGroupRatio()
	if billed, found := AdtyBilledGroupRatioFromContext(c); found {
		finalRatio = billed
	} else if finalRatio.IsZero() && !factors.SellPrice.IsZero() {
		finalRatio = factors.SellPrice
	}

	acceptedAt := time.Now().Unix()
	if !info.StartTime.IsZero() {
		acceptedAt = info.StartTime.Unix()
	}

	modelName := info.GetBillingModelName()
	if modelName == "" {
		modelName = info.OriginModelName
	}
	snap := &model.RequestPriceSnapshot{
		SchemaVersion:    model.RequestPriceSnapshotVersion,
		ModelName:        modelName,
		ModelBasePrice:   base.String(),
		BillingGroup:     factors.BillingGroup,
		GroupRatio:       factors.GroupRatio.String(),
		SiteCostFactor:   factors.CostFactor.String(),
		SiteMarkupRatio:  factors.MarkupRatio.String(),
		FinalPrice:       base.Mul(finalRatio).String(),
		Currency:         model.RequestPriceCurrencyUSD,
		BillingUnit:      unit,
		AcceptedAt:       acceptedAt,
		RuleVersion:      model.RequestPriceSnapshotVersion,
		SupplyCost:       base.Mul(factors.SupplyPrice).String(),
		SellPrice:        base.Mul(factors.SellPrice).String(),
		SiteId:           factors.SiteId,
		SiteOwnerUserId:  factors.SiteOwnerUserId,
		MainSite:         factors.MainSite || factors.SiteId == 0,
		BilledGroupRatio: finalRatio.String(),
		StoredMarkupK:    factors.StoredK.String(),
	}
	if factors.TeamApplied {
		snap.TeamApplied = true
		snap.TeamPayRatio = factors.TeamPayRatio.String()
	}
	if factors.FloorApplied {
		snap.SupplyPriceFloor = base.Mul(factors.SupplyFloor).String()
	}
	if price.UsePrice {
		snap.ModelPrice = base.String()
	} else if price.ModelRatio != 0 || unit == model.BillingUnitPerToken {
		snap.ModelRatio = decimal.NewFromFloat(price.ModelRatio).String()
	}
	if price.CompletionRatio != 0 {
		snap.CompletionRatio = decimal.NewFromFloat(price.CompletionRatio).String()
	}
	if price.CacheRatio != 0 {
		snap.CacheRatio = decimal.NewFromFloat(price.CacheRatio).String()
	}
	if price.AudioRatio != 0 {
		snap.AudioRatio = decimal.NewFromFloat(price.AudioRatio).String()
	}
	if price.AudioCompletionRatio != 0 {
		snap.AudioCompletionRatio = decimal.NewFromFloat(price.AudioCompletionRatio).String()
	}
	return snap
}

// GroupRatioInfoFromSnapshot rebuilds the billed group ratio from the frozen
// decimal snapshot so retries do not re-read live prices.
func GroupRatioInfoFromSnapshot(snap *model.RequestPriceSnapshot) hosttypes.GroupRatioInfo {
	info := hosttypes.GroupRatioInfo{}
	if snap == nil {
		return info
	}
	if billed, ok := snap.BilledGroupRatioDecimal(); ok {
		info.GroupRatio = billed.InexactFloat64()
		info.SetBilledGroupRatio(billed)
	}
	info.SiteMarkup = snap.LegacyMarkupFloat()
	return info
}

func applySnapshotToBillingOperation(row *model.BillingOperation, snap *model.RequestPriceSnapshot) {
	if row == nil || snap == nil {
		return
	}
	encoded, err := model.EncodeRequestPriceSnapshot(snap)
	if err != nil || encoded == "" {
		return
	}
	row.PriceSnapshot = encoded
	if snap.SiteId > 0 {
		row.SiteId = snap.SiteId
		row.SiteOwnerUserId = snap.SiteOwnerUserId
	}
	if k := snap.LegacyMarkupFloat(); k != 0 {
		row.SiteMarkup = k
	}
	if d := snap.LegacyCostFactorFloat(); d != 0 {
		row.SiteCostFactor = d
	}
}

func applySnapshotToQuotaInfo(info QuotaInfo, snap *model.RequestPriceSnapshot) QuotaInfo {
	if snap == nil {
		return info
	}
	if billed, ok := snap.BilledGroupRatioDecimal(); ok {
		info = withBilledGroupRatio(info, billed)
	}
	if d, ok := snap.ModelRatioDecimal(); ok {
		info.ModelRatio = d.InexactFloat64()
		info.ModelRatioDecimal = d
		info.hasModelRatioDecimal = true
	}
	if d, ok := snap.ModelPriceDecimal(); ok && snap.BillingUnit == model.BillingUnitPerCall {
		info.ModelPrice = d.InexactFloat64()
		info.ModelPriceDecimal = d
		info.hasModelPriceDecimal = true
		info.UsePrice = true
	}
	if d, ok := snap.CompletionRatioDecimal(); ok {
		info.CompletionRatioDecimal = d
		info.hasCompletionRatioDecimal = true
	}
	if d, ok := snap.AudioRatioDecimal(); ok {
		info.AudioRatioDecimal = d
		info.hasAudioRatioDecimal = true
	}
	if d, ok := snap.AudioCompletionRatioDecimal(); ok {
		info.AudioCompletionRatioDecimal = d
		info.hasAudioCompletionRatioDecimal = true
	}
	return info
}

// AdtyReuseFrozenPriceData reports whether this call should return the already
// frozen PriceData. A snapshot for a different model on a reused context is
// not reused, so accept-time helpers can still price a new request.
func AdtyReuseFrozenPriceData(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if c == nil || info == nil {
		return false
	}
	snap := AdtyFrozenPriceSnapshot(c)
	if snap == nil {
		return false
	}
	name := info.GetBillingModelName()
	if name == "" {
		name = info.OriginModelName
	}
	if snap.ModelName != "" && name != "" && snap.ModelName != name && snap.ModelName != info.OriginModelName {
		return false
	}
	return info.PriceData.ModelRatio != 0 || info.PriceData.ModelPrice != 0 || info.PriceData.UsePrice || info.PriceData.FreeModel || info.PriceData.QuotaToPreConsume != 0 || info.PriceData.Quota != 0
}
