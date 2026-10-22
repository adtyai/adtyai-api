package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TopupOrderStatusPending = "pending"
	TopupOrderStatusPaid    = "paid"
	TopupOrderStatusFailed  = "failed"

	RefundStatusNone    = "none"
	RefundStatusPartial = "partial"
	RefundStatusFull    = "full"
)

// TopupOrder 二开充值订单（含档位/赠送/首充快照）。与官方 topups 表并存，不删官方。
type TopupOrder struct {
	Id                    int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	OrderNo               string  `json:"order_no" gorm:"uniqueIndex;type:varchar(64);not null"`
	UserId                int     `json:"user_id" gorm:"index;not null"`
	AmountUSD             int64   `json:"amount_usd" gorm:"not null"` // 档位美元
	PaidAmountUSD         float64 `json:"paid_amount_usd" gorm:"type:decimal(18,6);not null"`
	CreditAmountUSD       float64 `json:"credit_amount_usd" gorm:"type:decimal(18,6);not null"` // 实充入账美元
	BonusAmountUSD        float64 `json:"bonus_amount_usd" gorm:"type:decimal(18,6);not null"`  // 立刻入赠送账美元
	PendingExtraUSD       float64 `json:"pending_extra_usd" gorm:"type:decimal(18,6);not null;default:0"`
	TierIndex             int     `json:"tier_index" gorm:"not null"`
	TierBonusRate         float64 `json:"tier_bonus_rate" gorm:"type:decimal(10,6);not null"`  // 0–1 快照
	FirstBonusRate        float64 `json:"first_bonus_rate" gorm:"type:decimal(10,6);not null"` // 0–1 快照
	IsFirstCandidate      bool    `json:"is_first_candidate"`
	ApplyFirstImmediately bool    `json:"apply_first_immediately"`
	PendingReason         string  `json:"pending_reason" gorm:"type:varchar(64)"`
	DeviceFp              string  `json:"device_fp" gorm:"type:varchar(128)"`
	ClientIP              string  `json:"client_ip" gorm:"type:varchar(64)"`
	PaymentMethod         string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider       string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	EpayInstanceId        int64   `json:"epay_instance_id" gorm:"index;default:0"`
	EpayPayURL            string  `json:"epay_pay_url,omitempty" gorm:"type:varchar(512);default:''"`
	EpayPayParams         string  `json:"-" gorm:"type:text"`
	FxRate                float64 `json:"fx_rate" gorm:"type:decimal(18,6);default:0"`
	PayCNY                float64 `json:"pay_cny" gorm:"type:decimal(18,6);default:0"`
	PayUSDT               float64 `json:"pay_usdt" gorm:"type:decimal(18,8);default:0"`
	UsdtFxRate            float64 `json:"usdt_fx_rate" gorm:"type:decimal(18,8);default:0"`
	UsdtExpireAt          int64   `json:"usdt_expire_at" gorm:"default:0"`
	UsdtChain             string  `json:"usdt_chain" gorm:"type:varchar(32);default:''"`
	UsdtPayAddress        string  `json:"usdt_pay_address" gorm:"type:varchar(128);default:''"`
	UsdtPaymentURL        string  `json:"usdt_payment_url" gorm:"type:varchar(512);default:''"`
	UsdtPaymentAmount     string  `json:"usdt_payment_amount" gorm:"type:varchar(64);default:''"`
	// UsdtCallbackAmount is the verified callback paid USDT when review starts. Nil means the callback had no usable amount. Credit still uses the order amount.
	UsdtCallbackAmount *float64 `json:"usdt_callback_amount,omitempty" gorm:"type:decimal(18,8)"`
	BonusRateSnapshot  float64  `json:"bonus_rate_snapshot" gorm:"type:decimal(10,6);default:0"`
	ProviderTradeNo    string   `json:"provider_trade_no" gorm:"type:varchar(128);index"` // UNIQUE partial: EnsureTopupOrderIndexes
	Status             string   `json:"status" gorm:"type:varchar(32);index;not null;default:'pending'"`
	PaidCreditedQuota  int64    `json:"paid_credited_quota" gorm:"type:bigint;default:0"`
	BonusCreditedQuota int64    `json:"bonus_credited_quota" gorm:"type:bigint;default:0"`
	RefundStatus       string   `json:"refund_status" gorm:"type:varchar(16);not null;default:'none'"` // none|partial|full
	RefundedAmount     float64  `json:"refunded_amount" gorm:"type:decimal(18,6);not null;default:0"`  // 累计已退美元
	CreateTime         int64    `json:"create_time" gorm:"index"`
	CompleteTime       int64    `json:"complete_time"`
}

