package controller

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// Hosted checkouts use their existing provider contracts and a shared durable
// order snapshot for bonuses, referrals, site funding, audit, and refunds.
func createAdtyHostedOrder(c *gin.Context, topUp *model.TopUp, baseQuota decimal.Decimal, currency, product, environment, promo string) error {
	payAmount := decimal.NewFromFloat(topUp.Money)
	switch topUp.PaymentProvider {
	case model.PaymentProviderWaffo:
		payAmount, _ = decimal.NewFromString(formatWaffoAmount(topUp.Money, currency))
	case model.PaymentProviderWaffoPancake:
		payAmount, _ = decimal.NewFromString(formatWaffoPancakeAmount(topUp.Money))
	}
	topUp.Money = payAmount.InexactFloat64()
	group, err := model.GetUserGroup(topUp.UserId, true)
	if err != nil {
		return err
	}
	site := service.AdtyBillingSiteForUser(topUp.UserId)
	quote, err := service.AdtyHostedQuoteForSite(site, group, baseQuota, payAmount, currency, adtyPromoInput(c, promo))
	if err != nil {
		return err
	}
	if err := model.ValidateTopUpQuotaCapacity(topUp.UserId, int(quote.CreditQuota)); err != nil {
		return err
	}
	ext := &model.AdtyTopupOrder{
		TradeNo: topUp.TradeNo, UserId: topUp.UserId, Provider: topUp.PaymentProvider, Method: topUp.PaymentMethod,
		FaceUSD: quote.FaceUSD, BonusRate: quote.BonusRate, BonusUSD: quote.BonusUSD, CreditQuota: quote.CreditQuota,
		PayAmount: quote.PayAmount, PayCurrency: quote.PayCurrency, TopupGroupRatio: quote.TopupGroupRatio,
		PromoCodeId: quote.PromoCodeId, PromoBonusUSD: quote.PromoBonusUSD, PromoFunder: quote.PromoFunder,
		ProviderProductId: product, ProviderEnvironment: environment, ExpireAt: time.Now().Add(45 * time.Minute).Unix(),
	}
	if site != nil {
		ext.SiteId = site.Id
	}
	if ext.Provider == model.PaymentProviderWaffo {
		ext.ProviderMerchantId = setting.WaffoMerchantId
	} else if ext.Provider == model.PaymentProviderWaffoPancake {
		ext.ProviderMerchantId = setting.WaffoPancakeStoreID
	}
	return model.CreateAdtyTopUp(topUp, ext)
}

func hostedPaymentEnvironment(test bool) string {
	if test {
		return "test"
	}
	return "prod"
}

func stripePaymentEnvironment(key string) string {
	return hostedPaymentEnvironment(strings.HasPrefix(key, "sk_test_") || strings.HasPrefix(key, "rk_test_"))
}

// External reversals require the existing finance reconciliation flow: the
// provider has already moved money, while local refund previews still apply
// the customer's remaining principal and gift rules. They are never payment
// proofs and a provider resend retains one review item per resource/state.
func recordHostedReversal(provider, eventType, resourceID, tradeNo, voucher, payload string) error {
	if strings.TrimSpace(resourceID) == "" {
		resourceID = common.Sha1([]byte(tradeNo + "|" + voucher + "|" + payload))
	}
	if tradeNo == "" && voucher != "" {
		var matches []model.AdtyTopupOrder
		if err := model.DB.Where("provider = ? AND (paid_tx_hash = ? OR gateway_trade_id = ?)", provider, voucher, voucher).Limit(2).Find(&matches).Error; err != nil {
			return err
		}
		if len(matches) == 1 {
			tradeNo = matches[0].TradeNo
		}
	}
	return model.RecordPaymentOrphanEvent(&model.PaymentOrphanEvent{
		Provider: provider, EventID: "reversal:" + common.Sha1([]byte(provider+"|"+eventType+"|"+resourceID)), EventType: eventType,
		ReferenceID: tradeNo, SessionID: voucher, Payload: payload,
		Reason: "External refund or dispute requires reconciliation",
	})
}
