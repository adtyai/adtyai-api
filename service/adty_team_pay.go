package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
)

// StartAdtyTeamTopupPayment opens an epay order for a team.
// The payable amount uses the configured top-up price. The order has no gift.
func StartAdtyTeamTopupPayment(leaderID int, teamID int64, amountUSD decimal.Decimal, method, key, clientIP string) (*model.AdtyTopupOrder, error) {
	method, key = strings.TrimSpace(method), strings.TrimSpace(key)
	if leaderID <= 0 || teamID <= 0 || !amountUSD.IsPositive() || !amountUSD.Equal(decimal.NewFromInt(amountUSD.IntPart())) || key == "" {
		return nil, ErrAdtyTeamInvalid
	}
	if IsAdtyUSDTMethod(method) {
		return startAdtyTeamBEpusdtTopup(leaderID, teamID, amountUSD, method, key)
	}
	if !IsAdtyEpayMethod(method) {
		return nil, ErrAdtyTeamInvalid
	}
	tradeNo := "TTEP" + common.Sha1([]byte(key))[:20]
	var existing model.AdtyTopupOrder
	lookup := model.DB.Where("trade_no = ?", tradeNo).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected == 1 {
		if existing.TeamId != teamID || existing.UserId != leaderID || !existing.FaceUSD.Equal(amountUSD) || existing.Method != method {
			return nil, ErrAdtyTeamInvalid
		}
		return &existing, nil
	}
	var team model.AdtyTeam
	if err := model.DB.First(&team, teamID).Error; err != nil {
		return nil, err
	}
	if team.Status != model.AdtyTeamStatusActive || team.LeaderUserId != leaderID {
		return nil, ErrAdtyTeamForbidden
	}
	var leader model.User
	if err := model.DB.First(&leader, leaderID).Error; err != nil {
		return nil, err
	}
	quote, err := AdtyQuoteForSite(AdtyBillingSiteForUser(leaderID), leader.Group, amountUSD.IntPart(), method, AdtyPromoInput{})
	if err != nil {
		return nil, err
	}
	quote.BonusRate, quote.BonusUSD, quote.PromoBonusUSD = decimal.Zero, decimal.Zero, decimal.Zero
	quote.PromoCodeId, quote.PromoFunder = 0, ""
	faceQuota, err := common.WalletQuotaFromDecimalStrict(quote.FaceUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil || faceQuota <= 0 {
		return nil, ErrAdtyTeamInvalid
	}
	quote.CreditQuota = int64(faceQuota)
	if _, err := AuthorizeAdtyPaymentScene(leaderID, "wallet_topup", model.PaymentProviderAdtyEpay, method, quote); err != nil {
		return nil, err
	}
	channel, channelMethod, err := PickAdtyEpayChannel(method, quote.PayAmount, "pc", nil, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	payMoney, _ := quote.PayAmount.Float64()
	topUp := &model.TopUp{
		UserId: leaderID, Amount: quote.FaceUSD.IntPart(), Money: payMoney, TradeNo: tradeNo,
		PaymentMethod: method, PaymentProvider: model.PaymentProviderAdtyEpay,
		CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending,
	}
	ext := &model.AdtyTopupOrder{
		TradeNo: tradeNo, UserId: leaderID, TeamId: teamID, Provider: model.PaymentProviderAdtyEpay,
		Method: method, FaceUSD: quote.FaceUSD, BonusRate: decimal.Zero, BonusUSD: decimal.Zero,
		PromoBonusUSD: decimal.Zero, CreditQuota: quote.CreditQuota, PayAmount: quote.PayAmount,
		PayCurrency: quote.PayCurrency, PriceSnapshot: quote.PriceCNYPerUSD, TopupGroupRatio: quote.TopupGroupRatio,
		ChannelId: channel.Id,
	}
	if site := AdtyBillingSiteForUser(leaderID); site != nil {
		ext.SiteId = site.Id
	}
	if err := CreateAdtyPaymentOrder(topUp, ext, "wallet_topup"); err != nil {
		return nil, err
	}
	result, err := CreateAdtyEpayOrder(context.Background(), channel, channelMethod, AdtyEpayCreateRequest{
		TradeNo: tradeNo, Name: "Team top-up " + tradeNo, ClientIP: clientIP, Device: "pc",
		MoneyCNY: quote.PayAmount, NotifyURL: strings.TrimRight(GetCallbackAddress(), "/") + "/api/user/epay/notify/" + strconv.Itoa(channel.Id),
		ReturnURL: AdtyPayResultURL(tradeNo),
	})
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderAdtyEpay, common.TopUpStatusFailed)
		return nil, err
	}
	if err := model.UpdateAdtyTopupOrderFields(tradeNo, map[string]any{
		"qr_code": result.QRCode, "pay_url": result.PayURL, "url_scheme": result.URLScheme, "gateway_trade_id": result.GatewayTradeNo,
	}); err != nil {
		return nil, err
	}
	return model.GetAdtyTopupOrder(tradeNo)
}

