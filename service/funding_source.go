package service

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包 or 订阅）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet" 或 "subscription"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

// ErrInsufficientWalletQuota 钱包原子预扣失败（余额不足），未发生任何扣减。
// BillingSession 据此映射为 ErrorCodeInsufficientUserQuota，
// 使 wallet_first 等计费偏好可以回退到订阅。
var ErrInsufficientWalletQuota = errors.New("wallet quota insufficient")

type WalletFunding struct {
	userId        int
	consumed      int // 实际预扣合计
	bonusConsumed int // 其中来自赠送
	paidConsumed  int // 其中来自实充
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

// NewWalletFundingForAccept ADTY B-03 accept 用：直接走真实 WalletFunding 预扣/结算/退回。
func NewWalletFundingForAccept(userId int) *WalletFunding { return &WalletFunding{userId: userId} }

// WalletSplitForAccept accept 用：当前净出资拆分。
func (w *WalletFunding) WalletSplitForAccept() (bonus, paid int) { return w.walletSplit() }

// walletSplit ADTY B-03：当前净出资拆分。
func (w *WalletFunding) walletSplit() (bonus, paid int) { return w.bonusConsumed, w.paidConsumed }

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	bonus, paid, reserved, err := model.TryReserveUserWallet(w.userId, amount)
	if err != nil {
		return err
	}
	if !reserved {
		return ErrInsufficientWalletQuota
	}
	w.consumed = amount
	w.bonusConsumed = bonus
	w.paidConsumed = paid
	return nil
}

func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		// 结算补扣：先赠送后实充；实充不够则整笔失败（映 ErrInsufficientWalletQuota → 403）
		bonus, paid, err := model.DecreaseUserWallet(w.userId, delta)
		if err != nil {
			if errors.Is(err, model.ErrInsufficientWalletBalance) {
				return ErrInsufficientWalletQuota
			}
			return err
		}
		model.ApplyUserQuotaCacheDelta(w.userId, -int64(delta))
		w.bonusConsumed += bonus
		w.paidConsumed += paid
		w.consumed += delta
		return nil
	}
	// delta < 0：多扣退回。按原扣顺序反向：先退实充再退赠送。
	refund := -delta
	fromPaid := refund
	if fromPaid > w.paidConsumed {
		fromPaid = w.paidConsumed
	}
	fromBonus := refund - fromPaid
	if fromBonus > w.bonusConsumed {
		fromBonus = w.bonusConsumed
		fromPaid = refund - fromBonus
	}
	// ADTY B-03 H-A：请求内退回，不写流水
	if err := model.ReturnReservedUserWallet(w.userId, fromBonus, fromPaid); err != nil {
		return err
	}
	w.bonusConsumed -= fromBonus
	w.paidConsumed -= fromPaid
	w.consumed -= refund
	return nil
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 && w.bonusConsumed <= 0 && w.paidConsumed <= 0 {
		return nil
	}
	// 按原拆分退回赠送/实充，不能整笔加回实充。ADTY B-03 H-A：请求内退回，不写流水。
	if err := model.ReturnReservedUserWallet(w.userId, w.bonusConsumed, w.paidConsumed); err != nil {
		return err
	}
	w.bonusConsumed, w.paidConsumed, w.consumed = 0, 0, 0
	return nil
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

type SubscriptionFunding struct {
	requestId      string
	userId         int
	modelName      string
	amount         int64 // 预扣的订阅额度（subConsume）
	subscriptionId int
	preConsumed    int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
}

func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (s *SubscriptionFunding) PreConsume(_ int) error {
	// amount 参数被忽略，使用内部 s.amount（已在构造时根据 preConsumedQuota 计算）
	res, err := model.PreConsumeUserSubscription(s.requestId, s.userId, s.modelName, 0, s.amount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return model.PostConsumeUserSubscriptionDelta(s.subscriptionId, int64(delta))
}

func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := range maxAttempts {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}
