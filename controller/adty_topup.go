package controller

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type adtyTierView struct {
	Amount int     `json:"amount"`
	Rate   float64 `json:"rate"`
}

// GetAdtyTopUpInfo 返回钱包页要的数据：档位和加赠、最低金额、支付方式、价格、有效期、待付款订单。
func GetAdtyTopUpInfo(c *gin.Context) {
	setting := adty_setting.GetTopupSetting()
	payment := operation_setting.GetPaymentSetting()
	displayRates := service.AdtyTopupDisplayRates(service.AdtyBillingSiteForUser(c.GetInt("id")))
	tiers := make([]adtyTierView, 0, len(payment.AmountOptions))
	for _, amount := range payment.AmountOptions {
		rate, _ := service.AdtyBonusFromRates(displayRates, int64(amount))
		value, _ := rate.Float64()
		tiers = append(tiers, adtyTierView{Amount: amount, Rate: value})
	}
	bonusRates := map[string]float64{}
	for tier, rate := range displayRates {
		bonusRates[fmt.Sprint(tier)] = rate
	}
	methods := make([]service.AdtyMethodInfo, 0)
	for _, provider := range service.AdtyPaymentProviders() {
		methods = append(methods, provider.Methods()...)
	}
	var pending *model.AdtyTopUpView
	if view, err := model.GetUserPendingAdtyTopUp(c.GetInt("id"), time.Now().Unix()); err == nil {
		pending = view
	}
	common.ApiSuccess(c, gin.H{
		"tiers":                tiers,
		"bonus_rates":          bonusRates,
		"min_topup":            operation_setting.MinTopUp,
		"methods":              methods,
		"price_cny_per_usd":    operation_setting.Price,
		"usdt_order_minutes":   setting.USDTOrderMinutes,
		"epay_order_minutes":   setting.EpayOrderMinutes,
		"pending":              pending,
		"compliance_confirmed": operation_setting.IsPaymentComplianceConfirmed(),
	})
}

type adtyQuoteRequest struct {
	Amount    int64  `json:"amount"`
	Method    string `json:"method"`
	PromoCode string `json:"promo_code"` // 任务 12.4
}

// adtyPromoInput 是当前用户带的优惠码（任务 12.4）。
func adtyPromoInput(c *gin.Context, code string) service.AdtyPromoInput {
	return service.AdtyPromoInput{UserId: c.GetInt("id"), SiteId: service.AdtyUserSiteID(c.GetInt("id")), Code: code}
}

// PostAdtyTopUpQuote 报价，同时返回这个金额下这种方式能不能用。
func PostAdtyTopUpQuote(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var req adtyQuoteRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	quote, err := service.AdtyQuoteForSite(service.AdtyBillingSiteForUser(c.GetInt("id")), c.GetString("group"), req.Amount, req.Method, adtyPromoInput(c, req.PromoCode))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	available := false
	if provider := service.AdtyPaymentProviderForMethod(req.Method); provider != nil {
		available = provider.AvailableFor(req.Method, quote)
	}
	common.ApiSuccess(c, gin.H{"quote": quote, "available": available})
}

