package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Business-param catalog (adtyai). All keys live in options.
// 「老版」defaults: placeholder until verified from 老版二开; see TODO comments.

var (
	ErrUnknownBusinessParam = errors.New("unknown business param")
	ErrBusinessParamRange   = errors.New("business param out of range")
	ErrBusinessParamFormat  = errors.New("business param invalid format")
)

const (
	BPTypeNumber = "number"
	BPTypeString = "string"
	BPTypeJSON   = "json"
	BPTypeEnum   = "enum"
)

// Runtime holders for adtyai business params (besides TopupBonus* already in topup_tiers.go).
var (
	bpMu sync.RWMutex

	// TODO(adtyai): TopupOrderTimeoutMinutes 默认先用 60，开工时从老版二开核实后改。
	bpTopupOrderTimeoutMinutes = 60
	// Bepusdt / USDT
	bpBepusdtOrderTimeoutMinutes = 15
	bpUsdtUnderpayTolerance      = 0.01
	// Affiliate — TODO(adtyai): Aff* 佣金/提现默认先占位，从老版核实后改。
	// 老版现见：ReferralDefaultRate=0, ReferralMinWithdrawAmount=0, ReferralWithdrawFee=0, FreezeDays=7
	bpAffTopupCommissionRate        = 0.10
	bpAffSubscriptionCommissionRate = 0.10
	bpAffFreezeDays                 = 7
	bpAffMinWithdrawAmount          = 10.0 // TODO(adtyai): 老版 ReferralMinWithdrawAmount=0，占位用 10
	bpAffWithdrawFeeRate            = 0.0
	bpBalanceAdjustMaxAmount        = 500.0
	bpSelfApproveAlertAmount        = 500.0
	bpEpayFailureDemoteThreshold    = 5
	bpChannelGracePeriodSeconds     = 600
	bpPayUnknownEscalateHours       = 24
	bpBalanceLowAlertThreshold      = 0.0 // 无独立官方项则 0；QuotaRemindThreshold 仍走官方路径
	bpBackupContactType             = "email"
	bpBackupContact                 = "support@adtyai.com"
	bpTopupOrderRateLimitNum        = 10
	bpTopupOrderRateLimitDuration   = 60
	bpDiskAlertWarnPct              = 80
	bpDiskAlertCritPct              = 90
	bpChannelBalanceAlertUsd        = 20.0
	bpDailyReportTime               = "09:00"
	bpWalletAccountReconcileFrom    = ""
	bpBackupRetentionDays           = 14
	bpAlertMergeMinutes             = 10
	bpRefundAlertAmount             = 500.0
	bpRefundAlertDailyCount         = 3
)

