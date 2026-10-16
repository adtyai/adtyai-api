package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// 固定充值档位（美元额度）。POST /api/topup/order 只接受这些金额。
var FixedTopupAmountsUSD = []int64{
	10, 30, 50, 100, 300, 500, 1000, 3000, 5000, 10000, 30000, 50000, 100000,
}

// 默认档位赠送比例（%），依次对应 FixedTopupAmountsUSD；最高档 8%。
var DefaultTopupBonusTierPercents = []float64{
	1, 1.5, 2, 2.5, 3, 3.5, 4, 4.5, 5, 5.5, 6, 6.5, 8,
}

const DefaultFirstTopupBonusRate = 0.10

var (
	ErrInvalidTopupAmount = errors.New("topup amount must be one of the fixed tiers")
	ErrInvalidTopupTiers  = errors.New("invalid TopupBonusTiers")
)

var (
	topupBonusMu     sync.RWMutex
	topupBonusPercents = append([]float64(nil), DefaultTopupBonusTierPercents...)
	firstTopupRate   = DefaultFirstTopupBonusRate
)

// DefaultTopupBonusTiersJSON 默认阶梯 JSON（百分数数组）。
func DefaultTopupBonusTiersJSON() string {
	b, _ := json.Marshal(DefaultTopupBonusTierPercents)
	return string(b)
}

// InitAdtyTopupBonusFromOptions 从 OptionMap 加载内存态（InitOptionMap / 启动后调用）。
func InitAdtyTopupBonusFromOptions() {
	common.OptionMapRWMutex.RLock()
	tiers := common.OptionMap["TopupBonusTiers"]
	rateStr := common.OptionMap["FirstTopupBonusRate"]
	common.OptionMapRWMutex.RUnlock()
	if tiers != "" {
		_ = UpdateTopupBonusTiersByJSONString(tiers)
	}
	if rateStr != "" {
		_ = UpdateFirstTopupBonusRateString(rateStr)
	}
}

func UpdateTopupBonusTiersByJSONString(jsonStr string) error {
	var percents []float64
	if err := json.Unmarshal([]byte(jsonStr), &percents); err != nil {
		return err
	}
	if err := validateTopupBonusTiers(percents); err != nil {
		return err
	}
	topupBonusMu.Lock()
	topupBonusPercents = percents
	topupBonusMu.Unlock()
	return nil
}

func UpdateFirstTopupBonusRateString(s string) error {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return err
	}
	f, _ := d.Float64()
	if f < 0 || f > 1 {
		return fmt.Errorf("FirstTopupBonusRate must be in [0,1]")
	}
	topupBonusMu.Lock()
	firstTopupRate = f
	topupBonusMu.Unlock()
	return nil
}

func validateTopupBonusTiers(percents []float64) error {
	if len(percents) != len(FixedTopupAmountsUSD) {
		return &BusinessParamCodedError{
			Code:    "topup_tiers_count_mismatch",
			Message: fmt.Sprintf("need %d rates, got %d", len(FixedTopupAmountsUSD), len(percents)),
			Params:  map[string]any{"key": "TopupBonusTiers", "expected_count": len(FixedTopupAmountsUSD), "actual_count": len(percents)},
			Err:     ErrInvalidTopupTiers,
		}
	}
	var prev float64 = -1
	for i, p := range percents {
		if p < 0 || p > 100 {
			return &BusinessParamCodedError{
				Code:    "out_of_range",
				Message: "value out of range",
				Params:  map[string]any{"key": "TopupBonusTiers", "index": i, "value": p, "min": 0, "max": 100},
				Err:     ErrInvalidTopupTiers,
			}
		}
		if p < prev {
			return &BusinessParamCodedError{
				Code:    "topup_tiers_not_ascending",
				Message: "rates must be non-decreasing",
				Params:  map[string]any{"key": "TopupBonusTiers", "index": i, "value": p, "prev_value": prev},
				Err:     ErrInvalidTopupTiers,
			}
		}
		prev = p
	}
	return nil
}

// GetTopupBonusTierPercents 返回当前档位赠送百分数副本。
func GetTopupBonusTierPercents() []float64 {
	topupBonusMu.RLock()
	defer topupBonusMu.RUnlock()
	out := make([]float64, len(topupBonusPercents))
	copy(out, topupBonusPercents)
	return out
}

// GetFirstTopupBonusRate 当前首充比例（0–1）。
func GetFirstTopupBonusRate() float64 {
	topupBonusMu.RLock()
	defer topupBonusMu.RUnlock()
	return firstTopupRate
}