func (TopupOrder) TableName() string { return "topup_orders" }

var (
	ErrTopupOrderNotFound          = errors.New("topup order not found")
	ErrTopupOrderStatusInvalid     = errors.New("topup order status invalid")
	ErrRefundConfirmRequired       = errors.New("confirm=true required")
	ErrRefundRequestIdRequired     = errors.New("request_id required")
	ErrRefundAmountInvalid         = errors.New("refund amount invalid")
	ErrRefundReasonRequired        = errors.New("reason required")
	ErrRefundExceedsAvailable      = errors.New("refund exceeds available")
	ErrRefundIdempotent            = errors.New("refund request_id exists")
	ErrRefundOrderNotPaid          = errors.New("topup order not paid")
	ErrRefundPreviewRequired       = errors.New("refund preview required")
	ErrRefundPreviewStale          = errors.New("refund preview stale")
	ErrRefundAmountConfirmRequired = errors.New("refund amount confirm required")
)

// RefundLargeAmountUSD is the threshold that requires the operator to retype the amount.
const RefundLargeAmountUSD = 100

// RefundExceedsAvailableError carries available/amount for console params (no numbers in message).
type RefundExceedsAvailableError struct {
	Available float64
	Amount    float64
}

func (e *RefundExceedsAvailableError) Error() string { return ErrRefundExceedsAvailable.Error() }
func (e *RefundExceedsAvailableError) Unwrap() error { return ErrRefundExceedsAvailable }

// BuildTopupBonusPlanForUser 计算赠送计划。
// 总裁令：不再查首充资格 / device / IP；结算只走档位。
func BuildTopupBonusPlanForUser(userId int, amountUSD int64, deviceFp, clientIP string) (BonusPlan, error) {
	_ = userId
	_ = deviceFp
	_ = clientIP
	if !IsFixedTopupAmount(amountUSD) {
		return BonusPlan{}, ErrInvalidTopupAmount
	}
	return ComputeBonusPlan(amountUSD, false, false, false)
}

// CreateTopupOrder 校验档位、快照赠送、落库 pending 订单（不发起支付）。
func CreateTopupOrder(userId int, amountUSD int64, paymentMethod, deviceFp, clientIP string) (*TopupOrder, error) {
	plan, err := BuildTopupBonusPlanForUser(userId, amountUSD, deviceFp, clientIP)
	if err != nil {
		return nil, err
	}
	orderNo := fmt.Sprintf("ADTY%dNO%s%d", userId, common.GetRandomString(8), time.Now().Unix())
	order := &TopupOrder{
		OrderNo:               orderNo,
		UserId:                userId,
		AmountUSD:             amountUSD,
		PaidAmountUSD:         float64(amountUSD),
		CreditAmountUSD:       float64(amountUSD),
		BonusAmountUSD:        plan.ImmediateBonusUSD,
		PendingExtraUSD:       plan.PendingExtraUSD,
		TierIndex:             plan.TierIndex,
		TierBonusRate:         plan.TierBonusRate,
		FirstBonusRate:        plan.FirstBonusRate,
		IsFirstCandidate:      plan.IsFirstCandidate,
		ApplyFirstImmediately: plan.ApplyFirstImmediately,
		PendingReason:         plan.PendingReason,
		DeviceFp:              deviceFp,
		ClientIP:              clientIP,
		PaymentMethod:         paymentMethod,
		PaymentProvider:       TopupOrderProviderPlaceholder, // 占位；真易支付/USDT 后续接
		Status:                TopupOrderStatusPending,
		CreateTime:            common.GetTimestamp(),
	}
	if err := DB.Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

// GetTopupOrderByNo 按订单号取单。
func GetTopupOrderByNo(orderNo string) (*TopupOrder, error) {
	var o TopupOrder
	if err := DB.Where("order_no = ?", orderNo).First(&o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopupOrderNotFound
		}
		return nil, err
	}
	return &o, nil
}

// CompleteTopupOrder 支付成功入账：实充/立刻赠送均经 AdjustQuota 写余额+流水（A4/A13）；
// 总裁令：首充多送已停，不再入队 risk_reviews / 不写 first_topup_claims。幂等：已 paid 直接返回。
// 本函数是最小闭环的「回调入账路径」；真支付回调后续应调同一入口。
// 加锁顺序：订单行 →（AdjustQuota 内）user_bonus_wallets → users。
// TopupOrderProviderPlaceholder 下单时的占位 provider；未真实拉起支付前保持此值。
const TopupOrderProviderPlaceholder = "adtyai"

