package service

import (
	"errors"
	"sync"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

// ErrAdtyPaymentDetected 表示网关明确说这笔订单已经有转账或已支付。本地必须留在待支付，等回调入账。
var ErrAdtyPaymentDetected = errors.New("adty payment detected")

// AdtyMethodInfo 是钱包页的一种支付方式。
type AdtyMethodInfo struct {
	Id        string `json:"id"`   // usdt.trc20 | alipay …
	Kind      string `json:"kind"` // usdt | epay
	Available bool   `json:"available"`
	Label     string `json:"label,omitempty"`
	FaceUSD   string `json:"face_usd,omitempty"`
}

// AdtyPaymentProvider 是一个支付商（P4 的 BEpusdt、P5 的易支付）。
type AdtyPaymentProvider interface {
	Name() string
	Methods() []AdtyMethodInfo
	AvailableFor(method string, quote *AdtyQuote) bool
	// Cancel 取消支付商那边的订单。返回 ErrAdtyPaymentDetected 时本地留在待支付。
	// 网络失败、未配置和其它业务错误不代表已经收到钱，调用方仍可取消本地订单。
	Cancel(ext *model.AdtyTopupOrder) error
}

var (
	adtyProvidersMu sync.RWMutex
	adtyProviders   []AdtyPaymentProvider
)

// RegisterAdtyPaymentProvider 登记支付商；同名的后登记的替换先登记的。
func RegisterAdtyPaymentProvider(provider AdtyPaymentProvider) {
	adtyProvidersMu.Lock()
	defer adtyProvidersMu.Unlock()
	for i, existing := range adtyProviders {
		if existing.Name() == provider.Name() {
			adtyProviders[i] = provider
			return
		}
	}
	adtyProviders = append(adtyProviders, provider)
}

// AdtyAvailablePaymentMethodIDs 是当前可以下单的支付方式编号，不含密钥和网关地址。
func AdtyAvailablePaymentMethodIDs() []string {
	ids := make([]string, 0)
	for _, provider := range AdtyPaymentProviders() {
		if provider.Name() == AdtyProviderEpay && model.DB == nil {
			continue
		}
		for _, method := range provider.Methods() {
			if method.Available && method.Id != "" {
				ids = append(ids, method.Id)
			}
		}
	}
	return ids
}

// AdtyPaymentProviders 返回登记的支付商。
func AdtyPaymentProviders() []AdtyPaymentProvider {
	adtyProvidersMu.RLock()
	defer adtyProvidersMu.RUnlock()
	return append([]AdtyPaymentProvider(nil), adtyProviders...)
}

// AdtyPaymentProviderByName 按名字取支付商。
func AdtyPaymentProviderByName(name string) AdtyPaymentProvider {
	for _, provider := range AdtyPaymentProviders() {
		if provider.Name() == name {
			return provider
		}
	}
	return nil
}

// AdtyPaymentProviderForMethod 返回支持这种方式的支付商。
func AdtyPaymentProviderForMethod(method string) AdtyPaymentProvider {
	for _, provider := range AdtyPaymentProviders() {
		for _, info := range provider.Methods() {
			if info.Id == method {
				return provider
			}
		}
	}
	return nil
}

// AdtyReferralSnapshotHook 在下单时调用（P6 注册），返回推广员和当时的比例（百分数）。
var AdtyReferralSnapshotHook func(userId int) (affiliateId int, rate decimal.Decimal)

// AdtyCanceledHook 在用户取消订单之后调用（P5 按通道计连续未付）。
var AdtyCanceledHook func(ext *model.AdtyTopupOrder)