// IsFixedTopupAmount 是否属于 13 个固定档位。
func IsFixedTopupAmount(amountUSD int64) bool {
	for _, a := range FixedTopupAmountsUSD {
		if a == amountUSD {
			return true
		}
	}
	return false
}

// TierIndexForAmount 返回档位下标；非法金额返回 -1。
func TierIndexForAmount(amountUSD int64) int {
	for i, a := range FixedTopupAmountsUSD {
		if a == amountUSD {
			return i
		}
	}
	return -1
}

// TierBonusRateForAmount 档位赠送比例（0–1）；非法金额返回 0,false。
func TierBonusRateForAmount(amountUSD int64) (float64, bool) {
	idx := TierIndexForAmount(amountUSD)
	if idx < 0 {
		return 0, false
	}
	percents := GetTopupBonusTierPercents()
	return percents[idx] / 100.0, true
}

// BonusPlan 下单时算出的赠送快照（美元）。
type BonusPlan struct {
	TierIndex          int
	TierBonusRate       float64 // 0–1 snapshot
	FirstBonusRate     float64 // 0–1 snapshot of configured first rate
	TierBonusUSD        float64
	FirstBonusUSD      float64
	ChosenBonusUSD     float64 // max(tier, first) if first eligible path; else tier
	ImmediateBonusUSD  float64 // 立刻入赠送账
	PendingExtraUSD    float64 // 进 pending_review 的首充多送
	IsFirstCandidate   bool    // 账号侧尚无 claimed
	ApplyFirstImmediately bool // 立刻按较高比例入账（有合法 device_fp 且设备未占用、IP 未冲突）
	PendingReason      string
}

// ComputeBonusPlan 按金额与首充资格计算赠送拆分（不写库）。
// firstCandidate: 该账号尚未 claimed。
// deviceOK: device_fp 非空且未被其他账号占用。
// ipConflict: 已有 claimed 记录同 IP。
func ComputeBonusPlan(amountUSD int64, firstCandidate, deviceOK, ipConflict bool) (BonusPlan, error) {
	rate, ok := TierBonusRateForAmount(amountUSD)
	if !ok {
		return BonusPlan{}, ErrInvalidTopupAmount
	}
	idx := TierIndexForAmount(amountUSD)
	firstRate := GetFirstTopupBonusRate()
	amt := decimal.NewFromInt(amountUSD)
	tierUSD, _ := amt.Mul(decimal.NewFromFloat(rate)).Round(4).Float64()
	firstUSD, _ := amt.Mul(decimal.NewFromFloat(firstRate)).Round(4).Float64()

	plan := BonusPlan{
		TierIndex:        idx,
		TierBonusRate:     rate,
		FirstBonusRate:   firstRate,
		TierBonusUSD:      tierUSD,
		FirstBonusUSD:    firstUSD,
		IsFirstCandidate: firstCandidate,
	}

	if !firstCandidate {
		plan.ChosenBonusUSD = tierUSD
		plan.ImmediateBonusUSD = tierUSD
		return plan, nil
	}

	// 首充候选人：取较高；是否立刻给满取决于 device / IP。
	chosen := firstUSD
	if tierUSD > chosen {
		chosen = tierUSD
	}
	plan.ChosenBonusUSD = chosen
	extra, _ := decimal.NewFromFloat(chosen).Sub(decimal.NewFromFloat(tierUSD)).Round(4).Float64()
	if extra < 0 {
		extra = 0
	}

	switch {
	case !deviceOK:
		// 缺指纹：实付+档位赠送立刻到账，首充多送进 pending_review
		plan.ImmediateBonusUSD = tierUSD
		plan.PendingExtraUSD = extra
		plan.PendingReason = "missing_device_fp"
		plan.ApplyFirstImmediately = false
	case ipConflict:
		plan.ImmediateBonusUSD = tierUSD
		plan.PendingExtraUSD = extra
		plan.PendingReason = "ip_conflict"
		plan.ApplyFirstImmediately = false
	default:
		plan.ImmediateBonusUSD = chosen
		plan.PendingExtraUSD = 0
		plan.ApplyFirstImmediately = true
	}
	return plan, nil
}

// DollarsToQuota 美元额度 → 内部 quota 整数。
func DollarsToQuota(dollars float64) (int, error) {
	if dollars <= 0 {
		return 0, nil
	}
	return common.WalletQuotaFromDecimalStrict(
		decimal.NewFromFloat(dollars).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
}
