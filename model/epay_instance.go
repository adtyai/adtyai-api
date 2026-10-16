package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	EpayInstanceStatusEnabled  = "enabled"
	EpayInstanceStatusDisabled = "disabled"
)

var (
	ErrEpayInstanceNotFound     = errors.New("epay instance not found")
	ErrEpayNoPrimary            = errors.New("no usable primary epay instance")
	ErrEpayConfirmRequired      = errors.New("confirm=true required")
	ErrEpaySecretRequired       = errors.New("secret required")
	ErrEpayTradeNoDuplicate     = errors.New("epay trade_no already used")
	ErrEpayInstanceDisabled     = errors.New("epay instance disabled")
	ErrEpayDailyLimitExceeded   = errors.New("epay daily limit exceeded")
	ErrEpayPaymentUnavailable   = errors.New("payment under maintenance")
)

// EpayInstance 易支付多实例（A9）。密钥只存密文，响应绝不回传 secret。
type EpayInstance struct {
	Id                 int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name               string     `json:"name" gorm:"type:varchar(128);not null"`
	PartnerId          string     `json:"partner_id" gorm:"type:varchar(64);not null;column:partner_id"` // 商户号 pid
	SecretCiphertext   string     `json:"-" gorm:"type:text;not null"`
	SecretFingerprint  string     `json:"secret_fingerprint" gorm:"type:varchar(16);not null;default:''"`
	PayAddress         string     `json:"pay_address" gorm:"type:varchar(512);not null"` // 网关
	CallbackBase       string     `json:"callback_base" gorm:"type:varchar(512);default:''"`
	DailyLimit         int64      `json:"daily_limit" gorm:"not null;default:0"` // 0=不限
	Status             string     `json:"status" gorm:"type:varchar(32);not null;default:'enabled';index"`
	IsPrimary          bool       `json:"is_primary" gorm:"not null;default:false;index"`
	FailureStreak      int        `json:"failure_streak" gorm:"not null;default:0"`
	DemotedReason      *string    `json:"demoted_reason" gorm:"type:text"`
	DemotedAt          *time.Time `json:"demoted_at"`
	LastFailureAt      *time.Time `json:"last_failure_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (EpayInstance) TableName() string { return "epay_instances" }

// EpayInstanceView 对外安全视图：无密文/明文，仅 secret_set + fingerprint。
type EpayInstanceView struct {
	Id                int64      `json:"id"`
	Name              string     `json:"name"`
	PartnerId         string     `json:"partner_id"`
	PayAddress        string     `json:"pay_address"`
	CallbackBase      string     `json:"callback_base"`
	DailyLimit        int64      `json:"daily_limit"`
	Status            string     `json:"status"`
	IsPrimary         bool       `json:"is_primary"`
	FailureStreak     int        `json:"failure_streak"`
	DemotedReason     *string    `json:"demoted_reason"`
	DemotedAt         *time.Time `json:"demoted_at"`
	LastFailureAt     *time.Time `json:"last_failure_at"`
	SecretSet         bool       `json:"secret_set"`
	SecretFingerprint string     `json:"secret_fingerprint"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (e *EpayInstance) ToView() EpayInstanceView {
	return EpayInstanceView{
		Id:                e.Id,
		Name:              e.Name,
		PartnerId:         e.PartnerId,
		PayAddress:        e.PayAddress,
		CallbackBase:      e.CallbackBase,
		DailyLimit:        e.DailyLimit,
		Status:            e.Status,
		IsPrimary:         e.IsPrimary,
		FailureStreak:     e.FailureStreak,
		DemotedReason:     e.DemotedReason,
		DemotedAt:         e.DemotedAt,
		LastFailureAt:     e.LastFailureAt,
		SecretSet:         e.SecretCiphertext != "",
		SecretFingerprint: e.SecretFingerprint,
		CreatedAt:         e.CreatedAt,
		UpdatedAt:         e.UpdatedAt,
	}
}

// EnsureEpayInstanceIndexes 部分唯一：任意时刻最多一套主用。
func EnsureEpayInstanceIndexes(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		// 非 PG 仅尽力；验收以 PG 为准
		return nil
	}
	return db.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS uq_epay_instances_one_primary
ON epay_instances (is_primary) WHERE is_primary = true
`).Error
}

// EnsureTopupOrderIndexes provider_trade_no 部分唯一，防并发双绑（空串/NULL 不占唯一）。
func EnsureTopupOrderIndexes(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return nil
	}
	return db.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS uq_topup_orders_provider_trade_no
ON topup_orders (provider_trade_no)
WHERE provider_trade_no IS NOT NULL AND provider_trade_no <> ''
`).Error
}

func GetEpayInstanceById(id int64) (*EpayInstance, error) {
	var row EpayInstance
	if err := DB.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEpayInstanceNotFound
		}
		return nil, err
	}
	return &row, nil
}

