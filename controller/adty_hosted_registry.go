package controller

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type adtyHostedProvider string

func init() {
	for _, name := range []string{model.PaymentProviderStripe, model.PaymentProviderCreem, model.PaymentProviderWaffo, model.PaymentProviderWaffoPancake} {
		service.RegisterAdtyPaymentProvider(adtyHostedProvider(name))
	}
}

func PostAdtyHostedPayment(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var req adtyQuoteRequest
	if common.DecodeJson(c.Request.Body, &req) != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}
	quote, err := adtyHostedQuote(c, req)
	provider := service.AdtyPaymentProviderForMethod(req.Method)
	if err != nil {
		paymentError(c, err)
		return
	}
	if provider == nil || !provider.AvailableFor(req.Method, quote) {
		paymentError(c, common.NewMessage("Unsupported payment channel"))
		return
	}
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens && !strings.HasPrefix(req.Method, "creem:") {
		amount, err := common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(req.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
		if err != nil {
			paymentError(c, err)
			return
		}
		req.Amount = int64(amount)
	}
	switch req.Method {
	case model.PaymentMethodStripe:
		stripeAdaptor.RequestPay(c, &StripePayRequest{Amount: req.Amount, PaymentMethod: req.Method, PromoCode: req.PromoCode})
	case model.PaymentMethodWaffo, model.PaymentMethodWaffoPancake:
		payload, err := common.Marshal(map[string]any{"amount": req.Amount, "promo_code": req.PromoCode})
		if err != nil {
			paymentError(c, err)
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(payload))
		if req.Method == model.PaymentMethodWaffo {
			RequestWaffoPay(c)
		} else {
			RequestWaffoPancakePay(c)
		}
	default:
		product, _ := strings.CutPrefix(req.Method, "creem:")
		creemAdaptor.RequestPay(c, &CreemPayRequest{ProductId: product, PaymentMethod: model.PaymentMethodCreem, PromoCode: req.PromoCode})
	}
}

func (p adtyHostedProvider) Name() string { return string(p) }

func (p adtyHostedProvider) Methods() []service.AdtyMethodInfo {
	switch p.Name() {
	case model.PaymentProviderStripe:
		return []service.AdtyMethodInfo{{Id: p.Name(), Kind: "hosted", Label: "Stripe", Available: isStripeTopUpEnabled()}}
	case model.PaymentProviderWaffo:
		return []service.AdtyMethodInfo{{Id: p.Name(), Kind: "hosted", Label: "Waffo", Available: isWaffoTopUpEnabled()}}
	case model.PaymentProviderWaffoPancake:
		return []service.AdtyMethodInfo{{Id: p.Name(), Kind: "hosted", Label: "Waffo Pancake", Available: isWaffoPancakeTopUpEnabled()}}
	case model.PaymentProviderCreem:
		var products []CreemProduct
		if common.UnmarshalJsonStr(setting.CreemProducts, &products) != nil {
			return nil
		}
		methods := make([]service.AdtyMethodInfo, 0, len(products))
		for _, product := range products {
			if product.ProductId == "" || product.Quota <= 0 || product.Price <= 0 || common.QuotaPerUnit <= 0 {
				continue
			}
			methods = append(methods, service.AdtyMethodInfo{
				Id: "creem:" + product.ProductId, Kind: "hosted", Label: "Creem · " + product.Name,
				FaceUSD: decimal.NewFromInt(product.Quota).Div(decimal.NewFromFloat(common.QuotaPerUnit)).String(), Available: isCreemTopUpEnabled() && isCreemWebhookConfigured(),
			})
		}
		return methods
	}
	return nil
}

func (p adtyHostedProvider) AvailableFor(method string, quote *service.AdtyQuote) bool {
	if quote == nil {
		return false
	}
	for _, info := range p.Methods() {
		if info.Id == method {
			return info.Available
		}
	}
	return false
}

// Checkout cancellation is local; a later verified payment can still settle.
func (p adtyHostedProvider) Cancel(*model.AdtyTopupOrder) error { return nil }

func adtyHostedQuote(c *gin.Context, req adtyQuoteRequest) (*service.AdtyQuote, error) {
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		return nil, err
	}
	baseQuota := decimal.NewFromInt(req.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	requestAmount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens && !strings.HasPrefix(req.Method, "creem:") {
		amount, err := common.WalletQuotaFromDecimalStrict(baseQuota)
		if err != nil {
			return nil, err
		}
		requestAmount = int64(amount)
	}
	var amount decimal.Decimal
	currency := "USD"
	switch req.Method {
	case model.PaymentMethodStripe:
		pay, ok := stripeQuotedPay(requestAmount, group)
		if !ok || requestAmount < getStripeMinTopup() || requestAmount > getStripeMaxTopup() {
			return nil, model.ErrInvalidTopUpQuota
		}
		amount = pay.Money
	case model.PaymentMethodWaffo:
		units, ok := waffoPurchaseUnits(requestAmount)
		if !ok || req.Amount < int64(setting.WaffoMinTopUp) {
			return nil, model.ErrInvalidTopUpQuota
		}
		currency = getWaffoCurrency()
		amount, err = decimal.NewFromString(formatWaffoAmount(getWaffoPayMoney(requestAmount, units, group), currency))
	case model.PaymentMethodWaffoPancake:
		if req.Amount < int64(setting.WaffoPancakeMinTopUp) {
			return nil, model.ErrInvalidTopUpQuota
		}
		amount, err = decimal.NewFromString(formatWaffoPancakeAmount(getWaffoPancakePayMoney(requestAmount, group)))
	default:
		productID, ok := strings.CutPrefix(req.Method, "creem:")
		if !ok {
			return nil, errors.New("unsupported hosted payment method")
		}
		var products []CreemProduct
		if err := common.UnmarshalJsonStr(setting.CreemProducts, &products); err != nil {
			return nil, err
		}
		for _, product := range products {
			if product.ProductId == productID {
				baseQuota, amount, currency = decimal.NewFromInt(product.Quota), decimal.NewFromFloat(product.Price), product.Currency
				break
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return service.AdtyHostedQuoteForSite(service.AdtyBillingSiteForUser(c.GetInt("id")), group, baseQuota, amount, currency, adtyPromoInput(c, req.PromoCode))
}
