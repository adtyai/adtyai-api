package model

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// HistoricalPriceSnapshotSource is already-stored pricing on a historical
// BillingOperation or TaskBillingContext row. The backfill may only copy or
// convert these values into official G.2-11 snapshot strings. It must not read
// live prices or rewrite reserved/actual amounts.
type HistoricalPriceSnapshotSource struct {
	ModelName       string
	BillingGroup    string
	ModelRatio      float64
	ModelPrice      float64
	GroupRatio      float64
	SiteMarkup      float64
	SiteCostFactor  float64
	SiteId          int
	SiteOwnerUserId int
	PerCallBilling  bool
	Tiered          bool
	AcceptedAt      int64
	Existing        *RequestPriceSnapshot
}

const (
	PriceSnapshotBackfillSkipAlreadyComplete     = "already_complete"
	PriceSnapshotBackfillSkipMissingBasePrice    = "missing_stored_model_price_or_ratio"
	PriceSnapshotBackfillSkipMissingGroupRatio   = "missing_stored_group_ratio"
	PriceSnapshotBackfillSkipIncompleteSiteRates = "incomplete_stored_site_cost_or_markup"
	PriceSnapshotBackfillSkipMissingAcceptedAt   = "missing_stored_acceptance_time"
	PriceSnapshotBackfillSkipNoStoredPricing     = "no_stored_pricing_fields"
)

// BuildHistoricalRequestPriceSnapshot copies already-stored amounts and ratios
// into a complete official G.2-11 snapshot. Rows that cannot be filled safely
// are left unchanged: the function returns a skip reason and a nil snapshot.
func BuildHistoricalRequestPriceSnapshot(src HistoricalPriceSnapshotSource) (*RequestPriceSnapshot, string) {
	if src.Existing != nil && src.Existing.HasOfficialFields() {
		return src.Existing, PriceSnapshotBackfillSkipAlreadyComplete
	}

	base, unit, hasBase := historicalBasePrice(src)
	if !hasBase {
		if src.ModelRatio == 0 && src.ModelPrice == 0 && src.GroupRatio == 0 && src.SiteMarkup == 0 && src.SiteCostFactor == 0 {
			return nil, PriceSnapshotBackfillSkipNoStoredPricing
		}
		// A tiered_expr row has no base ratio by design. Build it with the price
		// fields omitted, exactly as the accept-time freeze does, so the two
		// writers never disagree about whether such a record is legal and never
		// store a zero price that contradicts the amount billed.
		if unit != BillingUnitTieredExpr {
			return nil, PriceSnapshotBackfillSkipMissingBasePrice
		}
	}
	billed, ok := decimalFromStoredFloat(src.GroupRatio)
	if !ok {
		return nil, PriceSnapshotBackfillSkipMissingGroupRatio
	}
	cost, markupRatio, storedK, mainSite, ok := historicalSiteRates(src)
	if !ok {
		return nil, PriceSnapshotBackfillSkipIncompleteSiteRates
	}
	if src.AcceptedAt == 0 {
		return nil, PriceSnapshotBackfillSkipMissingAcceptedAt
	}

	basePrice := ""
	finalPrice := ""
	if hasBase {
		basePrice = base.String()
		finalPrice = base.Mul(billed).String()
	}
	snap := &RequestPriceSnapshot{
		SchemaVersion:    RequestPriceSnapshotVersion,
		ModelName:        strings.TrimSpace(src.ModelName),
		ModelBasePrice:   basePrice,
		BillingGroup:     strings.TrimSpace(src.BillingGroup),
		GroupRatio:       billed.String(),
		SiteCostFactor:   cost.String(),
		SiteMarkupRatio:  markupRatio.String(),
		FinalPrice:       finalPrice,
		Currency:         RequestPriceCurrencyUSD,
		BillingUnit:      unit,
		AcceptedAt:       src.AcceptedAt,
		RuleVersion:      RequestPriceSnapshotVersion,
		SiteId:           src.SiteId,
		SiteOwnerUserId:  src.SiteOwnerUserId,
		MainSite:         mainSite,
		BilledGroupRatio: billed.String(),
	}
	if storedK != "" {
		snap.StoredMarkupK = storedK
	}
	if unit == BillingUnitPerCall {
		snap.ModelPrice = basePrice
	} else if d, ok := decimalFromStoredFloat(src.ModelRatio); ok {
		snap.ModelRatio = d.String()
	} else if unit == BillingUnitPerToken {
		snap.ModelRatio = basePrice
	}
	if !snap.HasOfficialFields() {
		return nil, PriceSnapshotBackfillSkipNoStoredPricing
	}
	return snap, ""
}

func historicalBasePrice(src HistoricalPriceSnapshotSource) (decimal.Decimal, string, bool) {
	unit := BillingUnitPerToken
	if src.Tiered {
		unit = BillingUnitTieredExpr
	} else if src.PerCallBilling || (src.ModelPrice > 0 && src.ModelRatio == 0) {
		unit = BillingUnitPerCall
	}
	if unit == BillingUnitPerCall {
		d, ok := decimalFromStoredFloat(src.ModelPrice)
		return d, unit, ok
	}
	if d, ok := decimalFromStoredFloat(src.ModelRatio); ok {
		return d, unit, true
	}
	if unit == BillingUnitTieredExpr {
		if d, ok := decimalFromStoredFloat(src.ModelPrice); ok {
			return d, unit, true
		}
	}
	return decimal.Zero, unit, false
}

func historicalSiteRates(src HistoricalPriceSnapshotSource) (cost, markupRatio decimal.Decimal, storedK string, mainSite, ok bool) {
	// Historical rows encode a main-site customer as site_id=0 with zero k and d.
	if src.SiteId == 0 && src.SiteMarkup == 0 && src.SiteCostFactor == 0 {
		return decimal.NewFromInt(1), decimal.Zero, "", true, true
	}
	k, hasK := decimalFromStoredFloat(src.SiteMarkup)
	d, hasD := decimalFromStoredFloat(src.SiteCostFactor)
	if !hasK || !hasD || k.LessThan(decimal.NewFromInt(1)) {
		return decimal.Zero, decimal.Zero, "", false, false
	}
	return d, k.Sub(decimal.NewFromInt(1)), k.String(), src.SiteId == 0, true
}

func decimalFromStoredFloat(value float64) (decimal.Decimal, bool) {
	if value == 0 {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(strconv.FormatFloat(value, 'f', -1, 64))
	if err != nil || !d.IsPositive() {
		return decimal.Zero, false
	}
	return d, true
}