// BusinessParamDef describes one catalog entry for admin API meta.
type BusinessParamDef struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Default     string   `json:"default"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Snapshot    string   `json:"snapshot,omitempty"` // order | affiliate | withdraw | apply | ""
	Page        string   `json:"page,omitempty"`
	Notes       string   `json:"notes,omitempty"`
	Description string   `json:"description,omitempty"`
	// Nullable 允许空值（空表示关闭 / 未设置），accept 等检查不把空当错误。
	Nullable bool `json:"nullable,omitempty"`
}

func fptr(v float64) *float64 { return &v }

// BusinessParamCatalog returns the fixed ~30-item list (excludes EpayInstanceDailyLimit).
func BusinessParamCatalog() []BusinessParamDef {
	return []BusinessParamDef{
		{Key: "TopupBonusTiers", Type: BPTypeJSON, Default: DefaultTopupBonusTiersJSON(), Snapshot: "order", Page: "A10", Notes: "13 rates % matching FixedTopupAmountsUSD; 0-100 non-decreasing"},
		{Key: "FirstTopupBonusRate", Type: BPTypeNumber, Default: "0.10", Min: fptr(0), Max: fptr(1), Snapshot: "order", Page: "A11"},
		{Key: "Price", Type: BPTypeNumber, Default: "7.3", Min: fptr(1), Max: fptr(20), Snapshot: "order", Page: "A33", Notes: "official CNY/USD; no UsdToCnyRate"},
		{Key: "BepusdtOrderTimeoutMinutes", Type: BPTypeNumber, Default: "15", Min: fptr(5), Max: fptr(120), Snapshot: "order", Page: "A13"},
		{Key: "UsdtUnderpayTolerance", Type: BPTypeNumber, Default: "0.01", Min: fptr(0), Max: fptr(1), Snapshot: "order", Page: "A13"},
		{Key: "TopupOrderTimeoutMinutes", Type: BPTypeNumber, Default: "60", Min: fptr(5), Max: fptr(1440), Snapshot: "order", Page: "A33", Notes: "TODO(adtyai): 老版默认待核实，占位 60"},
		{Key: "AffTopupCommissionRate", Type: BPTypeNumber, Default: "0.10", Min: fptr(0), Max: fptr(0.5), Snapshot: "affiliate", Page: "A23", Notes: "TODO(adtyai): 老版 ReferralDefaultRate=0，占位 0.10"},
		{Key: "AffSubscriptionCommissionRate", Type: BPTypeNumber, Default: "0.10", Min: fptr(0), Max: fptr(0.5), Snapshot: "affiliate", Page: "A23", Notes: "TODO(adtyai): 老版待核实，占位 0.10"},
		{Key: "AffFreezeDays", Type: BPTypeNumber, Default: "7", Min: fptr(0), Max: fptr(90), Snapshot: "affiliate", Page: "A23"},
		{Key: "AffMinWithdrawAmount", Type: BPTypeNumber, Default: "10", Min: fptr(0), Snapshot: "withdraw", Page: "A23", Notes: "TODO(adtyai): 老版 ReferralMinWithdrawAmount=0，占位 10"},
		{Key: "AffWithdrawFeeRate", Type: BPTypeNumber, Default: "0", Min: fptr(0), Max: fptr(0.5), Snapshot: "withdraw", Page: "A23", Notes: "TODO(adtyai): 老版 ReferralWithdrawFee=0"},
		{Key: "BalanceAdjustMaxAmount", Type: BPTypeNumber, Default: "500", Min: fptr(0.0001), Snapshot: "apply", Page: "A22", Notes: ">0"},
		{Key: "SelfApproveAlertAmount", Type: BPTypeNumber, Default: "500", Min: fptr(0.0001), Page: "A22/A43", Notes: ">0"},
		{Key: "EpayFailureDemoteThreshold", Type: BPTypeNumber, Default: "5", Min: fptr(1), Max: fptr(100), Page: "A12/A38"},
		{Key: "ChannelGracePeriodSeconds", Type: BPTypeNumber, Default: "600", Min: fptr(0), Max: fptr(3600), Page: "A04"},
		{Key: "PayUnknownEscalateHours", Type: BPTypeNumber, Default: "24", Min: fptr(1), Max: fptr(168), Page: "A43"},
		{Key: "QuotaForNewUser", Type: BPTypeNumber, Default: "0", Min: fptr(0), Page: "A32", Notes: "official; into bonus wallet"},
		{Key: "BalanceLowAlertThreshold", Type: BPTypeNumber, Default: "0", Min: fptr(0), Page: "A32", Notes: "new key; official QuotaRemindThreshold remains separate"},
		{Key: "BackupContactType", Type: BPTypeEnum, Default: "email", Enum: []string{"email", "telegram"}, Page: "A29"},
		{Key: "BackupContact", Type: BPTypeString, Default: "support@adtyai.com", Page: "A29", Notes: "email or @handle"},
		{Key: "TopupOrderRateLimitNum", Type: BPTypeNumber, Default: "10", Min: fptr(1), Max: fptr(100), Page: "A36"},
		{Key: "TopupOrderRateLimitDuration", Type: BPTypeNumber, Default: "60", Min: fptr(10), Max: fptr(3600), Page: "A36"},
		{Key: "DiskAlertWarnPct", Type: BPTypeNumber, Default: "80", Min: fptr(50), Max: fptr(95), Page: "A43"},
		{Key: "DiskAlertCritPct", Type: BPTypeNumber, Default: "90", Min: fptr(51), Max: fptr(99), Page: "A43", Notes: "must be > DiskAlertWarnPct"},
		{Key: "ChannelBalanceAlertUsd", Type: BPTypeNumber, Default: "20", Min: fptr(0), Page: "A43"},
		{Key: "DailyReportTime", Type: BPTypeString, Default: "09:00", Page: "A43", Notes: "HH:MM Asia/Shanghai"},
		{Key: "WalletAccountReconcileFrom", Type: BPTypeString, Default: "", Page: "A43", Notes: "B-03 per-account reconcile cutover YYYY-MM-DD (Asia/Shanghai); empty = off; report only", Nullable: true},
		{Key: "BackupRetentionDays", Type: BPTypeNumber, Default: "14", Min: fptr(7), Max: fptr(90), Page: "A43"},
		{Key: "AlertMergeMinutes", Type: BPTypeNumber, Default: "10", Min: fptr(1), Max: fptr(60), Page: "A43"},
		{Key: "RefundAlertAmount", Type: BPTypeNumber, Default: "500", Min: fptr(0.0001), Page: "A43", Notes: ">0"},
		{Key: "RefundAlertDailyCount", Type: BPTypeNumber, Default: "3", Min: fptr(1), Max: fptr(100), Page: "A43"},
	}
}

func businessParamDefByKey(key string) (BusinessParamDef, bool) {
	for _, d := range BusinessParamCatalog() {
		if d.Key == key {
			return d, true
		}
	}
	return BusinessParamDef{}, false
}

// BusinessParamDefByKey returns the catalog entry for key (for console error params).
func BusinessParamDefByKey(key string) (BusinessParamDef, bool) {
	return businessParamDefByKey(key)
}

// IsBusinessParamKey reports whether key is in the adtyai business-param catalog.
func IsBusinessParamKey(key string) bool {
	_, ok := businessParamDefByKey(key)
	return ok
}

// SeedBusinessParamsIntoOptionMap writes defaults for missing keys into OptionMap
// (caller should hold OptionMapRWMutex or run during InitOptionMap before unlock).
func SeedBusinessParamsIntoOptionMap() {
	for _, d := range BusinessParamCatalog() {
		if _, ok := common.OptionMap[d.Key]; !ok {
			common.OptionMap[d.Key] = d.Default
		}
	}
}

// InitAdtyBusinessParamsFromOptions loads OptionMap into runtime vars.
func InitAdtyBusinessParamsFromOptions() {
	common.OptionMapRWMutex.RLock()
	m := make(map[string]string, len(BusinessParamCatalog()))
	for _, d := range BusinessParamCatalog() {
		m[d.Key] = common.OptionMap[d.Key]
	}
	common.OptionMapRWMutex.RUnlock()
	for k, v := range m {
		if v == "" {
			continue
		}
		_ = applyBusinessParamRuntime(k, v)
	}
}

func applyBusinessParamRuntime(key, value string) error {
	bpMu.Lock()
	defer bpMu.Unlock()
	switch key {
	case "TopupBonusTiers", "FirstTopupBonusRate":
		// handled by topup_tiers helpers (called from updateOptionMap)
		return nil
	case "Price", "QuotaForNewUser":
		return nil // official vars updated in updateOptionMap
	case "BepusdtOrderTimeoutMinutes":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpBepusdtOrderTimeoutMinutes = n
	case "UsdtUnderpayTolerance":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpUsdtUnderpayTolerance = f
	case "TopupOrderTimeoutMinutes":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpTopupOrderTimeoutMinutes = n
	case "AffTopupCommissionRate":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpAffTopupCommissionRate = f
	case "AffSubscriptionCommissionRate":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpAffSubscriptionCommissionRate = f
	case "AffFreezeDays":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpAffFreezeDays = n
	case "AffMinWithdrawAmount":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpAffMinWithdrawAmount = f
	case "AffWithdrawFeeRate":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpAffWithdrawFeeRate = f
	case "BalanceAdjustMaxAmount":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpBalanceAdjustMaxAmount = f
	case "SelfApproveAlertAmount":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpSelfApproveAlertAmount = f
	case "EpayFailureDemoteThreshold":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpEpayFailureDemoteThreshold = n
	case "ChannelGracePeriodSeconds":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpChannelGracePeriodSeconds = n
	case "PayUnknownEscalateHours":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpPayUnknownEscalateHours = n
	case "BalanceLowAlertThreshold":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpBalanceLowAlertThreshold = f
	case "BackupContactType":
		bpBackupContactType = value
	case "BackupContact":
		bpBackupContact = value
	case "TopupOrderRateLimitNum":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpTopupOrderRateLimitNum = n
	case "TopupOrderRateLimitDuration":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpTopupOrderRateLimitDuration = n
	case "DiskAlertWarnPct":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpDiskAlertWarnPct = n
	case "DiskAlertCritPct":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpDiskAlertCritPct = n
	case "ChannelBalanceAlertUsd":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpChannelBalanceAlertUsd = f
	case "DailyReportTime":
		bpDailyReportTime = value
	case "WalletAccountReconcileFrom":
		bpWalletAccountReconcileFrom = value
	case "BackupRetentionDays":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpBackupRetentionDays = n
	case "AlertMergeMinutes":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpAlertMergeMinutes = n
	case "RefundAlertAmount":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		bpRefundAlertAmount = f
	case "RefundAlertDailyCount":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		bpRefundAlertDailyCount = n
	}
	return nil
}

// GetTopupOrderRateLimit returns (num, durationSeconds) for middleware.
func GetTopupOrderRateLimit() (int, int64) {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpTopupOrderRateLimitNum, int64(bpTopupOrderRateLimitDuration)
}

func GetBepusdtOrderTimeoutMinutes() int {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpBepusdtOrderTimeoutMinutes
}

func GetUsdtUnderpayTolerance() float64 {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpUsdtUnderpayTolerance
}

func GetTopupOrderTimeoutMinutes() int {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpTopupOrderTimeoutMinutes
}

func GetAffTopupCommissionRate() float64 {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpAffTopupCommissionRate
}

func GetAffSubscriptionCommissionRate() float64 {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpAffSubscriptionCommissionRate
}

func GetAffFreezeDays() int {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpAffFreezeDays
}

func GetAffMinWithdrawAmount() float64 {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpAffMinWithdrawAmount
}

func GetAffWithdrawFeeRate() float64 {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpAffWithdrawFeeRate
}

func GetBalanceAdjustMaxAmount() float64 {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpBalanceAdjustMaxAmount
}

func GetSelfApproveAlertAmount() float64 {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpSelfApproveAlertAmount
}

func GetEpayFailureDemoteThreshold() int {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpEpayFailureDemoteThreshold
}

func GetBalanceLowAlertThreshold() float64 {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpBalanceLowAlertThreshold
}

func GetBackupContact() (typ, contact string) {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpBackupContactType, bpBackupContact
}

func GetDiskAlertPcts() (warn, crit int) {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpDiskAlertWarnPct, bpDiskAlertCritPct
}

func GetDailyReportTime() string {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpDailyReportTime
}

// GetBackupRetentionDays B12 备份保留天数（默认 14，范围 7..90）。
func GetBackupRetentionDays() int {
	bpMu.RLock()
	defer bpMu.RUnlock()
	if bpBackupRetentionDays < 1 {
		return 14
	}
	return bpBackupRetentionDays
}

func GetAlertMergeMinutes() int {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpAlertMergeMinutes
}

func GetRefundAlertAmount() float64 {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpRefundAlertAmount
}

func GetRefundAlertDailyCount() int {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpRefundAlertDailyCount
}

var (
	reDailyReportTime = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)
	reEmailSimple     = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	reTelegramHandle  = regexp.MustCompile(`^@[A-Za-z0-9_]{5,}$`)
)

// ValidateBusinessParam validates one key/value. proposed overrides OptionMap for cross-field checks.
func ValidateBusinessParam(key, value string, proposed map[string]string) error {
	def, ok := businessParamDefByKey(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownBusinessParam, key)
	}
	value = strings.TrimSpace(value)
	switch key {
	case "TopupBonusTiers":
		return UpdateTopupBonusTiersByJSONStringDryRun(value)
	case "FirstTopupBonusRate":
		return validateFloatRange(value, 0, 1, true, true)
	case "Price":
		return validateFloatRange(value, 1, 20, true, true)
	case "BepusdtOrderTimeoutMinutes":
		return validateIntRange(value, 5, 120)
	case "UsdtUnderpayTolerance":
		return validateFloatRange(value, 0, 1, true, true)
	case "TopupOrderTimeoutMinutes":
		return validateIntRange(value, 5, 1440)
	case "AffTopupCommissionRate", "AffSubscriptionCommissionRate", "AffWithdrawFeeRate":
		return validateFloatRange(value, 0, 0.5, true, true)
	case "AffFreezeDays":
		return validateIntRange(value, 0, 90)
	case "AffMinWithdrawAmount", "BalanceLowAlertThreshold", "ChannelBalanceAlertUsd":
		return validateFloatRange(value, 0, -1, true, false) // >=0, no max
	case "BalanceAdjustMaxAmount", "SelfApproveAlertAmount", "RefundAlertAmount":
		f, err := parseFiniteFloat(value)
		if err != nil {
			return err
		}
		if f <= 0 {
			return fmt.Errorf("%w: %s must be >0", ErrBusinessParamRange, key)
		}
		return nil
	case "EpayFailureDemoteThreshold":
		return validateIntRange(value, 1, 100)
	case "ChannelGracePeriodSeconds":
		return validateIntRange(value, 0, 3600)
	case "PayUnknownEscalateHours":
		return validateIntRange(value, 1, 168)
	case "QuotaForNewUser":
		return validateIntRange(value, 0, -1)
	case "BackupContactType":
		if value != "email" && value != "telegram" {
			return fmt.Errorf("%w: BackupContactType must be email|telegram", ErrBusinessParamFormat)
		}
		return nil
	case "BackupContact":
		return validateBackupContact(value, lookupProposedOrCurrent("BackupContactType", proposed))
	case "TopupOrderRateLimitNum":
		return validateIntRange(value, 1, 100)
	case "TopupOrderRateLimitDuration":
		return validateIntRange(value, 10, 3600)
	case "DiskAlertWarnPct":
		if err := validateIntRange(value, 50, 95); err != nil {
			return err
		}
		warn, _ := strconv.Atoi(value)
		crit := currentIntParam("DiskAlertCritPct", proposed, bpDiskAlertCritPct)
		if crit <= warn {
			return paramRuleViolation("DiskAlertWarnPct", "less_than", "DiskAlertCritPct",
				"DiskAlertWarnPct must be < DiskAlertCritPct")
		}
		return nil
	case "DiskAlertCritPct":
		crit, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%w: not an integer", ErrBusinessParamFormat)
		}
		if crit < 1 || crit > 99 {
			return fmt.Errorf("%w: DiskAlertCritPct must be in 1-99", ErrBusinessParamRange)
		}
		warn := currentIntParam("DiskAlertWarnPct", proposed, bpDiskAlertWarnPct)
		if crit <= warn {
			return paramRuleViolation("DiskAlertCritPct", "greater_than", "DiskAlertWarnPct",
				"DiskAlertCritPct must be > DiskAlertWarnPct")
		}
		return nil
	case "WalletAccountReconcileFrom":
		if value == "" {
			return nil
		}
		if _, err := time.ParseInLocation("2006-01-02", value, ShanghaiLocation()); err != nil {
			return fmt.Errorf("%w: WalletAccountReconcileFrom must be YYYY-MM-DD or empty", ErrBusinessParamFormat)
		}
		return nil
	case "DailyReportTime":
		if !reDailyReportTime.MatchString(value) {
			return fmt.Errorf("%w: DailyReportTime must be HH:MM", ErrBusinessParamFormat)
		}
		return nil
	case "BackupRetentionDays":
		return validateIntRange(value, 7, 90)
	case "AlertMergeMinutes":
		return validateIntRange(value, 1, 60)
	case "RefundAlertDailyCount":
		return validateIntRange(value, 1, 100)
	default:
		_ = def
		return fmt.Errorf("%w: %s", ErrUnknownBusinessParam, key)
	}
}

// UpdateTopupBonusTiersByJSONStringDryRun validates without mutating.
func UpdateTopupBonusTiersByJSONStringDryRun(jsonStr string) error {
	var percents []float64
	if err := json.Unmarshal([]byte(jsonStr), &percents); err != nil {
		return fmt.Errorf("%w: TopupBonusTiers must be a JSON array of numbers", ErrBusinessParamFormat)
	}
	return validateTopupBonusTiers(percents)
}

func lookupProposedOrCurrent(key string, proposed map[string]string) string {
	if proposed != nil {
		if v, ok := proposed[key]; ok {
			return v
		}
	}
	common.OptionMapRWMutex.RLock()
	v := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	if v != "" {
		return v
	}
	if d, ok := businessParamDefByKey(key); ok {
		return d.Default
	}
	return ""
}

func currentIntParam(key string, proposed map[string]string, fallback int) int {
	s := lookupProposedOrCurrent(key, proposed)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func validateBackupContact(contact, typ string) error {
	switch typ {
	case "telegram":
		if !reTelegramHandle.MatchString(contact) {
			return fmt.Errorf("%w: BackupContact for telegram must be @handle", ErrBusinessParamFormat)
		}
	default: // email or unknown → email shape
		if !reEmailSimple.MatchString(contact) && !reTelegramHandle.MatchString(contact) {
			return fmt.Errorf("%w: BackupContact must be email or @handle", ErrBusinessParamFormat)
		}
		if typ == "email" && !reEmailSimple.MatchString(contact) {
			return fmt.Errorf("%w: BackupContact for email must be a valid email", ErrBusinessParamFormat)
		}
	}
	return nil
}

func parseFiniteFloat(value string) (float64, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return 0, fmt.Errorf("%w: not a number", ErrBusinessParamFormat)
	}
	f, _ := d.Float64()
	return f, nil
}

func validateFloatRange(value string, min, max float64, minIncl, maxIncl bool) error {
	f, err := parseFiniteFloat(value)
	if err != nil {
		return err
	}
	if max < 0 { // no max
		if minIncl {
			if f < min {
				return fmt.Errorf("%w: must be >= %v", ErrBusinessParamRange, min)
			}
		} else if f <= min {
			return fmt.Errorf("%w: must be > %v", ErrBusinessParamRange, min)
		}
		return nil
	}
	if minIncl {
		if f < min {
			return fmt.Errorf("%w: must be >= %v", ErrBusinessParamRange, min)
		}
	} else if f <= min {
		return fmt.Errorf("%w: must be > %v", ErrBusinessParamRange, min)
	}
	if maxIncl {
		if f > max {
			return fmt.Errorf("%w: must be <= %v", ErrBusinessParamRange, max)
		}
	} else if f >= max {
		return fmt.Errorf("%w: must be < %v", ErrBusinessParamRange, max)
	}
	return nil
}

func validateIntRange(value string, min, max int) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%w: not an integer", ErrBusinessParamFormat)
	}
	if n < min {
		return fmt.Errorf("%w: must be >= %d", ErrBusinessParamRange, min)
	}
	if max >= 0 && n > max {
		return fmt.Errorf("%w: must be <= %d", ErrBusinessParamRange, max)
	}
	return nil
}

// BusinessParamCurrentValue returns string value from OptionMap or default / live official.
func BusinessParamCurrentValue(key string) string {
	common.OptionMapRWMutex.RLock()
	v := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	if v != "" {
		return v
	}
	switch key {
	case "Price":
		return strconv.FormatFloat(operation_setting.Price, 'f', -1, 64)
	case "QuotaForNewUser":
		return strconv.Itoa(common.QuotaForNewUser)
	case "FirstTopupBonusRate":
		return strconv.FormatFloat(GetFirstTopupBonusRate(), 'f', -1, 64)
	case "TopupBonusTiers":
		b, _ := json.Marshal(GetTopupBonusTierPercents())
		return string(b)
	}
	if d, ok := businessParamDefByKey(key); ok {
		return d.Default
	}
	return ""
}

// ListBusinessParamsWithValues builds API rows.
type BusinessParamRow struct {
	BusinessParamDef
	Value string `json:"value"`
}

func ListBusinessParamsWithValues() []BusinessParamRow {
	cat := BusinessParamCatalog()
	out := make([]BusinessParamRow, 0, len(cat))
	for _, d := range cat {
		out = append(out, BusinessParamRow{BusinessParamDef: d, Value: BusinessParamCurrentValue(d.Key)})
	}
	return out
}

// ApplyBusinessParamUpdates validates every key against the proposed set, then
// persists all values in one DB transaction. OptionMap / runtime are updated
// only after the transaction commits, so a validation or write failure leaves
// both DB and memory unchanged for the whole batch.
func ApplyBusinessParamUpdates(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	if err := RejectOptionSecretPrefix(values); err != nil {
		return err
	}
	// 按 key 排序校验：同一批多处出错时稳定报第一个出错的 key（而不是 map 迭代顺序）
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		if !IsBusinessParamKey(key) {
			return &BusinessParamError{Key: key, Value: value, Err: fmt.Errorf("%w: %s", ErrUnknownBusinessParam, key)}
		}
		if err := ValidateBusinessParam(key, value, values); err != nil {
			return &BusinessParamError{Key: key, Value: value, Err: err}
		}
	}
	dbValues, err := encodeOptionValuesForDB(values)
	if err != nil {
		return err
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		for k, v := range dbValues {
			option := Option{Key: k}
			if err := tx.FirstOrCreate(&option, Option{Key: k}).Error; err != nil {
				return err
			}
			option.Value = v
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for k, v := range values {
		if err := updateOptionMap(k, v); err != nil {
			return err
		}
	}
	return nil
}

// BusinessParamError 带出错 key/value 的校验错误；errors.Is 仍能匹配 ErrBusinessParamRange 等哨兵。
type BusinessParamError struct {
	Key   string
	Value string
	Err   error
}

func (e *BusinessParamError) Error() string { return e.Err.Error() }
func (e *BusinessParamError) Unwrap() error { return e.Err }

// BusinessParamCodedError 自带稳定 snake_case code + params 的校验错误（如档位赠送 TopupBonusTiers），
// controller 原样透传 code/params；Error() 只给英文兜底 message。
type BusinessParamCodedError struct {
	Code    string
	Message string
	Params  map[string]any
	Err     error
}

func (e *BusinessParamCodedError) Error() string {
	if e.Err != nil {
		return e.Err.Error() + ": " + e.Message
	}
	return e.Message
}
func (e *BusinessParamCodedError) Unwrap() error { return e.Err }

// paramRuleViolation 跨字段规则失败：code=param_rule_violation，params 恰好 {key, rule, other_key}（不带 value/min/max）。
// 仍 Unwrap 到 ErrBusinessParamRange，老的 errors.Is 判断不受影响。
func paramRuleViolation(key, rule, otherKey, msg string) error {
	return &BusinessParamCodedError{
		Code:    "param_rule_violation",
		Message: msg,
		Params:  map[string]any{"key": key, "rule": rule, "other_key": otherKey},
		Err:     ErrBusinessParamRange,
	}
}

// NormalizeBusinessParamValue stringifies JSON numbers etc. for storage.
func NormalizeBusinessParamValue(key, value string) (string, error) {
	def, ok := businessParamDefByKey(key)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownBusinessParam, key)
	}
	value = strings.TrimSpace(value)
	switch def.Type {
	case BPTypeNumber:
		// normalize via decimal to avoid float junk; ints stay ints when whole
		d, err := decimal.NewFromString(value)
		if err != nil {
			return "", fmt.Errorf("%w: not a number", ErrBusinessParamFormat)
		}
		if d.Equal(d.Truncate(0)) && !strings.Contains(value, ".") {
			return d.StringFixed(0), nil
		}
		return d.String(), nil
	case BPTypeEnum, BPTypeString:
		return value, nil
	case BPTypeJSON:
		var raw any
		if err := json.Unmarshal([]byte(value), &raw); err != nil {
			return "", err
		}
		b, err := json.Marshal(raw)
		if err != nil {
			return "", err
		}
		return string(b), nil
	default:
		return value, nil
	}
}

// GetWalletAccountReconcileFrom B-03 M-B：按账户对账的切换日期（上海日期，空=关闭）。
func GetWalletAccountReconcileFrom() string {
	bpMu.RLock()
	defer bpMu.RUnlock()
	return bpWalletAccountReconcileFrom
}

// SetWalletAccountReconcileFromForTest 测试 / accept 用。
func SetWalletAccountReconcileFromForTest(v string) {
	bpMu.Lock()
	defer bpMu.Unlock()
	bpWalletAccountReconcileFrom = v
}
