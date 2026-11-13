package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Supported providers share frozen order facts and the ADTY settlement ledger.
const (
	PaymentProviderAdtyBEpusdt = "bepusdt"
	PaymentProviderAdtyEpay    = "adty_epay"

	TopUpStatusCanceled = "canceled"
	TopUpStatusRefunded = "refunded"
	TopUpStatusReplaced = "replaced"

	AdtyPaymentSourceCallback = "callback"
	AdtyPaymentSourceQuery    = "query"
	AdtyPaymentSourceManual   = "manual"
)

// IsAdtyPaymentProvider 说明这是不是二开的支付商。
func IsAdtyPaymentProvider(provider string) bool {
	switch provider {
	case PaymentProviderAdtyBEpusdt, PaymentProviderAdtyEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake:
		return true
	}
	return false
}

// AdtyTopupOrder 是订单事实（任务 3.2），按 trade_no 对应上游的 top_ups，上游表不加字段。
type AdtyTopupOrder struct {
	Id                   int64           `json:"id"`
	TradeNo              string          `json:"trade_no" gorm:"uniqueIndex;type:varchar(64);not null"`
	UserId               int             `json:"user_id" gorm:"index"`
	Provider             string          `json:"provider" gorm:"type:varchar(32)"`
	PaymentScene         string          `json:"payment_scene" gorm:"type:varchar(64)"`
	PaymentSceneName     string          `json:"payment_scene_name" gorm:"type:varchar(480)"`
	PaymentSceneVersion  int64           `json:"payment_scene_version"`
	PaymentSceneSnapshot string          `json:"-" gorm:"type:text"`
	Method               string          `json:"method" gorm:"type:varchar(32)"`
	BonusMethod          string          `json:"bonus_method" gorm:"type:varchar(16)"`
	BonusExpiresAt       int64           `json:"bonus_expires_at" gorm:"not null;default:0"`
	GiftReserveSource    string          `json:"gift_reserve_source" gorm:"type:varchar(32)"`
	GiftReserveEarning   decimal.Decimal `json:"gift_reserve_earning" gorm:"type:decimal(20,8);not null;default:0"`
	GiftReserveMargin    decimal.Decimal `json:"gift_reserve_margin" gorm:"type:decimal(20,8);not null;default:0"`
	FaceUSD              decimal.Decimal `json:"face_usd" gorm:"column:face_usd;type:decimal(20,8);not null;default:0"`
	BonusRate            decimal.Decimal `json:"bonus_rate" gorm:"type:decimal(20,8);not null;default:0"`
	BonusUSD             decimal.Decimal `json:"bonus_usd" gorm:"column:bonus_usd;type:decimal(20,8);not null;default:0"`
	CreditQuota          int64           `json:"credit_quota"`
	PayAmount            decimal.Decimal `json:"pay_amount" gorm:"type:decimal(20,8);not null;default:0"`
	PayCurrency          string          `json:"pay_currency" gorm:"type:varchar(16)"`
	PriceSnapshot        decimal.Decimal `json:"price_snapshot" gorm:"type:decimal(20,8);not null;default:0"`
	TopupGroupRatio      decimal.Decimal `json:"topup_group_ratio" gorm:"type:decimal(20,8);not null;default:1"`
	ExpireAt             int64           `json:"expire_at" gorm:"index"`
	GatewayTradeId       string          `json:"gateway_trade_id" gorm:"type:varchar(128)"`
	PayAddress           string          `json:"pay_address" gorm:"type:varchar(128)"`
	PayURL               string          `json:"pay_url" gorm:"column:pay_url;type:text"`
	QRCode               string          `json:"qr_code" gorm:"column:qr_code;type:text"`
	URLScheme            string          `json:"url_scheme" gorm:"column:url_scheme;type:text"`
	ChannelId            int             `json:"channel_id" gorm:"index"`
	UnpaidCounted        bool            `json:"-"`
	PaidAmount           decimal.Decimal `json:"paid_amount" gorm:"type:decimal(20,8);not null;default:0"`
	PaidCurrency         string          `json:"paid_currency" gorm:"type:varchar(16)"`
	PaidTxHash           string          `json:"paid_tx_hash" gorm:"type:varchar(255)"`
	PaidSource           string          `json:"paid_source" gorm:"type:varchar(16)"`
	ReferralAffiliateId  int             `json:"-" gorm:"index"`
	ReferralRate         decimal.Decimal `json:"-" gorm:"type:decimal(10,4);not null;default:0"`
	PromoCodeId          int             `json:"promo_code_id" gorm:"index;not null;default:0"`                                       // 优惠码（任务 12.4），0 表示没用
	PromoBonusUSD        decimal.Decimal `json:"promo_bonus_usd" gorm:"column:promo_bonus_usd;type:decimal(20,8);not null;default:0"` // 下单时快照的优惠加赠，已含在 CreditQuota 里
	PromoFunder          string          `json:"-" gorm:"type:varchar(16);not null;default:''"`                                       // 优惠码出资方快照：platform 或 site_owner
	TeamId               int64           `json:"team_id" gorm:"index;not null;default:0"`                                             // 团队充值；0 表示个人订单
	SiteId               int             `json:"-" gorm:"index"`                                                                      // 下单时客户所属的分站（P11）；加赠由这个站的站长出
	SiteOwnerUserId      int             `json:"-" gorm:"not null;default:0"`
	LastQueryAt          int64           `json:"-"`
	RefundedAt           int64           `json:"refunded_at"`
	RefundReason         string          `json:"-" gorm:"type:text"`
	RefundQuota          int64           `json:"refund_quota" gorm:"not null;default:0"`                     // 退款时退的本金额度 P（赠金不退）
	RefundAmount         decimal.Decimal `json:"refund_amount" gorm:"type:decimal(20,8);not null;default:0"` // 退款时应退的金额，支付币种，线下退
	RefundProof          string          `json:"refund_proof" gorm:"type:varchar(255)"`
	ProviderPayload      string          `json:"-" gorm:"type:text"`
	ProviderProductId    string          `json:"-" gorm:"type:varchar(255)"`
	ProviderMerchantId   string          `json:"-" gorm:"type:varchar(255)"`
	ProviderEnvironment  string          `json:"-" gorm:"type:varchar(16)"`
	CreatedAt            int64           `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt            int64           `json:"updated_at" gorm:"autoUpdateTime"`
	// RefundDeductedQuota、RefundBonusQuota 是这次退款扣回的 R 和订单赠金 G，只在退款钩子里用，不入库。
	RefundDeductedQuota int64 `json:"-" gorm:"-"`
	RefundBonusQuota    int64 `json:"-" gorm:"-"`
}

func (AdtyTopupOrder) TableName() string {
	return "adty_topup_orders"
}

// AdtyPaymentFacts 是支付商告诉我们的到账事实。
type AdtyPaymentFacts struct {
	Provider, Method string
	PaidAmount       decimal.Decimal // USDT 数量或人民币金额
	PurchaseAmount   decimal.Decimal // Signed provider item subtotal before tax/discount; zero means exact payment only.
	FiatAmount       decimal.Decimal // BEpusdt 回调里的法币金额；易支付不用
	PaidCurrency     string
	PayAddress       string // BEpusdt 回调里的 token
	GatewayTradeId   string
	TxHash, Payload  string
	Source           string // callback | query | manual
	ProductId        string
	MerchantId       string
	Environment      string
	OperatorId       int // Server-side actor; zero denotes a provider or maintenance task.
}

var (
	ErrPaymentAmountMismatch = errors.New("payment amount mismatch")
	ErrAdtyTopUpNotCancel    = errors.New("adty topup cannot be canceled")
	ErrAdtyDuplicatePayment  = errors.New("duplicate payment on a completed order")
)

// AdtyAfterTopUpCreditedTx 在入账事务里调用（P6 注册返佣计佣任务）。只能做不会失败的写，
// PostgreSQL 事务里任何一条语句出错，整个入账事务都会作废（审阅重点 10）。
var AdtyAfterTopUpCreditedTx func(tx *gorm.DB, topUp *TopUp, ext *AdtyTopupOrder) error

// AdtyCreditTeamTopUpTx credits a team order inside the payment transaction.
// A nil hook must fail closed so a team order cannot fall through to a personal wallet.
var AdtyCreditTeamTopUpTx func(tx *gorm.DB, topUp *TopUp, ext *AdtyTopupOrder, principal int64) error

// AdtyBeforeTopUpCreateTx 在订单写入事务内生成需要冻结的业务快照。它在创建订单前执行，
// 使快照与站点身份变更共享同一行锁，避免在事务外读取后被并发开站插入。
var AdtyBeforeTopUpCreateTx func(tx *gorm.DB, topUp *TopUp, ext *AdtyTopupOrder) error

// AdtyAfterTopUpCompleted 在入账提交之后调用（P5 用它清零通道的连续未付）。未注册就跳过。
var AdtyAfterTopUpCompleted func(topUp *TopUp, ext *AdtyTopupOrder)

// CreateAdtyTopUp 在一个事务里插 top_ups 和扩展行。
func CreateAdtyTopUp(topUp *TopUp, ext *AdtyTopupOrder) error {
	return CreateAdtyTopUpWithPolicy(topUp, ext, nil, nil)
}

// Prepare runs before channel locks; validate runs after the existing referral
// identity locks. Callers must preserve configuration → channel → user order.
func CreateAdtyTopUpWithPolicy(topUp *TopUp, ext *AdtyTopupOrder, prepare, validate func(*gorm.DB) error) error {
	if topUp == nil || ext == nil || topUp.TradeNo == "" || topUp.TradeNo != ext.TradeNo {
		return errors.New("invalid adty topup order")
	}
	if !IsAdtyPaymentProvider(topUp.PaymentProvider) || topUp.PaymentProvider != ext.Provider {
		return ErrPaymentMethodMismatch
	}
	var options []*sql.TxOptions
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		// Identity checks after waiting for user locks must see committed site
		// openings, even when the daily-cap query ran earlier in this transaction.
		options = append(options, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if prepare != nil {
			if err := prepare(tx); err != nil {
				return err
			}
		}
		// Acquire the payment-method lock before any consistent read: on MySQL
		// REPEATABLE READ an earlier snapshot could miss a previous reservation.
		if ext.Provider == PaymentProviderAdtyEpay && ext.ChannelId > 0 {
			if err := lockAdtyEpayDailyRoom(tx, ext.ChannelId, ext.Method, ext.PayAmount, time.Now().Unix()); err != nil {
				return err
			}
		}
		if AdtyBeforeTopUpCreateTx != nil {
			if err := AdtyBeforeTopUpCreateTx(tx, topUp, ext); err != nil {
				return err
			}
		}
		if validate != nil {
			if err := validate(tx); err != nil {
				return err
			}
		}
		owner, err := CurrentAdtySiteFinancialOwnerTx(tx, ext.SiteId)
		if err != nil {
			return err
		}
		ext.SiteOwnerUserId = owner
		if err := tx.Create(topUp).Error; err != nil {
			return err
		}
		return tx.Create(ext).Error
	}, options...)
}

// GetAdtyTopupOrder 按订单号取扩展行，没有时返回 nil。
func GetAdtyTopupOrder(tradeNo string) (*AdtyTopupOrder, error) {
	var rows []AdtyTopupOrder
	if err := DB.Where("trade_no = ?", tradeNo).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// UpdateAdtyTopupOrderFields 改扩展行的几个字段（下单后写回网关信息等）。
func UpdateAdtyTopupOrderFields(tradeNo string, fields map[string]any) error {
	return DB.Model(&AdtyTopupOrder{}).Where("trade_no = ?", tradeNo).Updates(fields).Error
}

// lockAdtyTopUpTx 按固定顺序先锁 top_ups 行、再锁扩展行（Complete、Cancel、Expire、Refund 都这样）。
func lockAdtyTopUpTx(tx *gorm.DB, tradeNo string) (*TopUp, *AdtyTopupOrder, error) {
	var topUps []TopUp
	if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).Limit(1).Find(&topUps).Error; err != nil {
		return nil, nil, err
	}
	if len(topUps) == 0 {
		return nil, nil, ErrTopUpNotFound
	}
	var exts []AdtyTopupOrder
	if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).Limit(1).Find(&exts).Error; err != nil {
		return nil, nil, err
	}
	if len(exts) == 0 {
		return &topUps[0], nil, ErrTopUpNotFound
	}
	return &topUps[0], &exts[0], nil
}

// CompleteAdtyTopUpTx 在调用方的事务里给我们的订单入账（照上游 RechargeEpay 的写法）。
// pending、expired、canceled、failed 都允许入账：钱已经收到了（执行层决定 4）。
// Source=manual 时跳过金额核对，按下单时的面额加赠入账（任务 3.5）。
func CompleteAdtyTopUpTx(tx *gorm.DB, tradeNo string, facts AdtyPaymentFacts) (alreadyDone bool, credited int, topUp *TopUp, ext *AdtyTopupOrder, err error) {
	topUp, ext, err = lockAdtyTopUpTx(tx, tradeNo)
	if err != nil {
		return false, 0, nil, nil, err
	}
	if !IsAdtyPaymentProvider(topUp.PaymentProvider) || topUp.PaymentProvider != facts.Provider || ext.Provider != facts.Provider {
		return false, 0, nil, nil, ErrPaymentMethodMismatch
	}
	switch topUp.Status {
	case common.TopUpStatusSuccess:
		if adtyPaymentVoucherDiffers(ext, facts) {
			return false, 0, topUp, ext, ErrAdtyDuplicatePayment
		}
		err = appendAdtyPaymentEventTx(tx, ext, facts.Source, "replayed", facts.OperatorId)
		return true, 0, topUp, ext, err
	case common.TopUpStatusPending, common.TopUpStatusExpired, common.TopUpStatusFailed, TopUpStatusCanceled:
	default:
		return false, 0, nil, nil, ErrTopUpStatusInvalid
	}
	if facts.Source != AdtyPaymentSourceManual {
		var reversals int64
		if err := tx.Model(&PaymentOrphanEvent{}).Where("provider = ? AND reference_id = ? AND status <> ? AND (event_id LIKE ? OR status IN ?)", ext.Provider, tradeNo, PaymentOrphanStatusDismissed, "reversal:%", []string{PaymentOrphanStatusNeverCredited, PaymentOrphanStatusRefunded}).Count(&reversals).Error; err != nil {
			return false, 0, nil, nil, err
		}
		if reversals > 0 {
			return false, 0, nil, nil, ErrPaymentOrphanNotCredit
		}
		if err := checkAdtyPaymentFacts(ext, facts); err != nil {
			return false, 0, nil, nil, err
		}
	}
	if ext.CreditQuota <= 0 || ext.CreditQuota > int64(common.MaxWalletQuota) {
		return false, 0, nil, nil, ErrInvalidTopUpQuota
	}
	now := common.GetTimestamp()
	if err := LockAdtyPromoEligibilityTx(tx, ext.UserId); err != nil {
		return false, 0, nil, nil, err
	}
	if ext.PromoCodeId > 0 {
		if err := adtyRedeemPromoOrDropTx(tx, ext, now); err != nil {
			return false, 0, nil, nil, err
		}
	}
	credited = int(ext.CreditQuota)
	if ext.TeamId > 0 {
		if ext.BonusUSD.IsPositive() || ext.PromoBonusUSD.IsPositive() || !ext.BonusRate.IsZero() || int64(credited) != ext.FaceQuota() {
			return false, 0, nil, nil, errors.New("team top-up cannot include a gift")
		}
		if AdtyCreditTeamTopUpTx == nil {
			return false, 0, nil, nil, errors.New("team top-up credit is not configured")
		}
	}
	if err := tx.Model(&TopUp{}).Where("id = ?", topUp.Id).Updates(map[string]any{
		"status":        common.TopUpStatusSuccess,
		"complete_time": now,
	}).Error; err != nil {
		return false, 0, nil, nil, err
	}
	topUp.Status = common.TopUpStatusSuccess
	topUp.CompleteTime = now
	extUpdates := map[string]any{
		"paid_amount":   facts.PaidAmount,
		"paid_currency": facts.PaidCurrency,
		"paid_tx_hash":  strings.TrimSpace(facts.TxHash),
		"paid_source":   facts.Source,
	}
	if facts.Payload != "" {
		extUpdates["provider_payload"] = facts.Payload
	}
	if err := tx.Model(&AdtyTopupOrder{}).Where("id = ?", ext.Id).Updates(extUpdates).Error; err != nil {
		return false, 0, nil, nil, err
	}
	ext.PaidAmount = facts.PaidAmount
	ext.PaidCurrency = facts.PaidCurrency
	ext.PaidTxHash = strings.TrimSpace(facts.TxHash)
	ext.PaidSource = facts.Source
	// 不在事务里调 ValidateTopUpQuotaCapacity：它读全局 DB，看不到事务里的改动。
	if ext.TeamId > 0 {
		if err := AdtyCreditTeamTopUpTx(tx, topUp, ext, ext.FaceQuota()); err != nil {
			return false, 0, nil, nil, err
		}
		err = appendAdtyPaymentEventTx(tx, ext, facts.Source, "credited", facts.OperatorId)
		return false, credited, topUp, ext, err
	}
	var user User
	if err := lockForUpdate(tx).First(&user, topUp.UserId).Error; err != nil {
		return false, 0, nil, nil, err
	}
	before, err := userWalletBalance(user.Quota, user.QuotaPending)
	if err != nil {
		return false, 0, nil, nil, err
	}
	if err := creditTopUpQuota(tx, topUp.UserId, credited, nil); err != nil {
		return false, 0, nil, nil, err
	}
	faceQuota := ext.FaceQuota()
	bonusQuota := int64(credited) - faceQuota
	if bonusQuota < 0 {
		bonusQuota = 0
		faceQuota = int64(credited)
	}
	if err := CreateAdtyFundBatchesTx(tx, topUp.UserId, tradeNo, faceQuota, bonusQuota); err != nil {
		return false, 0, nil, nil, err
	}
	key := "topup:" + common.Sha1([]byte(tradeNo))
	if err := InsertAdtyLedgerTx(tx, &AdtyBalanceLedger{
		UserId: topUp.UserId, DeltaQuota: int64(credited), BalanceBefore: int64(before), BalanceAfter: int64(before) + int64(credited),
		Type: AdtyLedgerTypeTopUp, RefType: "topup", RefId: tradeNo, IdempotencyKey: &key,
	}); err != nil {
		return false, 0, nil, nil, err
	}
	if AdtyAfterTopUpCreditedTx != nil {
		if err := AdtyAfterTopUpCreditedTx(tx, topUp, ext); err != nil {
			return false, 0, nil, nil, err
		}
	}
	err = appendAdtyPaymentEventTx(tx, ext, facts.Source, "credited", facts.OperatorId)
	return false, credited, topUp, ext, err
}

// adtyPaymentVoucherDiffers is true when a completed order is notified again
// with a different payment voucher. An empty incoming hash is a replay of the
// same notice, not a second payment.
func adtyPaymentVoucherDiffers(ext *AdtyTopupOrder, facts AdtyPaymentFacts) bool {
	incoming := strings.TrimSpace(facts.TxHash)
	stored := strings.TrimSpace(ext.PaidTxHash)
	return incoming != "" && stored != "" && !strings.EqualFold(incoming, stored)
}

func recordDuplicatePaymentReview(tradeNo string, facts AdtyPaymentFacts, cause error) {
	if DB == nil {
		return
	}
	if err := RecordPaymentOrphanEvent(&PaymentOrphanEvent{
		Provider:    facts.Provider,
		EventType:   "duplicate_payment",
		ReferenceID: tradeNo,
		SessionID:   strings.TrimSpace(facts.TxHash),
		Reason:      "a completed order received a different payment voucher",
		Error:       cause.Error(),
		Payload:     facts.Payload,
	}); err != nil {
		common.SysError(common.LogText("record duplicate payment review failed trade_no=%s: %s", tradeNo, err.Error()))
	}
}

// checkAdtyPaymentFacts 核对回调或补查的金额：USDT 要求法币金额等于面额 × 分组充值倍率、
// 实付不少于应付、收款地址和 trade_id 与下单时一致；易支付要求人民币金额分毫不差。
func checkAdtyPaymentFacts(ext *AdtyTopupOrder, facts AdtyPaymentFacts) error {
	switch ext.Provider {
	case PaymentProviderAdtyBEpusdt:
		ratio := ext.TopupGroupRatio
		if !ratio.IsPositive() {
			ratio = decimal.NewFromInt(1)
		}
		expectedFiat := ext.FaceUSD.Mul(ratio).Round(2)
		if !facts.FiatAmount.Round(2).Equal(expectedFiat) {
			return fmt.Errorf("%w: fiat %s != %s", ErrPaymentAmountMismatch, facts.FiatAmount, expectedFiat)
		}
		if facts.PaidAmount.LessThan(ext.PayAmount) {
			return fmt.Errorf("%w: paid %s < %s", ErrPaymentAmountMismatch, facts.PaidAmount, ext.PayAmount)
		}
		if ext.PayAddress != "" && !strings.EqualFold(strings.TrimSpace(facts.PayAddress), ext.PayAddress) {
			return fmt.Errorf("%w: address", ErrPaymentAmountMismatch)
		}
		if ext.GatewayTradeId != "" && strings.TrimSpace(facts.GatewayTradeId) != ext.GatewayTradeId {
			return fmt.Errorf("%w: trade id", ErrPaymentAmountMismatch)
		}
	case PaymentProviderAdtyEpay:
		if !facts.PaidAmount.Round(2).Equal(ext.PayAmount.Round(2)) {
			return fmt.Errorf("%w: paid %s != %s", ErrPaymentAmountMismatch, facts.PaidAmount, ext.PayAmount)
		}
	case PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake:
		amountMatches := facts.PaidAmount.Equal(ext.PayAmount)
		if (ext.Provider == PaymentProviderStripe || ext.Provider == PaymentProviderCreem || ext.Provider == PaymentProviderWaffoPancake) && facts.PurchaseAmount.IsPositive() {
			amountMatches = facts.PurchaseAmount.Equal(ext.PayAmount) && facts.PaidAmount.IsPositive()
		}
		if !amountMatches || !facts.PaidAmount.IsPositive() || !strings.EqualFold(strings.TrimSpace(facts.PaidCurrency), ext.PayCurrency) {
			return fmt.Errorf("%w: amount or currency", ErrPaymentAmountMismatch)
		}
		if facts.Method != ext.Method || (ext.GatewayTradeId != "" && facts.GatewayTradeId != ext.GatewayTradeId) ||
			(ext.ProviderProductId != "" && facts.ProductId != ext.ProviderProductId) ||
			(ext.ProviderMerchantId != "" && facts.MerchantId != ext.ProviderMerchantId) ||
			(ext.ProviderEnvironment != "" && facts.Environment != ext.ProviderEnvironment) {
			return fmt.Errorf("%w: payment identity", ErrPaymentAmountMismatch)
		}
	default:
		return ErrPaymentMethodMismatch
	}
	return nil
}

// CompleteAdtyTopUp 包一层事务入账，提交后同步额度缓存、写充值日志。
func CompleteAdtyTopUp(tradeNo string, facts AdtyPaymentFacts, callerIp string) (alreadyDone bool, err error) {
	var credited int
	var topUp *TopUp
	var ext *AdtyTopupOrder
	err = DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		alreadyDone, credited, topUp, ext, txErr = CompleteAdtyTopUpTx(tx, tradeNo, facts)
		return txErr
	})
	if errors.Is(err, ErrAdtyDuplicatePayment) {
		recordAdtyRejectedPayment(tradeNo, facts)
		recordDuplicatePaymentReview(tradeNo, facts, err)
		return false, err
	}
	if err != nil {
		recordAdtyRejectedPayment(tradeNo, facts)
		return false, err
	}
	if alreadyDone {
		return true, nil
	}
	finishAdtyTopUpCredit(topUp, ext, credited, callerIp)
	return false, nil
}

// FinishAdtyTopUpCredit 是入账提交以后的收尾：同步缓存、写日志、通知钩子。补单在自己的事务里入账后也调它。
func FinishAdtyTopUpCredit(topUp *TopUp, ext *AdtyTopupOrder, credited int, callerIp string) {
	finishAdtyTopUpCredit(topUp, ext, credited, callerIp)
}

func finishAdtyTopUpCredit(topUp *TopUp, ext *AdtyTopupOrder, credited int, callerIp string) {
	syncCreditUserQuotaCache(topUp.UserId, credited, common.LogText("adty topup"))
	common.SysLog(common.LogText("adty top-up credited trade_no=%s user_id=%d quota=%d", topUp.TradeNo, topUp.UserId, credited))
	RecordTopupLog(topUp.UserId, common.NewMessage("Top-up credited: {{face}} plus {{bonus}} bonus, paid {{paid}} {{currency}}", map[string]any{
		"face":     "$" + ext.FaceUSD.StringFixed(2),
		"bonus":    "$" + ext.BonusUSD.StringFixed(2),
		"paid":     ext.PaidAmount.String(),
		"currency": ext.PaidCurrency,
		"quota":    logger.FormatQuota(credited),
	}), callerIp, topUp.PaymentMethod, topUp.PaymentProvider)
	if AdtyAfterTopUpCompleted != nil {
		AdtyAfterTopUpCompleted(topUp, ext)
	}
}

// CancelAdtyTopUp 取消本人的 pending 订单。
func CancelAdtyTopUp(tradeNo string, userId int) (*AdtyTopupOrder, error) {
	return CancelAdtyTopUpAfterLock(tradeNo, userId, nil)
}

// ListUserPendingAdtyTradeNos returns the user's still-payable order numbers.
func ListUserPendingAdtyTradeNos(userId int) ([]string, error) {
	var tradeNos []string
	err := DB.Table("adty_topup_orders AS a").
		Joins("JOIN top_ups t ON t.trade_no = a.trade_no").
		Where("a.user_id = ? AND t.status = ?", userId, common.TopUpStatusPending).
		Order("a.id ASC").
		Pluck("a.trade_no", &tradeNos).Error
	return tradeNos, err
}

// ReplaceAdtyTopUpAfterLock freezes a pending order the user is abandoning by
// starting another payment. A later success callback is reviewed instead of
// credited, unlike a user-canceled order that may still settle under PAY-06.
func ReplaceAdtyTopUpAfterLock(tradeNo string, userId int, confirm func(*AdtyTopupOrder) error) (*AdtyTopupOrder, error) {
	var ext *AdtyTopupOrder
	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp, locked, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		if topUp.UserId != userId {
			return ErrTopUpNotFound
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrAdtyTopUpNotCancel
		}
		if confirm != nil {
			if err := confirm(locked); err != nil {
				return err
			}
		}
		ext = locked
		return tx.Model(&TopUp{}).Where("id = ?", topUp.Id).Update("status", TopUpStatusReplaced).Error
	})
	return ext, err
}

// CancelAdtyTopUpAfterLock 先锁住订单。confirm 在锁内调用，用来问支付商。
// confirm 返回错误时本地状态不变。订单已经不是 pending 时不会调用 confirm。
func CancelAdtyTopUpAfterLock(tradeNo string, userId int, confirm func(*AdtyTopupOrder) error) (*AdtyTopupOrder, error) {
	var ext *AdtyTopupOrder
	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp, locked, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		if topUp.UserId != userId {
			return ErrTopUpNotFound
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrAdtyTopUpNotCancel
		}
		if confirm != nil {
			if err := confirm(locked); err != nil {
				return err
			}
		}
		ext = locked
		return tx.Model(&TopUp{}).Where("id = ?", topUp.Id).Update("status", TopUpStatusCanceled).Error
	})
	return ext, err
}

// ExpiredAdtyTopUp 是被过期任务改成 expired 的订单（5.3 按通道计数要用）。
type ExpiredAdtyTopUp struct {
	TradeNo   string
	Provider  string
	ChannelId int
}

// ExpireAdtyTopUps 把到期的 pending 订单改成 expired，一条语句完成（只用 PostgreSQL）。
func ExpireAdtyTopUps(now int64, limit int) ([]ExpiredAdtyTopUp, error) {
	if limit <= 0 {
		limit = 200
	}
	var tradeNos []string
	err := DB.Raw(`UPDATE top_ups SET status = ? WHERE status = ? AND trade_no IN (
		SELECT trade_no FROM adty_topup_orders WHERE expire_at > 0 AND expire_at < ? ORDER BY expire_at LIMIT ?
	) RETURNING trade_no`, common.TopUpStatusExpired, common.TopUpStatusPending, now, limit).Scan(&tradeNos).Error
	if err != nil || len(tradeNos) == 0 {
		return nil, err
	}
	var exts []AdtyTopupOrder
	if err := DB.Select("trade_no", "provider", "channel_id").Where("trade_no IN ?", tradeNos).Find(&exts).Error; err != nil {
		return nil, err
	}
	out := make([]ExpiredAdtyTopUp, 0, len(exts))
	for _, ext := range exts {
		out = append(out, ExpiredAdtyTopUp{TradeNo: ext.TradeNo, Provider: ext.Provider, ChannelId: ext.ChannelId})
	}
	return out, nil
}

// AdtyTopUpView 是用户和后台看到的一笔订单：top_ups 加扩展行。
type AdtyTopUpView struct {
	TopUp
	Ext *AdtyTopupOrder `json:"ext"`
}

// GetUserAdtyTopUp 取本人的一笔订单；别人的订单返回 ErrTopUpNotFound。
func GetUserAdtyTopUp(tradeNo string, userId int) (*AdtyTopUpView, error) {
	var topUps []TopUp
	if err := DB.Where("trade_no = ? AND user_id = ?", tradeNo, userId).Limit(1).Find(&topUps).Error; err != nil {
		return nil, err
	}
	if len(topUps) == 0 {
		return nil, ErrTopUpNotFound
	}
	ext, err := GetAdtyTopupOrder(tradeNo)
	if err != nil {
		return nil, err
	}
	if ext == nil {
		return nil, ErrTopUpNotFound
	}
	return &AdtyTopUpView{TopUp: topUps[0], Ext: ext}, nil
}

// GetUserPendingAdtyTopUp 返回本人最新的一笔还没过期的 pending 订单，没有时为 nil。
func GetUserPendingAdtyTopUp(userId int, now int64) (*AdtyTopUpView, error) {
	var exts []AdtyTopupOrder
	err := DB.Table("adty_topup_orders AS a").Select("a.*").
		Joins("JOIN top_ups t ON t.trade_no = a.trade_no").
		Where("a.user_id = ? AND t.status = ? AND a.expire_at > ?", userId, common.TopUpStatusPending, now).
		Order("a.id DESC").Limit(1).Scan(&exts).Error
	if err != nil || len(exts) == 0 {
		return nil, err
	}
	return GetUserAdtyTopUp(exts[0].TradeNo, userId)
}

// ListUserAdtyTopUps 分页列出本人的二开订单。
func ListUserAdtyTopUps(userId int, pageInfo *common.PageInfo) ([]AdtyTopUpView, int64, error) {
	query := DB.Model(&TopUp{}).Where("user_id = ? AND payment_provider IN ?", userId, []string{
		PaymentProviderAdtyBEpusdt, PaymentProviderAdtyEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake,
	})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var topUps []TopUp
	if err := query.Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topUps).Error; err != nil {
		return nil, 0, err
	}
	tradeNos := make([]string, 0, len(topUps))
	for _, topUp := range topUps {
		tradeNos = append(tradeNos, topUp.TradeNo)
	}
	byTradeNo := map[string]*AdtyTopupOrder{}
	if len(tradeNos) > 0 {
		var exts []AdtyTopupOrder
		if err := DB.Where("trade_no IN ?", tradeNos).Find(&exts).Error; err != nil {
			return nil, 0, err
		}
		for i := range exts {
			byTradeNo[exts[i].TradeNo] = &exts[i]
		}
	}
	out := make([]AdtyTopUpView, 0, len(topUps))
	for _, topUp := range topUps {
		out = append(out, AdtyTopUpView{TopUp: topUp, Ext: byTradeNo[topUp.TradeNo]})
	}
	return out, total, nil
}
