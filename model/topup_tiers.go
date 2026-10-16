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

const DefaultFirstTopupBonusRate = 0.0 // 总裁令：首充赠送已停用，恒为 0

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
// 总裁令：首充赠送整条停用，对外/结算恒返回 0（内存态可仍被 option 写入，但不生效）。
func GetFirstTopupBonusRate() float64 {
	return 0
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
	ChosenBonusUSD     float64 // 现仅档位；历史字段名保留
	ImmediateBonusUSD  float64 // 立刻入赠送账
	PendingExtraUSD    float64 // 已停用：恒 0（不再进首充多送风控）
	IsFirstCandidate   bool    // 已停用：恒 false
	ApplyFirstImmediately bool // 已停用：恒 false
	PendingReason      string
}

// ComputeBonusPlan 按金额计算赠送拆分（不写库）。
// 总裁令：不再与首充取高；firstCandidate/deviceOK/ipConflict 保留签名兼容，一律忽略。
// 结算只走档位赠送；PendingExtraUSD / ApplyFirstImmediately 恒为 0/false（首充多送风控不再入队）。
func ComputeBonusPlan(amountUSD int64, firstCandidate, deviceOK, ipConflict bool) (BonusPlan, error) {
	_ = firstCandidate
	_ = deviceOK
	_ = ipConflict
	rate, ok := TierBonusRateForAmount(amountUSD)
	if !ok {
		return BonusPlan{}, ErrInvalidTopupAmount
	}
	idx := TierIndexForAmount(amountUSD)
	amt := decimal.NewFromInt(amountUSD)
	tierUSD, _ := amt.Mul(decimal.NewFromFloat(rate)).Round(4).Float64()

	return BonusPlan{
		TierIndex:             idx,
		TierBonusRate:          rate,
		FirstBonusRate:        0, // 首充已停
		TierBonusUSD:           tierUSD,
		FirstBonusUSD:         0,
		ChosenBonusUSD:        tierUSD,
		ImmediateBonusUSD:     tierUSD,
		PendingExtraUSD:       0,
		IsFirstCandidate:      false,
		ApplyFirstImmediately: false,
	}, nil
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
