package model

import (
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TopupOrderStatusNeedManual = "need_manual"
	PaymentProviderBepusdt     = "bepusdt"
)

const (
	UsdtPaidCallbackCredit     = "credit"
	UsdtPaidCallbackIdempotent = "idempotent"
	UsdtPaidCallbackManual     = "manual"
	UsdtPaidCallbackRetry      = "retry"
	usdtRemainingSecondsMax    = int64(7 * 24 * 3600)
)

var (
	ErrUsdtUnderpay       = errors.New("usdt underpay beyond tolerance")
	ErrUsdtOrderExpired   = errors.New("usdt order expired")
	ErrUsdtPaymentInvalid = errors.New("usdt payment invalid")
)

// UsdtPaidCallbackDecision is the merchant action for a verified BEpusdt status-2 callback.
// Chain matching stays with BEpusdt. ADTY only credits, retries, or hands a ledger conflict to review.
type UsdtPaidCallbackDecision struct {
	Action string
	Reason string
}

// ApplyUsdtSnapshotToOrder writes locked USDT amount / fx / expiry onto pending order.
func ApplyUsdtSnapshotToOrder(order *TopupOrder, payUSDT, usdtFx float64, expireAt int64, tradeID string) error {
	if order == nil {
		return ErrTopupOrderNotFound
	}
	updates := map[string]any{
		"payment_provider": PaymentProviderBepusdt,
		"pay_usdt":         payUSDT,
		"usdt_fx_rate":     usdtFx,
		"usdt_expire_at":   expireAt,
		"fx_rate":          CurrentTopupFxRate(),
	}
	// provider_trade_no is bound only on paid callback (unique trade hash), not at create.
	_ = tradeID
	if err := DB.Model(order).Updates(updates).Error; err != nil {
		return err
	}
	order.PaymentProvider = PaymentProviderBepusdt
	order.PayUSDT = payUSDT
	order.UsdtFxRate = usdtFx
	order.UsdtExpireAt = expireAt
	return nil
}

// MarkTopupOrderNeedManual sets status need_manual for a ledger conflict. It does not credit.
// paidUSDT < 0 means the callback had no usable amount and is not stored.
// A later callback may fill a missing amount, but does not overwrite the first stored amount or reason.
// Paid orders are never moved here. Failed or expired rows can be marked so a conflicting success is visible.
func MarkTopupOrderNeedManual(orderNo, reason string, paidUSDT float64) (*TopupOrder, error) {
	var out *TopupOrder
	err := DB.Transaction(func(tx *gorm.DB) error {
		var order TopupOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopupOrderNotFound
			}
			return err
		}
		if order.Status == TopupOrderStatusPaid {
			out = &order
			return nil
		}
		if order.Status == TopupOrderStatusNeedManual {
			if order.UsdtCallbackAmount == nil && paidUSDT >= 0 {
				amount := paidUSDT
				order.UsdtCallbackAmount = &amount
				if err := tx.Model(&order).Update("usdt_callback_amount", amount).Error; err != nil {
					return err
				}
			}
			out = &order
			return nil
		}
		if order.Status != TopupOrderStatusPending && order.Status != TopupOrderStatusFailed && order.Status != TopupOrderStatusExpired {
			return ErrTopupOrderStatusInvalid
		}
		order.Status = TopupOrderStatusNeedManual
		order.PendingReason = reason
		if paidUSDT >= 0 {
			amount := paidUSDT
			order.UsdtCallbackAmount = &amount
		}
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		out = &order
		return nil
	})
	return out, err
}

// UsdtExpireAtFromBepusdtRemaining converts create-transaction expiration_time.
// BEpusdt returns remaining seconds, not a unix timestamp. A timestamp-shaped or
// otherwise unusable value falls back to the configured payment window.
func UsdtExpireAtFromBepusdtRemaining(remainingSeconds int64, now time.Time) int64 {
	if remainingSeconds > 0 && remainingSeconds <= usdtRemainingSecondsMax {
		return now.Add(time.Duration(remainingSeconds) * time.Second).Unix()
	}
	return DefaultUsdtExpireAt(now)
}

// DecideUsdtPaidCallback does not compare chain amount or the local clock.
// Status 2 on a still-open order credits the snapshotted USD. Short pay is audit-only.
func DecideUsdtPaidCallback(order *TopupOrder, paidUSDT float64) UsdtPaidCallbackDecision {
	if order == nil {
		return UsdtPaidCallbackDecision{Action: UsdtPaidCallbackRetry, Reason: "not_found"}
	}
	switch order.Status {
	case TopupOrderStatusPaid:
		return UsdtPaidCallbackDecision{Action: UsdtPaidCallbackIdempotent}
	case TopupOrderStatusNeedManual:
		return UsdtPaidCallbackDecision{Action: UsdtPaidCallbackManual, Reason: "already_manual"}
	case TopupOrderStatusFailed:
		return UsdtPaidCallbackDecision{Action: UsdtPaidCallbackManual, Reason: "paid_after_failed"}
	case TopupOrderStatusPending, TopupOrderStatusExpired:
		if paidUSDT < 0 {
			return UsdtPaidCallbackDecision{Action: UsdtPaidCallbackRetry, Reason: "missing_actual_amount"}
		}
		return UsdtPaidCallbackDecision{Action: UsdtPaidCallbackCredit}
	default:
		return UsdtPaidCallbackDecision{Action: UsdtPaidCallbackManual, Reason: "status_" + order.Status}
	}
}

// EvaluateUsdtCallbackCredit reports whether a verified status-2 callback may credit.
// nowUnix is ignored: the local payment window is display-only.
func EvaluateUsdtCallbackCredit(order *TopupOrder, paidUSDT float64, nowUnix int64) error {
	_ = nowUnix
	if order == nil {
		return ErrTopupOrderNotFound
	}
	switch DecideUsdtPaidCallback(order, paidUSDT).Action {
	case UsdtPaidCallbackCredit, UsdtPaidCallbackIdempotent:
		return nil
	default:
		return ErrUsdtPaymentInvalid
	}
}

func ParsePayUSDTString(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func DefaultUsdtExpireAt(now time.Time) int64 {
	mins := GetBepusdtOrderTimeoutMinutes()
	if mins <= 0 {
		mins = 15
	}
	return now.Add(time.Duration(mins) * time.Minute).Unix()
}

// BepusdtApiTokenSecretMeta for GET options: secret_set + hm_ fingerprint (no plaintext).
func BepusdtApiTokenSecretMeta() (secretSet bool, fingerprint string) {
	tok := setting.GetBepusdtApiToken()
	if tok == "" {
		return false, ""
	}
	fp, err := common.AdtySecretFingerprint(tok)
	if err != nil {
		return true, ""
	}
	return true, "hm_" + fp
}

func IsBepusdtConfigured() bool { return setting.IsBepusdtConfigured() }

func IsBepusdtLegacyNotifyEnabled() bool { return setting.IsBepusdtLegacyNotifyEnabled() }
