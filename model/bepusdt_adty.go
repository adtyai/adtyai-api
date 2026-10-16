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

var (
	ErrUsdtUnderpay       = errors.New("usdt underpay beyond tolerance")
	ErrUsdtOrderExpired   = errors.New("usdt order expired")
	ErrUsdtPaymentInvalid = errors.New("usdt payment invalid")
)

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

// MarkTopupOrderNeedManual sets status need_manual with reason (underpay / timeout); no credit.
func MarkTopupOrderNeedManual(orderNo, reason string) (*TopupOrder, error) {
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
			out = &order
			return nil
		}
		if order.Status != TopupOrderStatusPending {
			return ErrTopupOrderStatusInvalid
		}
		order.Status = TopupOrderStatusNeedManual
		order.PendingReason = reason
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		out = &order
		return nil
	})
	return out, err
}

// EvaluateUsdtCallbackCredit decides whether a verified paid callback may CompleteTopupOrder.
func EvaluateUsdtCallbackCredit(order *TopupOrder, paidUSDT float64, nowUnix int64) error {
	if order == nil {
		return ErrTopupOrderNotFound
	}
	if order.Status == TopupOrderStatusPaid {
		return nil
	}
	// need_manual (underpay/timeout) never auto-credits again; admin补单 only.
	if order.Status == TopupOrderStatusNeedManual {
		return ErrUsdtPaymentInvalid
	}
	if order.Status != TopupOrderStatusPending {
		return ErrTopupOrderStatusInvalid
	}
	if order.UsdtExpireAt > 0 && nowUnix > order.UsdtExpireAt {
		return ErrUsdtOrderExpired
	}
	want := order.PayUSDT
	if want <= 0 {
		return ErrUsdtPaymentInvalid
	}
	if paidUSDT < 0 {
		return ErrUsdtPaymentInvalid
	}
	tol := GetUsdtUnderpayTolerance()
	short := want - paidUSDT
	if short > tol {
		return ErrUsdtUnderpay
	}
	return nil
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