// CompleteTopupOrder 支付回调等系统路径入账（operator_id=0 表示系统）。
func CompleteTopupOrder(orderNo string) (*TopupOrder, error) {
	return completeTopupOrder(orderNo, 0, true)
}

// CompleteTopupOrderBy 入账；operatorID>0 为人工补单的操作人，写入 ledger_entries.operator_id。
func CompleteTopupOrderBy(orderNo string, operatorID int) (*TopupOrder, error) {
	return completeTopupOrder(orderNo, operatorID, true)
}

// CompleteTopupOrderFromUsdtCallback credits pending or expired orders only.
// An order already in need_manual is a ledger conflict and must not be auto-credited.
func CompleteTopupOrderFromUsdtCallback(orderNo string) (*TopupOrder, error) {
	return completeTopupOrder(orderNo, 0, false)
}

func completeTopupOrder(orderNo string, operatorID int, allowNeedManual bool) (*TopupOrder, error) {
	if orderNo == "" {
		return nil, ErrTopupOrderNotFound
	}
	var out *TopupOrder
	newlyPaid := false
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
			return nil // 幂等
		}
		// 审查 N-P2-8：超时置 expired 后，已验签的迟到支付仍须入账。
		// need_manual 只允许管理员补单；USDT 回调传 allowNeedManual=false。
		if order.Status == TopupOrderStatusNeedManual && !allowNeedManual {
			return ErrTopupOrderStatusInvalid
		}
		if order.Status != TopupOrderStatusPending && order.Status != TopupOrderStatusExpired && order.Status != TopupOrderStatusNeedManual {
			return ErrTopupOrderStatusInvalid
		}

		paidQuota, err := DollarsToQuota(order.CreditAmountUSD)
		if err != nil {
			return err
		}
		bonusQuota, err := DollarsToQuota(order.BonusAmountUSD)
		if err != nil {
			return err
		}
		pendingQuota, err := DollarsToQuota(order.PendingExtraUSD)
		if err != nil {
			return err
		}
		_ = pendingQuota // 首充多送已停；历史 PendingExtraUSD 亦不入队

		// 入账经 adjustQuotaKeepUserCache：不直接写 users.quota / bonus，也不在事务里删额度缓存。
		// 锁序仍由 adjustQuota 保证（bonus→users）。订单行已 FOR UPDATE，同事务再锁 wallet/users 不致死锁。

		// 实充。不在事务里删额度缓存；提交后按实付+赠送增量补进已有哈希。
		if paidQuota > 0 {
			if _, err := adjustQuotaKeepUserCache(tx, order.UserId, "paid", int64(paidQuota),
				"adtyai topup paid", "topup_order", order.OrderNo,
				"topup_paid:"+order.OrderNo, operatorID, "topup_paid"); err != nil {
				return err
			}
		}

		// 立刻赠送：仅档位（首充取高已停）；ledger kind 统一 topup_tier_bonus
		if bonusQuota > 0 {
			if _, err := adjustQuotaKeepUserCache(tx, order.UserId, "bonus", int64(bonusQuota),
				"adtyai topup bonus", "topup_order", order.OrderNo,
				"topup_bonus:"+order.OrderNo, operatorID, "topup_tier_bonus"); err != nil {
				return err
			}
		}

		// 总裁令：首充多送风控队列停入队；历史 PendingExtraUSD 快照也不再入队、不写 claim
		_ = order.PendingExtraUSD
		_ = order.IsFirstCandidate
		_ = order.ApplyFirstImmediately

		order.Status = TopupOrderStatusPaid
		order.CompleteTime = common.GetTimestamp()
		order.PaidCreditedQuota = int64(paidQuota)
		order.BonusCreditedQuota = int64(bonusQuota)
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		out = &order
		newlyPaid = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if newlyPaid && out != nil {
		syncTopupOrderCreditCache(out.UserId, out.PaidCreditedQuota, out.BonusCreditedQuota)
	}
	// A7: async aff commission after commit; failures do not block credit
	if newlyPaid && out != nil && out.OrderNo != "" {
		go func(ono string) {
			defer func() {
				if r := recover(); r != nil {
					common.SysError(common.LogText("aff commission panic order=%s: %v", ono, r))
				}
			}()
			if err := ProcessTopupAffCommission(ono); err != nil {
				common.SysError(common.LogText("aff commission failed order=%s: %s", ono, err.Error()))
			}
		}(out.OrderNo)
	}
	return out, nil
}

