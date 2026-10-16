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
	FxRate                float64 `json:"fx_rate" gorm:"type:decimal(18,6);default:0"`
	PayCNY                float64 `json:"pay_cny" gorm:"type:decimal(18,6);default:0"`
	PayUSDT               float64 `json:"pay_usdt" gorm:"type:decimal(18,8);default:0"`
	UsdtFxRate            float64 `json:"usdt_fx_rate" gorm:"type:decimal(18,8);default:0"`
	UsdtExpireAt          int64   `json:"usdt_expire_at" gorm:"default:0"`
	BonusRateSnapshot     float64 `json:"bonus_rate_snapshot" gorm:"type:decimal(10,6);default:0"`
	ProviderTradeNo       string  `json:"provider_trade_no" gorm:"type:varchar(128);index"` // UNIQUE partial: EnsureTopupOrderIndexes
	Status                string  `json:"status" gorm:"type:varchar(32);index;not null;default:'pending'"`
	PaidCreditedQuota     int64   `json:"paid_credited_quota" gorm:"type:bigint;default:0"`
	BonusCreditedQuota    int64   `json:"bonus_credited_quota" gorm:"type:bigint;default:0"`
	RefundStatus          string  `json:"refund_status" gorm:"type:varchar(16);not null;default:'none'"` // none|partial|full
	RefundedAmount        float64 `json:"refunded_amount" gorm:"type:decimal(18,6);not null;default:0"`  // 累计已退美元
	CreateTime            int64   `json:"create_time" gorm:"index"`
	CompleteTime          int64   `json:"complete_time"`
}

func (TopupOrder) TableName() string { return "topup_orders" }

var (
	ErrTopupOrderNotFound      = errors.New("topup order not found")
	ErrTopupOrderStatusInvalid = errors.New("topup order status invalid")
	ErrRefundConfirmRequired   = errors.New("confirm=true required")
	ErrRefundRequestIdRequired = errors.New("request_id required")
	ErrRefundAmountInvalid     = errors.New("refund amount invalid")
	ErrRefundReasonRequired    = errors.New("reason required")
	ErrRefundExceedsAvailable  = errors.New("refund exceeds available")
	ErrRefundIdempotent        = errors.New("refund request_id exists")
	ErrRefundOrderNotPaid      = errors.New("topup order not paid")
)

// RefundExceedsAvailableError carries available/amount for console params (no numbers in message).
type RefundExceedsAvailableError struct {
	Available float64
	Amount    float64
}

func (e *RefundExceedsAvailableError) Error() string { return ErrRefundExceedsAvailable.Error() }
func (e *RefundExceedsAvailableError) Unwrap() error { return ErrRefundExceedsAvailable }

// BuildTopupBonusPlanForUser 读取首充资格并计算赠送计划。
func BuildTopupBonusPlanForUser(userId int, amountUSD int64, deviceFp, clientIP string) (BonusPlan, error) {
	if !IsFixedTopupAmount(amountUSD) {
		return BonusPlan{}, ErrInvalidTopupAmount
	}
	hasRecord, err := UserHasFirstTopupRecord(userId)
	if err != nil {
		return BonusPlan{}, err
	}
	firstCandidate := !hasRecord
	deviceOK := false
	if deviceFp != "" {
		used, err := DeviceFpClaimed(deviceFp)
		if err != nil {
			return BonusPlan{}, err
		}
		deviceOK = !used
	}
	ipConflict := false
	if clientIP != "" {
		ipConflict, err = IPHasClaimedFirstTopup(clientIP)
		if err != nil {
			return BonusPlan{}, err
		}
	}
	return ComputeBonusPlan(amountUSD, firstCandidate, deviceOK, ipConflict)
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
		PaymentProvider:       "adtyai", // 最小闭环占位；真易支付/USDT 后续接
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
// 首充多送 pending → risk_reviews；快照不变。幂等：已 paid 直接返回。
// 本函数是最小闭环的「回调入账路径」；真支付回调后续应调同一入口。
// 加锁顺序：订单行 →（AdjustQuota 内）user_bonus_wallets → users。
func CompleteTopupOrder(orderNo string) (*TopupOrder, error) {
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
		if order.Status != TopupOrderStatusPending {
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
		_ = pendingQuota // 多送进 risk_reviews，审批后再入账（本块不自动加）

		// 入账经 AdjustQuota（A4/A13）：不直接写 users.quota / bonus；锁序由 AdjustQuota 保证（bonus→users）。
		// 订单行已 FOR UPDATE；同事务内 AdjustQuota 再锁 wallet/users 不致死锁。

		// 实充
		if paidQuota > 0 {
			if _, err := AdjustQuota(tx, order.UserId, "paid", int64(paidQuota),
				"adtyai topup paid", "topup_order", order.OrderNo,
				"topup_paid:"+order.OrderNo, 0, "topup_paid"); err != nil {
				return err
			}
		}

		// 立刻赠送（档位或首充取高后的 immediate 部分）
		if bonusQuota > 0 {
			kind := "topup_tier_bonus"
			if order.ApplyFirstImmediately {
				kind = "topup_first_bonus"
			}
			if _, err := AdjustQuota(tx, order.UserId, "bonus", int64(bonusQuota),
				"adtyai topup bonus", "topup_order", order.OrderNo,
				"topup_bonus:"+order.OrderNo, 0, kind); err != nil {
				return err
			}
		}

		// 首充多送进人工待审
		if order.PendingExtraUSD > 0 {
			if err := InsertRiskReviewTx(tx, RiskReviewKindFirstTopupExtra, order.UserId, order.OrderNo,
				order.PendingExtraUSD, order.PendingReason); err != nil {
				return err
			}
		}

		// 领取记录：立刻享首充 → claimed；仅 pending 多送 → pending_review（不占 device 唯一也可写账号行）
		if order.IsFirstCandidate {
			status := FirstTopupStatusPendingReview
			if order.ApplyFirstImmediately {
				status = FirstTopupStatusClaimed
			}
			// 缺指纹时仍写账号行（status=pending_review），device_fp 为 NULL，不触唯一冲突
			if order.ApplyFirstImmediately || order.PendingExtraUSD > 0 {
				if err := InsertFirstTopupClaimTx(tx, order.UserId, order.DeviceFp, order.ClientIP,
					order.OrderNo, order.Id, status); err != nil {
					return err
				}
			}
		}

		order.Status = TopupOrderStatusPaid
		order.CompleteTime = common.GetTimestamp()
		order.PaidCreditedQuota = int64(paidQuota)
		order.BonusCreditedQuota = int64(bonusQuota)
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		out = &order
		newlyPaid = true
		_ = invalidateUserCache(order.UserId)
		return nil
	})
	if err != nil {
		return nil, err
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
		q = q.Where("status = ?", f.Status)
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