// GetAdtyTopUpOrder 返回本人一笔订单的状态和支付信息；别人的订单当作不存在。
func GetAdtyTopUpOrder(c *gin.Context) {
	view, err := model.GetUserAdtyTopUp(c.Param("trade_no"), c.GetInt("id"))
	if err != nil {
		writeAdtyTopUpError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

// CancelAdtyTopUpOrder 先锁住本地订单，再问支付商。只有网关明确说已经有转账时才留下待支付。
func CancelAdtyTopUpOrder(c *gin.Context) {
	userID := c.GetInt("id")
	ext, err := model.CancelAdtyTopUpAfterLock(c.Param("trade_no"), userID, func(locked *model.AdtyTopupOrder) error {
		provider := service.AdtyPaymentProviderByName(locked.Provider)
		if provider == nil {
			return nil
		}
		cancelErr := provider.Cancel(locked)
		if cancelErr == nil || errors.Is(cancelErr, service.ErrAdtyPaymentDetected) {
			return cancelErr
		}
		common.SysLog(common.LogText("cancel adty order at provider failed trade_no=%s: %s", locked.TradeNo, cancelErr.Error()))
		return nil
	})
	if errors.Is(err, service.ErrAdtyPaymentDetected) {
		common.ApiErrorT(c, "A transfer has been detected. Please wait")
		return
	}
	if err != nil {
		writeAdtyTopUpError(c, err)
		return
	}
	if service.AdtyCanceledHook != nil {
		service.AdtyCanceledHook(ext)
	}
	common.ApiSuccess(c, nil)
}

// GetAdtyTopUpPending 返回本人最新一笔还没过期的 pending 订单，没有时 data 为 null。
func GetAdtyTopUpPending(c *gin.Context) {
	view, err := model.GetUserPendingAdtyTopUp(c.GetInt("id"), time.Now().Unix())
	if err != nil {
		writeAdtyTopUpError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

// GetAdtyTopUpSelf 分页列出本人的充值记录。
func GetAdtyTopUpSelf(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListUserAdtyTopUps(c.GetInt("id"), pageInfo)
	if err != nil {
		writeAdtyTopUpError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// adtyOrderDraft 是下单时要的数据，P4 和 P5 共用 createAdtyOrder。
type adtyOrderDraft struct {
	prefix    string // EPU | EPY
	provider  string
	method    string
	quote     *service.AdtyQuote
	expireAt  int64
	channelID int
}

// newAdtyTradeNo 生成 <前缀><uid><6 位随机><unix>，易支付要求不超过 32 位。
func newAdtyTradeNo(prefix string, userID int) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d%06d%d", prefix, userID, n.Int64(), time.Now().Unix()), nil
}

// createAdtyOrder 写 top_ups 和扩展行，带上返佣快照。
func createAdtyOrder(c *gin.Context, draft adtyOrderDraft) (*model.TopUp, *model.AdtyTopupOrder, error) {
	userID := c.GetInt("id")
	tradeNo, err := newAdtyTradeNo(draft.prefix, userID)
	if err != nil {
		return nil, nil, err
	}
	quote := draft.quote
	payMoney, _ := quote.PayAmount.Float64()
	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          quote.FaceUSD.IntPart(),
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   draft.method,
		PaymentProvider: draft.provider,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	ext := &model.AdtyTopupOrder{
		TradeNo:         tradeNo,
		UserId:          userID,
		Provider:        draft.provider,
		Method:          draft.method,
		FaceUSD:         quote.FaceUSD,
		BonusRate:       quote.BonusRate,
		BonusUSD:        quote.BonusUSD,
		CreditQuota:     quote.CreditQuota,
		PayAmount:       quote.PayAmount,
		PayCurrency:     quote.PayCurrency,
		PriceSnapshot:   quote.PriceCNYPerUSD,
		TopupGroupRatio: quote.TopupGroupRatio,
		ExpireAt:        draft.expireAt,
		ChannelId:       draft.channelID,
		PromoCodeId:     quote.PromoCodeId,
		PromoBonusUSD:   quote.PromoBonusUSD,
		PromoFunder:     quote.PromoFunder,
	}
	if service.AdtyReferralSnapshotHook != nil {
		ext.ReferralAffiliateId, ext.ReferralRate = service.AdtyReferralSnapshotHook(userID)
	}
	if site := service.AdtyBillingSiteForUser(userID); site != nil {
		ext.SiteId = site.Id // 分站客户的加赠由站长出（任务 11.5）
	}
	if err := model.CreateAdtyTopUp(topUp, ext); err != nil {
		return nil, nil, err
	}
	return topUp, ext, nil
}

func writeAdtyTopUpError(c *gin.Context, err error) {
	var message *common.Message
	switch {
	case errors.As(err, &message):
		common.ApiError(c, err)
	case errors.Is(err, model.ErrTopUpNotFound):
		common.ApiErrorStatus(c, 404, common.NewMessage("Order not found"))
	case errors.Is(err, model.ErrAdtyTopUpNotCancel):
		common.ApiErrorT(c, "Only orders waiting for payment can be canceled")
	default:
		common.SysError(common.LogText("adty topup: %s", err.Error()))
		common.ApiErrorT(c, "Database error, please contact the administrator")
	}
}

// adtyCallerIP 是写日志用的来源 IP。
func adtyCallerIP(c *gin.Context) string {
	return strings.TrimSpace(c.ClientIP())
}
