package model

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// G.2-11 snapshot schema / price-rule version. This is a stable identifier for
// the official field set, not a product versioning system.
const (
	RequestPriceSnapshotVersion = "g2-11-v1"
	RequestPriceCurrencyUSD     = "USD"
	BillingUnitPerToken         = "per_token"
	BillingUnitPerCall          = "per_call"
	BillingUnitTieredExpr       = "tiered_expr"
)

// RequestPriceSnapshot is the immutable G.2-11 / A.10-34 request price record.
// Authoritative numeric fields are exact decimal strings. Settlement, retry,
// and resume must parse these strings and must not reconstruct them from
// float64 columns when this snapshot is present.
type RequestPriceSnapshot struct {
	SchemaVersion        string `json:"schema_version"`
	ModelName            string `json:"model_name,omitempty"`
	ModelBasePrice       string `json:"model_base_price"`
	BillingGroup         string `json:"billing_group"`
	GroupRatio           string `json:"group_ratio"`
	SiteCostFactor       string `json:"site_cost_factor"`
	SiteMarkupRatio      string `json:"site_markup_ratio"`
	TeamPayRatio         string `json:"team_pay_ratio,omitempty"`
	TeamApplied          bool   `json:"team_applied"`
	SupplyPriceFloor     string `json:"supply_price_floor,omitempty"`
	FinalPrice           string `json:"final_price"`
	Currency             string `json:"currency"`
	BillingUnit          string `json:"billing_unit"`
	AcceptedAt           int64  `json:"accepted_at"`
	RuleVersion          string `json:"rule_version"`
	SupplyCost           string `json:"supply_cost"`
	SellPrice            string `json:"sell_price"`
	ConsumedAmount       string `json:"consumed_amount,omitempty"`
	SiteId               int    `json:"site_id,omitempty"`
	SiteOwnerUserId      int    `json:"site_owner_user_id,omitempty"`
	MainSite             bool   `json:"main_site,omitempty"`
	ModelRatio           string `json:"model_ratio,omitempty"`
	ModelPrice           string `json:"model_price,omitempty"`
	CompletionRatio      string `json:"completion_ratio,omitempty"`
	CacheRatio           string `json:"cache_ratio,omitempty"`
	AudioRatio           string `json:"audio_ratio,omitempty"`
	AudioCompletionRatio string `json:"audio_completion_ratio,omitempty"`
	BilledGroupRatio     string `json:"billed_group_ratio"`
	StoredMarkupK        string `json:"stored_markup_k,omitempty"`
}

// HasOfficialFields reports whether the G.2-11 field set is present as stored
// strings (not reconstructed from float64). An empty billing group is allowed
// when the request had no named group; team pay and the supply floor may be
// empty when they do not apply.
func (s *RequestPriceSnapshot) HasOfficialFields() bool {
	if s == nil {
		return false
	}
	return strings.TrimSpace(s.ModelBasePrice) != "" &&
		strings.TrimSpace(s.GroupRatio) != "" &&
		strings.TrimSpace(s.SiteCostFactor) != "" &&
		strings.TrimSpace(s.SiteMarkupRatio) != "" &&
		strings.TrimSpace(s.FinalPrice) != "" &&
		strings.TrimSpace(s.Currency) != "" &&
		strings.TrimSpace(s.BillingUnit) != "" &&
		s.AcceptedAt != 0 &&
		strings.TrimSpace(s.RuleVersion) != ""
}

// ParseSnapshotDecimal parses an authoritative snapshot decimal string.
func ParseSnapshotDecimal(value string) (decimal.Decimal, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, false
	}
	return d, true
}

func (s *RequestPriceSnapshot) BilledGroupRatioDecimal() (decimal.Decimal, bool) {
	if s == nil {
		return decimal.Zero, false
	}
	return ParseSnapshotDecimal(s.BilledGroupRatio)
}

func (s *RequestPriceSnapshot) ModelRatioDecimal() (decimal.Decimal, bool) {
	if s == nil {
		return decimal.Zero, false
	}
	if d, ok := ParseSnapshotDecimal(s.ModelRatio); ok {
		return d, true
	}
	if s.BillingUnit == BillingUnitPerToken {
		return ParseSnapshotDecimal(s.ModelBasePrice)
	}
	return decimal.Zero, false
}

func (s *RequestPriceSnapshot) ModelPriceDecimal() (decimal.Decimal, bool) {
	if s == nil {
		return decimal.Zero, false
	}
	if d, ok := ParseSnapshotDecimal(s.ModelPrice); ok {
		return d, true
	}
	if s.BillingUnit == BillingUnitPerCall {
		return ParseSnapshotDecimal(s.ModelBasePrice)
	}
	return decimal.Zero, false
}

func (s *RequestPriceSnapshot) CompletionRatioDecimal() (decimal.Decimal, bool) {
	if s == nil {
		return decimal.Zero, false
	}
	return ParseSnapshotDecimal(s.CompletionRatio)
}

func (s *RequestPriceSnapshot) AudioRatioDecimal() (decimal.Decimal, bool) {
	if s == nil {
		return decimal.Zero, false
	}
	return ParseSnapshotDecimal(s.AudioRatio)
}

func (s *RequestPriceSnapshot) AudioCompletionRatioDecimal() (decimal.Decimal, bool) {
	if s == nil {
		return decimal.Zero, false
	}
	return ParseSnapshotDecimal(s.AudioCompletionRatio)
}

// LegacyMarkupFloat is the stored k for compatibility columns only.
func (s *RequestPriceSnapshot) LegacyMarkupFloat() float64 {
	if s == nil {
		return 0
	}
	if d, ok := ParseSnapshotDecimal(s.StoredMarkupK); ok {
		return d.InexactFloat64()
	}
	return 0
}

// LegacyCostFactorFloat is the supply multiplier for compatibility columns only.
func (s *RequestPriceSnapshot) LegacyCostFactorFloat() float64 {
	if s == nil {
		return 0
	}
	if d, ok := ParseSnapshotDecimal(s.SiteCostFactor); ok {
		return d.InexactFloat64()
	}
	return 0
}

// WithConsumedQuota records the final billed amount in USD once, without
// changing any other frozen field. quota is the integer wallet amount.
func (s *RequestPriceSnapshot) WithConsumedQuota(quota int) *RequestPriceSnapshot {
	if s == nil || quota < 0 || strings.TrimSpace(s.ConsumedAmount) != "" {
		return s
	}
	clone := *s
	clone.ConsumedAmount = decimal.NewFromInt(int64(quota)).Div(common.QuotaPerUnitDecimal()).String()
	return &clone
}

func EncodeRequestPriceSnapshot(s *RequestPriceSnapshot) (string, error) {
	if s == nil {
		return "", nil
	}
	encoded, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func DecodeRequestPriceSnapshot(raw string) *RequestPriceSnapshot {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var snapshot RequestPriceSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return nil
	}
	if !snapshot.HasOfficialFields() {
		return nil
	}
	return &snapshot
}