func ListEpayInstances() ([]EpayInstance, error) {
	var rows []EpayInstance
	if err := DB.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type CreateEpayInstanceInput struct {
	Name         string
	PartnerId    string
	Secret       string // plaintext, never stored as-is
	PayAddress   string
	CallbackBase string
	DailyLimit   int64
	Enabled      bool
}

func CreateEpayInstance(in CreateEpayInstanceInput) (*EpayInstance, error) {
	if in.Secret == "" {
		return nil, ErrEpaySecretRequired
	}
	if in.Name == "" || in.PartnerId == "" || in.PayAddress == "" {
		return nil, fmt.Errorf("name, partner_id and pay_address required")
	}
	ct, err := common.EncryptAdtySecret(in.Secret)
	if err != nil {
		return nil, err
	}
	fp, err := common.AdtySecretFingerprint(in.Secret)
	if err != nil {
		return nil, err
	}
	status := EpayInstanceStatusEnabled
	if !in.Enabled {
		status = EpayInstanceStatusDisabled
	}
	row := &EpayInstance{
		Name:              in.Name,
		PartnerId:         in.PartnerId,
		SecretCiphertext:  ct,
		SecretFingerprint: fp,
		PayAddress:        in.PayAddress,
		CallbackBase:      in.CallbackBase,
		DailyLimit:        in.DailyLimit,
		Status:            status,
		IsPrimary:         false,
		FailureStreak:     0,
	}
	if err := DB.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

type UpdateEpayInstanceInput struct {
	Name         *string
	PartnerId    *string
	Secret       *string // nil = keep; non-nil empty = reject
	PayAddress   *string
	CallbackBase *string
	DailyLimit   *int64
	Enabled      *bool
}

func UpdateEpayInstance(id int64, in UpdateEpayInstanceInput) (*EpayInstance, error) {
	row, err := GetEpayInstanceById(id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		row.Name = *in.Name
	}
	if in.PartnerId != nil {
		row.PartnerId = *in.PartnerId
	}
	if in.PayAddress != nil {
		row.PayAddress = *in.PayAddress
	}
	if in.CallbackBase != nil {
		row.CallbackBase = *in.CallbackBase
	}
	if in.DailyLimit != nil {
		row.DailyLimit = *in.DailyLimit
	}
	if in.Enabled != nil {
		if *in.Enabled {
			row.Status = EpayInstanceStatusEnabled
		} else {
			row.Status = EpayInstanceStatusDisabled
			row.IsPrimary = false
		}
	}
	if in.Secret != nil {
		if *in.Secret == "" {
			return nil, ErrEpaySecretRequired
		}
		ct, err := common.EncryptAdtySecret(*in.Secret)
		if err != nil {
			return nil, err
		}
		fp, err := common.AdtySecretFingerprint(*in.Secret)
		if err != nil {
			return nil, err
		}
		row.SecretCiphertext = ct
		row.SecretFingerprint = fp
	}
	if err := DB.Save(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// SetEpayInstancePrimary 设主用：清其它主用，本实例 failure_streak/demoted 清零。需事务。
func SetEpayInstancePrimary(id int64) (*EpayInstance, error) {
	var out *EpayInstance
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row EpayInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEpayInstanceNotFound
			}
			return err
		}
		if row.Status != EpayInstanceStatusEnabled {
			return ErrEpayInstanceDisabled
		}
		if err := tx.Model(&EpayInstance{}).
			Where("is_primary = ?", true).
			Updates(map[string]any{"is_primary": false}).Error; err != nil {
			return err
		}
		now := time.Now()
		row.IsPrimary = true
		row.FailureStreak = 0
		row.DemotedReason = nil
		row.DemotedAt = nil
		row.UpdatedAt = now
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		out = &row
		return nil
	})
	return out, err
}

