package billingexpr

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// snapshotQuotaPerUnitDecimal is the financial authority for converting
// expression output into internal quota. Official USD books always use the
// fixed 1 USD = 500,000 scale; a non-standard snapshot unit (tests or
// historical fixtures) is converted once at this boundary.
func snapshotQuotaPerUnitDecimal(snap *BillingSnapshot) decimal.Decimal {
	if snap == nil || snap.QuotaPerUnit == 0 || snap.QuotaPerUnit == float64(common.FixedQuotaPerUnit) {
		return common.QuotaPerUnitDecimal()
	}
	return decimal.NewFromFloat(snap.QuotaPerUnit)
}

func groupRatioDecimal(snap *BillingSnapshot) decimal.Decimal {
	if snap == nil {
		return decimal.NewFromInt(1)
	}
	return decimal.NewFromFloat(snap.GroupRatio)
}

// quotaConversionDecimal converts raw expression output to quota based on the
// expression version. Intermediate math stays in decimal; the caller rounds
// once when producing the billed integer amount.
func quotaConversionDecimal(exprOutput float64, snap *BillingSnapshot) decimal.Decimal {
	output := decimal.NewFromFloat(exprOutput)
	unit := snapshotQuotaPerUnitDecimal(snap)
	if snap != nil && snap.TaskUsageBilling {
		return output.Mul(unit)
	}
	return output.Div(decimal.NewFromInt(1_000_000)).Mul(unit)
}

// ComputeTieredQuota runs the Expr from a frozen BillingSnapshot against
// actual token counts and returns the settlement result.
func ComputeTieredQuota(snap *BillingSnapshot, params TokenParams) (TieredResult, error) {
	return ComputeTieredQuotaWithRequest(snap, params, RequestInput{})
}

func ComputeTieredQuotaWithRequest(snap *BillingSnapshot, params TokenParams, request RequestInput) (TieredResult, error) {
	if snap.TaskUsageBilling && UsesFixedPricingByHash(snap.ExprString, snap.ExprHash) {
		return TieredResult{}, fmt.Errorf("fixed pricing is not supported for task usage expressions")
	}
	cost, trace, err := RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, params, request)
	if err != nil {
		return TieredResult{}, err
	}

	quotaBeforeGroupDec := quotaConversionDecimal(cost, snap)
	afterGroup, clamp := common.QuotaFromDecimalChecked(quotaBeforeGroupDec.Mul(groupRatioDecimal(snap)))
	crossed := trace.MatchedTier != snap.EstimatedTier

	result := TieredResult{
		ImageCount:                trace.ImageCount,
		BillingUnit:               trace.BillingUnit,
		FixedPrice:                trace.FixedPrice,
		ActualQuotaBeforeGroup:    quotaBeforeGroupDec.InexactFloat64(),
		ActualQuotaBeforeGroupDec: quotaBeforeGroupDec,
		ActualQuotaAfterGroup:     afterGroup,
		MatchedTier:               trace.MatchedTier,
		RequestRules:              trace.RequestRules,
		CrossedTier:               crossed,
		Clamp:                     clamp,
	}
	if trace.BillingUnit == BillingUnitToken && UsedVarsByHash(snap.ExprString, snap.ExprHash)["img_cr"] {
		result.BillingTokens = &params
	}
	return result, nil
}
