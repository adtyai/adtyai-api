package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time"`
	Status          string  `json:"status"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
)

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

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
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

// RechargeEpay 原子完成易支付订单：订单行锁、状态校验、成功更新与用户额度增加
// 在同一个事务内完成，因此同一订单的并发/重复回调（包括多实例部署下）最多充值一次。
// alreadyDone=true 表示订单此前已完成，本次为幂等重复回调。
// 进程内的 LockOrder 只是优化，正确性由本函数的数据库行锁保证。
func RechargeEpay(tradeNo string, actualPaymentMethod string, paidMoney decimal.Decimal, callerIp string) (alreadyDone bool, err error) {
	if tradeNo == "" {
		return false, errors.New("payment order number not provided")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var quotaToAdd int
	topUp := &TopUp{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderEpay {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if actualPaymentMethod != "" && topUp.PaymentMethod != actualPaymentMethod {
			topUp.PaymentMethod = actualPaymentMethod
		}
		if !paidMoney.Round(2).Equal(decimal.NewFromFloat(topUp.Money).Round(2)) {
			return ErrPaymentAmountMismatch
		}
		var quotaErr error
		quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		return creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil)
	})
	if err != nil {
		if !errors.Is(err, ErrTopUpNotFound) && !errors.Is(err, ErrPaymentMethodMismatch) && !errors.Is(err, ErrTopUpStatusInvalid) {
			common.SysError(common.LogText("epay topup failed: %s", err.Error()))
		}
		return false, err
	}
	if alreadyDone {
		return true, nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, common.LogText("epay topup"))

	common.SysLog(common.LogText("Epay top-up succeeded trade_no=%s user_id=%d quota_to_add=%d money=%.2f", topUp.TradeNo, topUp.UserId, quotaToAdd, topUp.Money))
	RecordTopupLog(topUp.UserId, common.NewMessage("Online top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"quota": logger.FormatQuota(quotaToAdd), "amount": fmt.Sprintf("%f", topUp.Money)}), callerIp, topUp.PaymentMethod, PaymentProviderEpay)
	return false, nil
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("payment order number not provided")
	}

	var quota int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("top-up order not found")
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("invalid top-up order status")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quota, err = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}
		return creditTopUpQuota(tx, topUp.UserId, quota, map[string]any{
			"stripe_customer": customerId,
		})
	})

	if err != nil {
		common.SysError(common.LogText("topup failed: %s", err.Error()))
		return errors.New("top-up failed")
	}
	syncCreditUserQuotaCache(topUp.UserId, quota, common.LogText("stripe topup"))

	RecordTopupLog(topUp.UserId, common.NewMessage("Online top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"quota": logger.FormatQuota(quota), "amount": fmt.Sprintf("%.2f", topUp.Money)}), callerIp, topUp.PaymentMethod, PaymentMethodStripe)

	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
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

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
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

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return common.NewMessage("Order number not provided")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string
	var credited bool

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return common.NewMessage("Top-up order does not exist")
		}
		// 二开：我们的订单只能走充值对账入账（面额加赠、返佣、流水），任务 3.2
		adtyOrder := topUp.PaymentProvider == PaymentProviderAdtyBEpusdt || topUp.PaymentProvider == PaymentProviderAdtyEpay
		if !adtyOrder && IsAdtyPaymentProvider(topUp.PaymentProvider) {
			var count int64
			if err := tx.Model(&AdtyTopupOrder{}).Where("trade_no = ?", tradeNo).Count(&count).Error; err != nil {
				return err
			}
			adtyOrder = count > 0
		}
		if adtyOrder {
			return common.NewMessage("Use Recharge Audit to complete this order")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return common.NewMessage("Order is not pending payment and cannot be completed")
		}

		// 计算应充值额度。Stripe 的 Amount 是购买单位，单价、分组倍率和折扣只写在 Money 里。
		// Creem 的 Amount 已经是额度。其它订单的 Amount 是美元数量。后两类都再乘 QuotaPerUnit。
		var quotaErr error
		switch topUp.PaymentProvider {
		case PaymentProviderCreem:
			// Creem 的 Amount 已经是额度，和 RechargeCreem 同一口径，不能再乘 QuotaPerUnit。
			quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(topUp.Amount))
		default:
			quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(
				decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
		}
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		credited = true
		return nil
	})

	if err != nil {
		return err
	}
	if !credited {
		return nil
	}

	// 事务外记录日志，避免阻塞
	syncCreditUserQuotaCache(userId, quotaToAdd, common.LogText("manual topup"))
	RecordTopupLog(userId, common.NewMessage("Administrator completed the order, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"quota": logger.FormatQuota(quotaToAdd), "amount": fmt.Sprintf("%f", payMoney)}), callerIp, paymentMethod, "admin")
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("payment order number not provided")
	}

	var quota int
	var alreadyDone bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return ErrTopUpNotFound
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota, err = common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(topUp.Amount))
		if err != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]any{}

		// 回调邮箱只在账号还没有邮箱、且没有别人占用时写入。占用时仍入账，不把别人的邮箱记到这个账号上。
		if customerEmail != "" {
			var user User
			err = tx.Select("id", "email").Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}
			normalized := NormalizeEmail(customerEmail)
			if user.Email == "" && normalized != "" {
				err = withNormalizedEmailLock(tx, normalized, func(locked *gorm.DB) error {
					availableErr := ensureEmailAvailableWithTx(locked, normalized, topUp.UserId)
					if errors.Is(availableErr, ErrEmailAlreadyTaken) {
						common.SysLog(common.LogText("creem callback email already taken trade_no=%s user_id=%d", topUp.TradeNo, topUp.UserId))
						return nil
					}
					if availableErr != nil {
						return availableErr
					}
					updateFields["email"] = normalized
					return nil
				})
				if err != nil {
					return err
				}
			}
		}

		return creditTopUpQuota(tx, topUp.UserId, quota, updateFields)
	})

	if err != nil {
		common.SysError(common.LogText("creem topup failed: %s", err.Error()))
		return err
	}
	if alreadyDone {
		return nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quota, common.LogText("creem topup"))

	RecordTopupLog(topUp.UserId, common.NewMessage("{{provider}} top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"provider": "Creem", "quota": quota, "amount": fmt.Sprintf("%.2f", topUp.Money)}), callerIp, topUp.PaymentMethod, PaymentMethodCreem)

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("payment order number not provided")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("top-up order not found")
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess || topUp.Status == common.TopUpStatusRefunded {
			return nil // 已入账或已退款都不再入账
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("invalid top-up order status")
		}

		quotaToAdd, err = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		return creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil)
	})

	if err != nil {
		common.SysError(common.LogText("waffo topup failed: %s", err.Error()))
		return errors.New("top-up failed")
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, common.LogText("waffo topup"))

	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, common.NewMessage("{{provider}} top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"provider": "Waffo", "quota": logger.FormatQuota(quotaToAdd), "amount": fmt.Sprintf("%.2f", topUp.Money)}), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("payment order number not provided")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("top-up order not found")
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("invalid top-up order status")
		}

		quotaToAdd, err = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		return creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil)
	})

	if err != nil {
		common.SysError(common.LogText("waffo pancake topup failed: %s", err.Error()))
		return errors.New("top-up failed")
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, common.LogText("waffo pancake topup"))

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, common.NewMessage("{{provider}} top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"provider": "Waffo Pancake", "quota": logger.FormatQuota(quotaToAdd), "amount": fmt.Sprintf("%.2f", topUp.Money)}))
	}

	return nil
}

// RefundWaffo 收回一笔 Waffo 已入账额度。fullRefund 为真时扣回剩余全部额度并标成已退款。
// 部分退款按本次金额占订单金额的比例扣，并用 idemKey 保证同一笔退款重放不再扣。余额不够时只扣到 0。
func RefundWaffo(tradeNo string, idemKey string, fullRefund bool, refundAmount string) error {
	if tradeNo == "" {
		return errors.New("payment order number not provided")
	}
	idemKey = strings.TrimSpace(idemKey)
	if idemKey == "" || len(idemKey) > 64 {
		return errors.New("refund request id required")
	}
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}
	var clawed int
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}
		existing, err := FindAdtyLedgerByIdemKeyTx(tx, idemKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Type != AdtyLedgerTypeRefund || existing.RefId != tradeNo {
				return ErrAdtyIdempotencyConflict
			}
			return nil
		}
		if topUp.Status == common.TopUpStatusRefunded {
			return nil
		}
		if topUp.Status != common.TopUpStatusSuccess {
			topUp.Status = common.TopUpStatusRefunded
			return tx.Save(topUp).Error
		}
		credited, err := common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || credited <= 0 {
			return ErrInvalidTopUpQuota
		}
		var already int64
		if err := tx.Raw(
			`SELECT COALESCE(SUM(-delta_quota), 0) FROM adty_balance_ledgers WHERE ref_id = ? AND type = ?`,
			tradeNo, AdtyLedgerTypeRefund,
		).Scan(&already).Error; err != nil {
			return err
		}
		if already < 0 {
			already = 0
		}
		remaining := int64(credited) - already
		if remaining < 0 {
			remaining = 0
		}
		claw, err := waffoRefundClaw(topUp.Money, refundAmount, fullRefund, remaining, int64(credited))
		if err != nil {
			return err
		}
		var user User
		if err := lockForUpdate(tx).First(&user, topUp.UserId).Error; err != nil {
			return err
		}
		balance, err := userWalletBalance(user.Quota, user.QuotaPending)
		if err != nil {
			return err
		}
		if int64(balance) < claw {
			claw = int64(balance)
		}
		if claw < 0 {
			claw = 0
		}
		userId = topUp.UserId
		clawed = int(claw)
		if claw > 0 {
			if err := applyUserQuotaDeltaTx(tx, &user, -int(claw), false); err != nil {
				return err
			}
		}
		if fullRefund || remaining-claw <= 0 {
			topUp.Status = common.TopUpStatusRefunded
			if err := tx.Save(topUp).Error; err != nil {
				return err
			}
		}
		key := idemKey
		before := int64(balance)
		return InsertAdtyLedgerTx(tx, &AdtyBalanceLedger{
			UserId:         user.Id,
			DeltaQuota:     -claw,
			BalanceBefore:  before,
			BalanceAfter:   before - claw,
			Type:           AdtyLedgerTypeRefund,
			RefType:        "topup",
			RefId:          tradeNo,
			Reason:         "waffo refund",
			IdempotencyKey: &key,
		})
	})
	if err != nil {
		return err
	}
	if clawed > 0 {
		if err := invalidateUserCache(userId); err != nil {
			common.SysError(common.LogText("failed to invalidate refunded wallet for user %d: %s", userId, err.Error()))
		}
	}
	return nil
}

func waffoRefundClaw(orderMoney float64, refundAmount string, fullRefund bool, remaining int64, credited int64) (int64, error) {
	if remaining <= 0 {
		return 0, nil
	}
	if fullRefund {
		return remaining, nil
	}
	money := decimal.NewFromFloat(orderMoney)
	if !money.IsPositive() {
		return 0, ErrInvalidTopUpQuota
	}
	paid, err := decimal.NewFromString(strings.TrimSpace(refundAmount))
	if err != nil || !paid.IsPositive() {
		return 0, ErrPaymentAmountMismatch
	}
	if paid.GreaterThanOrEqual(money) {
		return remaining, nil
	}
	portion := decimal.NewFromInt(credited).Mul(paid).Div(money)
	claw, err := common.WalletQuotaFromDecimalStrict(portion)
	if err != nil {
		return 0, err
	}
	if int64(claw) > remaining {
		return remaining, nil
	}
	return int64(claw), nil
}

// RefundWaffoPancake 收回一笔已入账的 Pancake 额度，最多扣到 0。重复退款不再扣。
func RefundWaffoPancake(tradeNo string) error {
	if tradeNo == "" {
		return errors.New("payment order number not provided")
	}
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}
	var clawed int
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return err
		}
		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusRefunded {
			return nil
		}
		userId = topUp.UserId
		if topUp.Status == common.TopUpStatusSuccess {
			quota, err := common.WalletQuotaFromDecimalStrict(
				decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
			if err != nil || quota <= 0 {
				return ErrInvalidTopUpQuota
			}
			var user User
			if err := lockForUpdate(tx).First(&user, topUp.UserId).Error; err != nil {
				return err
			}
			balance, err := userWalletBalance(user.Quota, user.QuotaPending)
			if err != nil {
				return err
			}
			clawed = max(0, min(balance, quota))
			if clawed > 0 {
				if err := applyUserQuotaDeltaTx(tx, &user, -clawed, false); err != nil {
					return err
				}
			}
		}
		topUp.Status = common.TopUpStatusRefunded
		return tx.Save(topUp).Error
	})
	if err != nil {
		return err
	}
	if clawed > 0 {
		if err := invalidateUserCache(userId); err != nil {
			common.SysError(common.LogText("failed to invalidate refunded wallet for user %d: %s", userId, err.Error()))
		}
	}
	return nil
}