// DisableEpayInstance 停用：若是主用则顺带摘主。已停用则幂等成功。
func DisableEpayInstance(id int64) (*EpayInstance, error) {
	row, err := GetEpayInstanceById(id)
	if err != nil {
		return nil, err
	}
	row.Status = EpayInstanceStatusDisabled
	row.IsPrimary = false
	if err := DB.Save(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// EnableEpayInstance 重新启用（不自动设主用）。已启用则幂等成功。
func EnableEpayInstance(id int64) (*EpayInstance, error) {
	row, err := GetEpayInstanceById(id)
	if err != nil {
		return nil, err
	}
	row.Status = EpayInstanceStatusEnabled
	if err := DB.Save(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// GetPrimaryEpayInstance 返回当前主用（不论是否因日限额不可用）。
func GetPrimaryEpayInstance() (*EpayInstance, error) {
	var row EpayInstance
	err := DB.Where("is_primary = ? AND status = ?", true, EpayInstanceStatusEnabled).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEpayNoPrimary
		}
		return nil, err
	}
	return &row, nil
}

// CountEpayInstancePaidToday 今日该实例已支付订单数（用于日限额）。
func CountEpayInstancePaidToday(instanceId int64) (int64, error) {
	start := time.Now().In(time.Local)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	startUnix := start.Unix()
	var n int64
	err := DB.Model(&TopupOrder{}).
		Where("epay_instance_id = ? AND status = ? AND complete_time >= ?",
			instanceId, TopupOrderStatusPaid, startUnix).
		Count(&n).Error
	return n, err
}

// IsEpayInstanceUsable 主用且未超日限额。
func IsEpayInstanceUsable(inst *EpayInstance) (bool, string) {
	if inst == nil {
		return false, "支付维护中"
	}
	if !inst.IsPrimary || inst.Status != EpayInstanceStatusEnabled {
		return false, "支付维护中"
	}
	if inst.DailyLimit > 0 {
		n, err := CountEpayInstancePaidToday(inst.Id)
		if err != nil {
			return false, "支付维护中"
		}
		if n >= inst.DailyLimit {
			return false, "支付维护中（已达日限额）"
		}
	}
	return true, ""
}

// HasUsablePrimaryEpay 支付宝/微信 available 判断。
func HasUsablePrimaryEpay() (bool, string) {
	inst, err := GetPrimaryEpayInstance()
	if err != nil {
		return false, "支付维护中"
	}
	return IsEpayInstanceUsable(inst)
}

// DecryptEpayInstanceSecret 解密实例密钥明文（仅内部验签/下单用）。
func DecryptEpayInstanceSecret(inst *EpayInstance) (string, error) {
	if inst == nil {
		return "", ErrEpayInstanceNotFound
	}
	return common.DecryptAdtySecret(inst.SecretCiphertext)
}

// RecordEpayInstanceFailure 连续失败 +1，达阈值自动摘主。
func RecordEpayInstanceFailure(id int64, reason string) (*EpayInstance, bool, error) {
	demoted := false
	var out *EpayInstance
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row EpayInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEpayInstanceNotFound
			}
			return err
		}
		now := time.Now()
		row.FailureStreak++
		row.LastFailureAt = &now
		threshold := GetEpayFailureDemoteThreshold()
		if row.IsPrimary && row.FailureStreak >= threshold {
			row.IsPrimary = false
			r := reason
			if r == "" {
				r = fmt.Sprintf("failure_streak>=%d", threshold)
			}
			row.DemotedReason = &r
			row.DemotedAt = &now
			demoted = true
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		out = &row
		return nil
	})
	return out, demoted, err
}

// DemoteEpayInstancePrimary 强制摘主（验收/运维）。
func DemoteEpayInstancePrimary(id int64, reason string) (*EpayInstance, error) {
	var out *EpayInstance
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row EpayInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEpayInstanceNotFound
			}
			return err
		}
		now := time.Now()
		row.IsPrimary = false
		r := reason
		if r == "" {
			r = "manual_demote"
		}
		row.DemotedReason = &r
		row.DemotedAt = &now
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		out = &row
		return nil
	})
	return out, err
}

// ResetEpayInstanceFailureStreak 成功后清零连续失败（下单成功时调用）。
func ResetEpayInstanceFailureStreak(id int64) error {
	return DB.Model(&EpayInstance{}).Where("id = ?", id).
		Updates(map[string]any{"failure_streak": 0}).Error
}

// FindTopupOrderByProviderTradeNo 按易支付交易号查订单（补单查重）。
func FindTopupOrderByProviderTradeNo(tradeNo string) (*TopupOrder, error) {
	if tradeNo == "" {
		return nil, ErrTopupOrderNotFound
	}
	var o TopupOrder
	if err := DB.Where("provider_trade_no = ?", tradeNo).First(&o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopupOrderNotFound
		}
		return nil, err
	}
	return &o, nil
}

// AttachProviderTradeNo 入账前/补单时写入易支付交易号；若已被其它单占用则冲突。
func AttachProviderTradeNo(orderNo, tradeNo string) error {
	if tradeNo == "" {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing TopupOrder
		err := tx.Where("provider_trade_no = ? AND order_no <> ?", tradeNo, orderNo).First(&existing).Error
		if err == nil {
			return ErrEpayTradeNoDuplicate
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var order TopupOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopupOrderNotFound
			}
			return err
		}
		if order.ProviderTradeNo != "" && order.ProviderTradeNo != tradeNo {
			// 已绑其它交易号
			return ErrEpayTradeNoDuplicate
		}
		if order.ProviderTradeNo == tradeNo {
			return nil
		}
		if err := tx.Model(&order).Update("provider_trade_no", tradeNo).Error; err != nil {
			// 并发下靠部分唯一索引挡住双绑
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "uq_topup_orders_provider_trade_no") ||
				strings.Contains(msg, "duplicate key") ||
				strings.Contains(msg, "unique constraint") {
				return ErrEpayTradeNoDuplicate
			}
			return err
		}
		return nil
	})
}

// CurrentTopupFxRate 充值 CNY/USD（官方 Price）。
func CurrentTopupFxRate() float64 {
	return operation_setting.Price
}