// StartAdtyTeamStripeTopup opens a team order and asks the existing Stripe checkout for a pay link.
// The pay amount comes from the live Stripe quote. The order stores no gift.
func StartAdtyTeamStripeTopup(leaderID int, teamID int64, amountUSD, payAmount decimal.Decimal, key string, checkout func(tradeNo string, money decimal.Decimal) (string, string, error)) (*model.AdtyTopupOrder, error) {
	key = strings.TrimSpace(key)
	if leaderID <= 0 || teamID <= 0 || !amountUSD.IsPositive() || !amountUSD.Equal(decimal.NewFromInt(amountUSD.IntPart())) || !payAmount.IsPositive() || key == "" || checkout == nil {
		return nil, ErrAdtyTeamInvalid
	}
	tradeNo := "TTS" + common.Sha1([]byte(key))[:20]
	var existing model.AdtyTopupOrder
	lookup := model.DB.Where("trade_no = ?", tradeNo).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected == 1 {
		if existing.TeamId != teamID || existing.UserId != leaderID || !existing.FaceUSD.Equal(amountUSD) || existing.Provider != model.PaymentProviderStripe {
			return nil, ErrAdtyTeamInvalid
		}
		return &existing, nil
	}
	return createAdtyTeamHostedTopup(leaderID, teamID, amountUSD, payAmount, "USD", model.PaymentProviderStripe, model.PaymentMethodStripe, tradeNo, func(tradeNo string) (string, string, error) {
		return checkout(tradeNo, payAmount)
	})
}

func createAdtyTeamHostedTopup(leaderID int, teamID int64, amountUSD, payAmount decimal.Decimal, currency, provider, method, tradeNo string, checkout func(tradeNo string) (string, string, error)) (*model.AdtyTopupOrder, error) {
	var team model.AdtyTeam
	if err := model.DB.First(&team, teamID).Error; err != nil {
		return nil, err
	}
	if team.Status != model.AdtyTeamStatusActive || team.LeaderUserId != leaderID {
		return nil, ErrAdtyTeamForbidden
	}
	var leader model.User
	if err := model.DB.First(&leader, leaderID).Error; err != nil {
		return nil, err
	}
	faceQuota, err := common.WalletQuotaFromDecimalStrict(amountUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil || faceQuota <= 0 {
		return nil, ErrAdtyTeamInvalid
	}
	quote, err := AdtyHostedQuoteForSite(AdtyBillingSiteForUser(leaderID), leader.Group, decimal.NewFromInt(int64(faceQuota)), payAmount, currency, AdtyPromoInput{})
	if err != nil {
		return nil, err
	}
	quote.BonusRate, quote.BonusUSD, quote.PromoBonusUSD = decimal.Zero, decimal.Zero, decimal.Zero
	quote.PromoCodeId, quote.PromoFunder = 0, ""
	quote.CreditQuota = int64(faceQuota)
	quote.FaceUSD = amountUSD
	if _, err := AuthorizeAdtyPaymentScene(leaderID, "wallet_topup", provider, method, quote); err != nil {
		return nil, err
	}
	payMoney, _ := payAmount.Float64()
	topUp := &model.TopUp{
		UserId: leaderID, Amount: amountUSD.IntPart(), Money: payMoney, TradeNo: tradeNo,
		PaymentMethod: method, PaymentProvider: provider,
		CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending,
	}
	ext := &model.AdtyTopupOrder{
		TradeNo: tradeNo, UserId: leaderID, TeamId: teamID, Provider: provider, Method: method,
		FaceUSD: amountUSD, BonusRate: decimal.Zero, BonusUSD: decimal.Zero, PromoBonusUSD: decimal.Zero,
		CreditQuota: quote.CreditQuota, PayAmount: payAmount, PayCurrency: currency, TopupGroupRatio: quote.TopupGroupRatio,
	}
	if site := AdtyBillingSiteForUser(leaderID); site != nil {
		ext.SiteId = site.Id
	}
	if err := CreateAdtyPaymentOrder(topUp, ext, "wallet_topup"); err != nil {
		return nil, err
	}
	payURL, gatewayID, err := checkout(tradeNo)
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, provider, common.TopUpStatusFailed)
		return nil, err
	}
	if err := model.UpdateAdtyTopupOrderFields(tradeNo, map[string]any{"pay_url": payURL, "gateway_trade_id": gatewayID}); err != nil {
		return nil, err
	}
	return model.GetAdtyTopupOrder(tradeNo)
}

