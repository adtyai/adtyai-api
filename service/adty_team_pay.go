package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

// StartAdtyTeamTopupPayment opens an epay order for a team.
// The payable amount uses the configured top-up price. The order has no gift.
func StartAdtyTeamTopupPayment(leaderID int, teamID int64, amountUSD decimal.Decimal, method, key, clientIP string) (*model.AdtyTopupOrder, error) {
	method, key = strings.TrimSpace(method), strings.TrimSpace(key)
	if leaderID <= 0 || teamID <= 0 || !amountUSD.IsPositive() || !amountUSD.Equal(decimal.NewFromInt(amountUSD.IntPart())) || key == "" || !IsAdtyEpayMethod(method) {
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
