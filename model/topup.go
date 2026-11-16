package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int             `json:"id"`
	UserId          int             `json:"user_id" gorm:"index"`
	Amount          int64           `json:"amount"`
	Money           float64         `json:"money"`
	MoneyExact      decimal.Decimal `json:"money_exact" gorm:"type:decimal(20,8);not null;default:0"`
	TradeNo         string          `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string          `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string          `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime      int64           `json:"create_time"`
	CompleteTime    int64           `json:"complete_time"`
	Status          string          `json:"status"`
}

// CompatFaceUSD returns the exact compatibility face amount when one was
// stored on MoneyExact. Rows that only wrote the legacy float64 Money
// column fall back to that value. Neither field is a wallet or ledger
// credit source.
func (topUp *TopUp) CompatFaceUSD() decimal.Decimal {
	if topUp == nil {
		return decimal.Zero
	}
	if !topUp.MoneyExact.IsZero() {
		return topUp.MoneyExact
	}
	if topUp.Money == 0 {
		return decimal.Zero
	}
	return decimal.NewFromFloat(topUp.Money)
}

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
	// The quota errors reach the web console, so they carry message keys.
	ErrInvalidTopUpQuota        error = common.NewMessage("Invalid top-up quota")
	ErrTopUpQuotaLimitExceeded  error = common.NewMessage("Top-up quota limit exceeded")
	ErrWalletQuotaLimitExceeded error = common.NewMessage("Wallet quota limit exceeded")
)

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func topUpQuotaMaxCurrent(creditedQuota int) (int, error) {
	if creditedQuota <= 0 || creditedQuota > common.MaxWalletQuota {
		return 0, ErrInvalidTopUpQuota
	}
	return common.MaxWalletQuota - creditedQuota, nil
}

// ValidateTopUpQuotaCapacity performs the user-facing pre-payment check. The
// settlement path repeats the same invariant with an atomic conditional
// update, because the wallet balance can change after checkout creation.
func ValidateTopUpQuotaCapacity(userId int, creditedQuota int) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	var user User
	if err := DB.Select("quota", "quota_pending").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	balance, err := userWalletBalance(user.Quota, user.QuotaPending)
	if err != nil || balance > maxCurrentQuota {
		return ErrTopUpQuotaLimitExceeded
	}
	return nil
}

// creditTopUpQuota shares the owner lock with reservations, refunds, and flushes.
// Both the logical balance and the raw field addition must fit before credit.
func creditTopUpQuota(tx *gorm.DB, userId int, creditedQuota int, updates map[string]any) error {
	if _, err := topUpQuotaMaxCurrent(creditedQuota); err != nil {
		return err
	}
	var user User
	if err := lockForUpdate(tx).First(&user, userId).Error; err != nil {
		return err
	}
	if err := applyUserQuotaDeltaTx(tx, &user, creditedQuota, false); err != nil {
		if errors.Is(err, ErrWalletQuotaLimitExceeded) {
			return ErrTopUpQuotaLimitExceeded
		}
		return err
	}
	if len(updates) > 0 {
		return tx.Model(&User{}).Where("id = ?", userId).Updates(updates).Error
	}
	return nil
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	topUp, err := LookupTopUpByTradeNo(tradeNo)
	if err != nil {
		return nil
	}
	return topUp
}

// LookupTopUpByTradeNo 把「没有这张订单」和数据库故障分开。前者是永久错误，后者要让支付回调重试。
func LookupTopUpByTradeNo(tradeNo string) (*TopUp, error) {
	var topUp TopUp
	err := DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil, err
	}
	return &topUp, nil
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("payment order number not provided")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError(common.LogText("failed to count search topups: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to search top-up records")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError(common.LogText("failed to search topups: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to search top-up records")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}