// syncTopupOrderCreditCache adds the committed paid and bonus quota to an existing
// user hash. A missing hash is left alone so the next read hydrates the committed row.
// Do not delete the hash after a successful increment; in-flight pre-deductions live only there.
func syncTopupOrderCreditCache(userId int, paidQuota, bonusQuota int64) {
	if userId <= 0 {
		return
	}
	total := paidQuota + bonusQuota
	if total <= 0 {
		return
	}
	maxInt := int64(^uint(0) >> 1)
	if total > maxInt {
		common.SysError(common.LogText("adty topup credit cache overflow user_id=%d quota=%d", userId, total))
		_ = invalidateUserCache(userId)
		return
	}
	syncCreditUserQuotaCache(userId, int(total), common.LogText("adty topup order"))
}

// AdminTopupOrderListFilter GET /api/admin/orders 筛选条件。
type AdminTopupOrderListFilter struct {
	Status          string
	UserId          int
	PaymentMethod   string
	PaymentProvider string
	OrderNo         string // 精确匹配
	CreateTimeFrom  int64  // inclusive unix；0 = 不限
	CreateTimeTo    int64  // inclusive unix；0 = 不限
	Limit           int
	Offset          int
	Now             int64 // unix 秒；0 表示 time.Now()。测试可冻结。
}

// ListAdminTopupOrders 管理端订单列表：分页 + 常用筛选；按 id DESC。
func ListAdminTopupOrders(f AdminTopupOrderListFilter) ([]TopupOrder, int64, error) {
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	q := DB.Model(&TopupOrder{})
	if f.Status != "" {
		q = applyAdminTopupStatusFilter(q, f.Status, adminTopupListNow(f.Now))
	}
	if f.UserId > 0 {
		q = q.Where("user_id = ?", f.UserId)
	}
	if f.PaymentMethod != "" {
		q = q.Where("payment_method = ?", f.PaymentMethod)
	}
	if f.PaymentProvider != "" {
		q = q.Where("payment_provider = ?", f.PaymentProvider)
	}
	if f.OrderNo != "" {
		q = q.Where("order_no = ?", f.OrderNo)
	}
	if f.CreateTimeFrom > 0 {
		q = q.Where("create_time >= ?", f.CreateTimeFrom)
	}
	if f.CreateTimeTo > 0 {
		q = q.Where("create_time <= ?", f.CreateTimeTo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []TopupOrder
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	if rows == nil {
		rows = []TopupOrder{}
	}
	return rows, total, nil
}

// GetUsernamesByIds 批量按 id 取用户名（一次查询，供管理端订单列表避免 N+1）。
// 已软删除或不存在的用户不在结果里，调用方取不到即为 ""。
func GetUsernamesByIds(ids []int) (map[int]string, error) {
	out := make(map[int]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	seen := make(map[int]struct{}, len(ids))
	uniq := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return out, nil
	}
	var rows []struct {
		Id       int
		Username string
	}
	if err := DB.Model(&User{}).Select("id", "username").Where("id IN ?", uniq).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Id] = r.Username
	}
	return out, nil
}

func adminTopupListNow(now int64) int64 {
	if now > 0 {
		return now
	}
	return time.Now().Unix()
}

// expired 含原始 expired，以及仍 pending、但付款窗口已过的 USDT。
// pending 排除这些展示过期单，避免同一单同时出现在两个筛选项。
// BEpusdt 行不改写成 expired，迟到转账仍可进核对。
func applyAdminTopupStatusFilter(q *gorm.DB, status string, now int64) *gorm.DB {
	switch status {
	case TopupOrderStatusExpired:
		return q.Where(
			"status = ? OR (status = ? AND LOWER(payment_method) = ? AND usdt_expire_at > 0 AND usdt_expire_at <= ?)",
			TopupOrderStatusExpired, TopupOrderStatusPending, "usdt", now,
		)
	case TopupOrderStatusPending:
		return q.Where(
			"status = ? AND NOT (LOWER(payment_method) = ? AND usdt_expire_at > 0 AND usdt_expire_at <= ?)",
			TopupOrderStatusPending, "usdt", now,
		)
	default:
		return q.Where("status = ?", status)
	}
}
