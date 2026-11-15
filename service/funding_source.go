package service

import (
	"errors"

	"github.com/QuantumNous/new-api/model"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还），返回补扣的实扣额。
	// 二开（余额下限）：补扣最多扣到 0，delta − 实扣由平台承担。
	// 钱包实现的正数分支自 ISS-R4-0054 起不再扣费，见 WalletFunding.Settle。
	Settle(delta int) (charged int, err error)
	// Refund 退还所有预扣费
	Refund() error
}

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

// ErrInsufficientWalletQuota 钱包原子预扣失败（余额不足），未发生任何扣减。
// BillingSession 据此映射为 ErrorCodeInsufficientUserQuota。
var ErrInsufficientWalletQuota = errors.New("wallet quota insufficient")

// ErrLegacyWalletCharge 表示调用方走到了「没有 operation key 的扣费」这条路。
//
// 自 ISS-R4-0054 起这里**没有扣费实现**：production 唯一的构造点总是给
// BillingSession 设 `operationKey`，实际扣费由 ApplyBillingSettlement 完成，
// 原来那条 `ChargeUserQuotaUpTo` 已连同钉住它的测试一起删除。
//
// 为什么返回错误而不是返回 (0, nil)：`BillingSession.Settle` 的 legacy 分支
// 只有在「不变量被违反」时才可达。那时静默不扣费等于免费放行，比报错坏得多；
// 报错是可见的失败，也是让下一个人去读 ISS-R4-0046 / ISS-R4-0054 的信号。
var ErrLegacyWalletCharge = errors.New("legacy wallet settle has no charge path: settlement must carry an operation key")

type WalletFunding struct {
	userId       int
	operationKey string // 本次请求的业务身份，预扣分摊按它登记，释放时按它归还
	consumed     int    // 实际预扣的用户额度
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	reserved, err := model.TryReserveUserQuotaForOperation(w.userId, amount, w.operationKey)
	if err != nil {
		return err
	}
	if !reserved {
		return ErrInsufficientWalletQuota
	}
	// 发送前补扣会再次预扣，累加后退款才能退回全部预扣。
	w.consumed += amount
	return nil
}

func (w *WalletFunding) Settle(delta int) (int, error) {
	if delta == 0 {
		return 0, nil
	}
	if delta > 0 {
		// 正数补扣在生产上不存在：带 operation key 的结算走 ApplyBillingSettlement
		// 自己扣费（含余额下限）。走到这里说明 BillingSession 的 legacy 分支被复活了。
		return 0, ErrLegacyWalletCharge
	}
	return 0, model.IncreaseUserQuota(w.userId, -delta, false)
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 && w.operationKey == "" {
		return nil
	}
	// 退钱和释放预扣分摊必须同一个事务：只退钱的话，预扣时登记的 held 会留在
	// 库里，而可用批次额度算的是 `remaining - held`，那笔占用会永久占住容量。
	// 实测见 model.RefundUserQuotaForOperation 的注释。
	//
	// 退额度是非幂等操作，不能重试，否则会多退。
	return model.RefundUserQuotaForOperation(w.userId, w.consumed, w.operationKey)
}