func startAdtyTeamBEpusdtTopup(leaderID int, teamID int64, amountUSD decimal.Decimal, method, key string) (*model.AdtyTopupOrder, error) {
	if !setting.BEpusdtConfigured() || !setting.BEpusdtChainEnabled(method) {
		return nil, common.NewMessage("This payment method is not available")
	}
	tradeNo := "TTU" + common.Sha1([]byte(key))[:20]
	var existing model.AdtyTopupOrder
	lookup := model.DB.Where("trade_no = ?", tradeNo).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected == 1 {
		if existing.TeamId != teamID || existing.UserId != leaderID || !existing.FaceUSD.Equal(amountUSD) || existing.Method != method {
			return nil, ErrAdtyTeamInvalid
		}
		return &existing, nil
	}
	var team model.AdtyTeam
	if err := model.DB.First(&team, teamID).Error; err != nil {
		return nil, err
	}
	if team.Status != model.AdtyTeamStatusActive || team.LeaderUserId != leaderID {
		return nil, ErrAdtyTeamForbidden
	}
	var leader model.User
	if err := model.DB.First(&leader, leaderID).Error; err != nil {
		return nil, err
	}
	quote, err := AdtyQuoteForSite(AdtyBillingSiteForUser(leaderID), leader.Group, amountUSD.IntPart(), method, AdtyPromoInput{})
	if err != nil {
		return nil, err
	}
	quote.BonusRate, quote.BonusUSD, quote.PromoBonusUSD = decimal.Zero, decimal.Zero, decimal.Zero
	quote.PromoCodeId, quote.PromoFunder = 0, ""
	faceQuota, err := common.WalletQuotaFromDecimalStrict(quote.FaceUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil || faceQuota <= 0 {
		return nil, ErrAdtyTeamInvalid
	}
	quote.CreditQuota = int64(faceQuota)
	if _, err := AuthorizeAdtyPaymentScene(leaderID, "wallet_topup", model.PaymentProviderAdtyBEpusdt, method, quote); err != nil {
		return nil, err
	}
	payMoney, _ := quote.PayAmount.Float64()
	topUp := &model.TopUp{
		UserId: leaderID, Amount: quote.FaceUSD.IntPart(), Money: payMoney, TradeNo: tradeNo,
		PaymentMethod: method, PaymentProvider: model.PaymentProviderAdtyBEpusdt,
		CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending,
	}
	ext := &model.AdtyTopupOrder{
		TradeNo: tradeNo, UserId: leaderID, TeamId: teamID, Provider: model.PaymentProviderAdtyBEpusdt,
		Method: method, FaceUSD: quote.FaceUSD, BonusRate: decimal.Zero, BonusUSD: decimal.Zero,
		PromoBonusUSD: decimal.Zero, CreditQuota: quote.CreditQuota, PayAmount: quote.PayAmount,
		PayCurrency: quote.PayCurrency, PriceSnapshot: quote.PriceCNYPerUSD, TopupGroupRatio: quote.TopupGroupRatio,
	}
	if site := AdtyBillingSiteForUser(leaderID); site != nil {
		ext.SiteId = site.Id
	}
	if err := CreateAdtyPaymentOrder(topUp, ext, "wallet_topup"); err != nil {
		return nil, err
	}
	minutes := 15
	order, err := CreateBEpusdtOrder(BEpusdtCreateRequest{
		OrderID: tradeNo, AmountUSD: quote.PayAmount, TradeType: method,
		NotifyURL:   strings.TrimRight(GetCallbackAddress(), "/") + "/api/user/bepusdt/notify",
		RedirectURL: AdtyPayResultURL(tradeNo), Name: "Team top-up " + tradeNo, Timeout: minutes * 60,
	})
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderAdtyBEpusdt, common.TopUpStatusFailed)
		return nil, err
	}
	if err := model.UpdateAdtyTopupOrderFields(tradeNo, map[string]any{
		"pay_address": order.Address, "pay_amount": order.ActualAmount, "pay_url": order.PaymentURL,
		"expire_at": order.ExpireAt, "gateway_trade_id": order.TradeId,
	}); err != nil {
		return nil, err
	}
	return model.GetAdtyTopupOrder(tradeNo)
}
