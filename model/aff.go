package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A7 返佣 / 提现状态与错误。

const (
	AffCommissionSourceTopup        = "topup"
	AffCommissionSourceSubscription = "subscription" // 直接付款订阅实付计佣（余额买订阅不计）

	AffCommissionStatusFrozen    = "frozen"
	AffCommissionStatusAvailable = "available"
	AffCommissionStatusVoided    = "voided"
	AffCommissionStatusBlocked   = "blocked" // 自邀拦截，不计佣

	AffWithdrawStatusPending        = "pending"
	AffWithdrawStatusApproved       = "approved"
	AffWithdrawStatusRejected       = "rejected"
	AffWithdrawStatusPaid           = "paid"
	AffWithdrawStatusPayFailed      = "pay_failed"
	AffWithdrawStatusPayFailedFinal = "pay_failed_final" // 自 pay_unknown 核实失败，不可 retry
	AffWithdrawStatusPayUnknown     = "pay_unknown"

	RiskReviewKindAffSelfInvite = "aff_self_invite"
)

var (
	ErrAffCommissionNotFound      = errors.New("aff commission not found")
	ErrAffCommissionStatusInvalid = errors.New("aff commission status invalid")
	ErrAffWithdrawNotFound        = errors.New("aff withdrawal not found")
	ErrAffWithdrawStatusInvalid   = errors.New("aff withdrawal status invalid")
	ErrAffWithdrawIdempotent      = errors.New("aff withdrawal idem_key exists")
	ErrAffWithdrawInsufficient    = errors.New("aff withdrawable balance insufficient")
	ErrAffWithdrawBelowMin        = errors.New("aff withdraw below minimum")
	ErrAffWithdrawRetryForbidden  = errors.New("aff withdraw retry forbidden")
	ErrAffInviteCycle             = errors.New("invite binding would create a cycle")
	ErrAffConfirmRequired         = errors.New("confirm=true required")
)

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "duplicate") || strings.Contains(s, "unique") || strings.Contains(s, "23505")
}

// isAffCodeUniqueViolation 只重试邀请码冲突。邮箱等其它唯一约束必须原样失败，不能换码再插。
func isAffCodeUniqueViolation(err error) bool {
	if !isUniqueViolation(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "aff_code")
}

// AffCommission 返佣明细；冻结期按生成时快照，改参不影响已有行。
type AffCommission struct {
	Id              int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	InviterId       int        `json:"inviter_id" gorm:"index;not null"`
	InviteeId       int        `json:"invitee_id" gorm:"index;not null"`
	OrderNo         string     `json:"order_no" gorm:"type:varchar(64);uniqueIndex;not null"`
	Source          string     `json:"source" gorm:"type:varchar(32);not null;default:'topup'"`
	PaidAmountUSD   float64    `json:"paid_amount_usd" gorm:"type:decimal(18,6);not null"` // 实付美元（不计赠送）
	RateSnapshot    float64    `json:"rate_snapshot" gorm:"type:decimal(10,6);not null"`
	CommissionQuota int64      `json:"commission_quota" gorm:"type:bigint;not null"`
	FreezeDaysSnap  int        `json:"freeze_days_snapshot" gorm:"not null;default:0"`
	Status          string     `json:"status" gorm:"type:varchar(32);index;not null"`
	UnfreezeAt      time.Time  `json:"unfreeze_at" gorm:"index"`
	BlockReason     string     `json:"block_reason" gorm:"type:varchar(128)"`
	VoidedBy        int        `json:"voided_by" gorm:"default:0"`
	VoidedAt        *time.Time `json:"voided_at"`
	CreatedAt       time.Time  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt       time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AffCommission) TableName() string { return "aff_commissions" }

// AffWithdrawal 提现单；idem_key 唯一；终结前金额保持冻结。
type AffWithdrawal struct {
	Id              int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId          int        `json:"user_id" gorm:"index;not null"`
	AmountQuota     int64      `json:"amount_quota" gorm:"type:bigint;not null"` // 申请扣减的返佣额度（含费前）
	FeeQuota        int64      `json:"fee_quota" gorm:"type:bigint;not null;default:0"`
	NetQuota        int64      `json:"net_quota" gorm:"type:bigint;not null"` // 到账 = amount - fee
	FeeRateSnapshot float64    `json:"fee_rate_snapshot" gorm:"type:decimal(10,6);not null;default:0"`
	MinAmountSnap   float64    `json:"min_amount_snapshot" gorm:"type:decimal(18,6);not null;default:0"`
	IdemKey         string     `json:"idem_key" gorm:"type:varchar(128);uniqueIndex;not null"`
	Status          string     `json:"status" gorm:"type:varchar(32);index;not null"`
	PayAccount      string     `json:"pay_account" gorm:"type:varchar(255)"` // 人工打款备注/账号
	RejectReason    string     `json:"reject_reason" gorm:"type:varchar(255)"`
	OperatorId      int        `json:"operator_id" gorm:"default:0"`
	CreatedAt       time.Time  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt       time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
	ResolvedAt      *time.Time `json:"resolved_at"`
}

func (AffWithdrawal) TableName() string { return "aff_withdrawals" }

// NormalizeAffEmail 自邀比对：去 + 后缀；gmail.com / googlemail.com 去点。
func NormalizeAffEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return ""
	}
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return email
	}
	local, domain := email[:at], email[at+1:]
	if plus := strings.IndexByte(local, '+'); plus >= 0 {
		local = local[:plus]
	}
	if domain == "gmail.com" || domain == "googlemail.com" {
		local = strings.ReplaceAll(local, ".", "")
		domain = "gmail.com"
	}
	return local + "@" + domain
}

// AssertNoInviteCycle 沿上级链检查；出现 userId 自身则成环。
func AssertNoInviteCycle(userId, newInviterId int) error {
	if newInviterId <= 0 {
		return nil
	}
	if userId > 0 && newInviterId == userId {
		return ErrAffInviteCycle
	}
	seen := map[int]struct{}{newInviterId: {}}
	cur := newInviterId
	for i := 0; i < 64; i++ {
		var u User
		if err := DB.Select("id", "inviter_id").Where("id = ?", cur).First(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if u.InviterId <= 0 {
			return nil
		}
		if userId > 0 && u.InviterId == userId {
			return ErrAffInviteCycle
		}
		if _, ok := seen[u.InviterId]; ok {
			return ErrAffInviteCycle
		}
		seen[u.InviterId] = struct{}{}
		cur = u.InviterId
	}
	return ErrAffInviteCycle
}

// ResolveSignupInviter 解析注册邀请码。空码、不存在、成环都不绑邀请人，也不让注册失败。
// 数据库错误原样返回，避免故障时把真实邀请关系静默丢掉。必须在插入事务外调用。
func ResolveSignupInviter(affCode string) (int, error) {
	affCode = strings.TrimSpace(affCode)
	if affCode == "" {
		return 0, nil
	}
	id, err := GetUserIdByAffCode(affCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	if id <= 0 {
		return 0, nil
	}
	if err := AssertNoInviteCycle(0, id); err != nil {
		if errors.Is(err, ErrAffInviteCycle) {
			common.SysLog(common.LogText("signup affiliate code dropped: cyclic inviter %d", id))
			return 0, nil
		}
		return 0, err
	}
	return id, nil
}

// BindInviter 设置邀请关系（含防成环）。已有 inviter 时拒绝改绑（最小实现）。
func BindInviter(userId, inviterId int) error {
	if userId <= 0 || inviterId <= 0 {
		return fmt.Errorf("invalid user/inviter")
	}
	if err := AssertNoInviteCycle(userId, inviterId); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var u User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "inviter_id").Where("id = ?", userId).First(&u).Error; err != nil {
			return err
		}
		if u.InviterId != 0 && u.InviterId != inviterId {
			return fmt.Errorf("inviter already bound")
		}
		if u.InviterId == inviterId {
			return nil
		}
		return tx.Model(&User{}).Where("id = ?", userId).Update("inviter_id", inviterId).Error
	})
}

func lookupUserSignals(userId int) (deviceFp, ip, email string) {
	var u User
	if err := DB.Select("id", "email", "register_ip", "register_device_fp").Where("id = ?", userId).First(&u).Error; err == nil {
		email = u.Email
		deviceFp = u.RegisterDeviceFp
		ip = u.RegisterIp
	}
	if deviceFp != "" && ip != "" {
		return deviceFp, ip, email
	}
	var order TopupOrder
	q := DB.Where("user_id = ? AND status = ?", userId, TopupOrderStatusPaid).Order("id DESC")
	if err := q.Select("device_fp", "client_ip").First(&order).Error; err == nil {
		if deviceFp == "" {
			deviceFp = order.DeviceFp
		}
		if ip == "" {
			ip = order.ClientIP
		}
	}
	return deviceFp, ip, email
}

// DetectAffSelfInvite 被邀人与邀请人 IP / device_fp / 规范化邮箱任一相同 → 自邀。
func DetectAffSelfInvite(inviteeId, inviterId int, inviteeDeviceFp, inviteeIP string) (bool, string) {
	if inviteeId <= 0 || inviterId <= 0 {
		return false, ""
	}
	invFp, invIP, invEmail := lookupUserSignals(inviterId)
	var invitee User
	_ = DB.Select("email", "register_ip", "register_device_fp").Where("id = ?", inviteeId).First(&invitee)
	if inviteeDeviceFp == "" {
		inviteeDeviceFp = invitee.RegisterDeviceFp
	}
	if inviteeIP == "" {
		inviteeIP = invitee.RegisterIp
	}

	neInvitee := NormalizeAffEmail(invitee.Email)
	neInviter := NormalizeAffEmail(invEmail)
	if neInvitee != "" && neInvitee == neInviter {
		return true, "same_email"
	}
	if inviteeDeviceFp != "" && invFp != "" && inviteeDeviceFp == invFp {
		return true, "same_device_fp"
	}
	if inviteeIP != "" && invIP != "" && inviteeIP == invIP {
		return true, "same_ip"
	}
	return false, ""
}

// ProcessTopupAffCommission 充值实付入账后计佣（可同步调；CompleteTopupOrder 异步投递）。
// 计佣基数 = 实付（CreditAmountUSD，缺省 PaidAmountUSD）− 已退金额；full 退或净值≤0 跳过。
// 事务内 FOR UPDATE 锁订单行，与退款互斥。赠送不计。失败可重试；同 order_no 幂等。
func ProcessTopupAffCommission(orderNo string) error {
	if orderNo == "" {
		return ErrTopupOrderNotFound
	}
	// 事务外只读：自邀检测会查全局 DB；SQLite 单连接下不能在持锁事务里再开查询。
	orderPeek, err := GetTopupOrderByNo(orderNo)
	if err != nil {
		return err
	}
	if orderPeek.Status != TopupOrderStatusPaid {
		return nil
	}
	if orderPeek.RefundStatus == RefundStatusFull {
		return nil
	}
	inviteePeek, err := GetUserById(orderPeek.UserId, false)
	if err != nil {
		return err
	}
	if inviteePeek.InviterId <= 0 {
		return nil
	}
	blocked, blockReason := DetectAffSelfInvite(inviteePeek.Id, inviteePeek.InviterId, orderPeek.DeviceFp, orderPeek.ClientIP)

	return DB.Transaction(func(tx *gorm.DB) error {
		var order TopupOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopupOrderNotFound
			}
			return err
		}
		if order.Status != TopupOrderStatusPaid {
			return nil
		}
		if order.RefundStatus == RefundStatusFull {
			return nil
		}

		// 幂等：已有佣金行（含 blocked）则跳过
		var existing AffCommission
		if err := tx.Where("order_no = ?", orderNo).First(&existing).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var invitee User
		if err := tx.Where("id = ?", order.UserId).First(&invitee).Error; err != nil {
			return err
		}
		if invitee.InviterId <= 0 {
			return nil
		}
		inviterId := invitee.InviterId

		paidUSD := order.CreditAmountUSD
		if paidUSD <= 0 {
			paidUSD = order.PaidAmountUSD
		}
		paidUSD -= order.RefundedAmount
		if paidUSD <= 1e-9 {
			return nil
		}

		if blocked {
			commQuota, qerr := DollarsToQuota(paidUSD * GetAffTopupCommissionRate())
			if qerr != nil {
				commQuota = 0
			}
			row := AffCommission{
				InviterId:       inviterId,
				InviteeId:       invitee.Id,
				OrderNo:         orderNo,
				Source:          AffCommissionSourceTopup,
				PaidAmountUSD:   paidUSD,
				RateSnapshot:    GetAffTopupCommissionRate(),
				CommissionQuota: int64(commQuota),
				FreezeDaysSnap:  GetAffFreezeDays(),
				Status:          AffCommissionStatusBlocked,
				UnfreezeAt:      time.Now(),
				BlockReason:     blockReason,
			}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return nil
			}
			return InsertRiskReviewTx(tx, RiskReviewKindAffSelfInvite, invitee.Id, orderNo, paidUSD, blockReason)
		}

		rate := GetAffTopupCommissionRate()
		if rate <= 0 {
			// 不写行。费率日后调高后，补偿任务还能补这笔。
			return nil
		}
		commQuota, err := DollarsToQuota(paidUSD * rate)
		if err != nil {
			return err
		}
		if commQuota <= 0 {
			// 正费率下四舍五入为 0 的单若留在扫描队列里，会占满 LIMIT。记一笔 0 元作废行让它退出队列。
			now := time.Now()
			row := AffCommission{
				InviterId:       inviterId,
				InviteeId:       invitee.Id,
				OrderNo:         orderNo,
				Source:          AffCommissionSourceTopup,
				PaidAmountUSD:   paidUSD,
				RateSnapshot:    rate,
				CommissionQuota: 0,
				FreezeDaysSnap:  GetAffFreezeDays(),
				Status:          AffCommissionStatusVoided,
				UnfreezeAt:      now,
				BlockReason:     "commission rounds to zero",
				VoidedAt:        &now,
			}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			return res.Error
		}
		freezeDays := GetAffFreezeDays()
		if freezeDays < 0 {
			freezeDays = 0
		}
		now := time.Now()
		unfreezeAt := now.Add(time.Duration(freezeDays) * 24 * time.Hour)
		status := AffCommissionStatusFrozen
		if freezeDays == 0 {
			status = AffCommissionStatusAvailable
		}

		// 锁序：订单（已锁）→ user_bonus_wallets → users(id 升序) → 返佣行
		if err := lockAffUsersInOrder(tx, []int{inviterId}); err != nil {
			return err
		}
		row := AffCommission{
			InviterId:       inviterId,
			InviteeId:       invitee.Id,
			OrderNo:         orderNo,
			Source:          AffCommissionSourceTopup,
			PaidAmountUSD:   paidUSD,
			RateSnapshot:    rate,
			CommissionQuota: int64(commQuota),
			FreezeDaysSnap:  freezeDays,
			Status:          status,
			UnfreezeAt:      unfreezeAt,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if status == AffCommissionStatusAvailable {
			if err := creditAffQuotaTx(tx, inviterId, int64(commQuota), "aff_commission", orderNo); err != nil {
				return err
			}
		}
		return nil
	})
}

// CompensateMissingTopupAffCommissions 启动补偿：扫已付款、有邀请人、未全额退、尚无返佣行的充值订单，重跑计佣。
// 用于补回 CompleteTopupOrder 异步 goroutine 在进程重启时丢失的内存任务；非定时任务。
// 扫描侧 JOIN users 且 inviter_id>0，排除 refund_status=full 与净值≤0，避免无邀请人/已全退订单占满 LIMIT 饿死真补算。
// limit<=0 时默认 500。返回本轮新写出佣金行的笔数（空跑 / no-op 不计）；单笔失败只记日志并继续。
func CompensateMissingTopupAffCommissions(limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	// 费率为 0 时 Process 不写行。若仍按 LIMIT 扫描，这批空跑单会一直占住名额。
	// 费率改成正数后下一轮再补历史单，不在费率为 0 时写跳过行。
	if GetAffTopupCommissionRate() <= 0 {
		return 0, nil
	}
	var orderNos []string
	err := DB.Raw(`
SELECT o.order_no
FROM topup_orders o
INNER JOIN users u ON u.id = o.user_id
WHERE o.status = ?
  AND u.inviter_id > 0
  AND o.refund_status <> ?
  AND (CASE WHEN o.credit_amount_usd > 0 THEN o.credit_amount_usd ELSE o.paid_amount_usd END) - o.refunded_amount > 0
  AND NOT EXISTS (SELECT 1 FROM aff_commissions c WHERE c.order_no = o.order_no)
ORDER BY o.id ASC
LIMIT ?
`, TopupOrderStatusPaid, RefundStatusFull, limit).Scan(&orderNos).Error
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, ono := range orderNos {
		var before int64
		if err := DB.Model(&AffCommission{}).Where("order_no = ?", ono).Count(&before).Error; err != nil {
			common.SysError(common.LogText("aff commission compensate count failed order=%s: %s", ono, err.Error()))
			continue
		}
		if err := ProcessTopupAffCommission(ono); err != nil {
			common.SysError(common.LogText("aff commission compensate failed order=%s: %s", ono, err.Error()))
			continue
		}
		var after int64
		if err := DB.Model(&AffCommission{}).Where("order_no = ?", ono).Count(&after).Error; err != nil {
			common.SysError(common.LogText("aff commission compensate recount failed order=%s: %s", ono, err.Error()))
			continue
		}
		if after > before {
			processed++
		}
	}
	return processed, nil
}

const affMaintenanceRoundCap = 20

// CompensateAllMissingAffCommissions 把单次上限扫完，避免启动只补 500 笔后永久停住。
func CompensateAllMissingAffCommissions() (int, error) {
	total := 0
	for round := 0; round < affMaintenanceRoundCap; round++ {
		n, err := CompensateMissingTopupAffCommissions(500)
		if err != nil {
			return total, err
		}
		sub, serr := compensateMissingSubscriptionAffCommissions(500)
		if serr != nil {
			return total + n, serr
		}
		total += n + sub
		if n+sub == 0 {
			return total, nil
		}
	}
	return total, nil
}

// compensateMissingSubscriptionAffCommissions 补直付成功订阅单的返佣。余额购买不计佣。
// 费率为 0 时不扫，避免空转；费率改成正数后下一轮再补历史单。
func compensateMissingSubscriptionAffCommissions(limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	if GetAffSubscriptionCommissionRate() <= 0 {
		return 0, nil
	}
	var tradeNos []string
	err := DB.Raw(`
SELECT o.trade_no
FROM subscription_orders o
INNER JOIN users u ON u.id = o.user_id
WHERE o.status = ?
  AND u.inviter_id > 0
  AND COALESCE(o.payment_provider, '') <> ?
  AND COALESCE(o.payment_method, '') <> ?
  AND o.money > 0
  AND NOT EXISTS (SELECT 1 FROM aff_commissions c WHERE c.order_no = o.trade_no)
ORDER BY o.id ASC
LIMIT ?
`, common.TopUpStatusSuccess, PaymentProviderBalance, PaymentMethodBalance, limit).Scan(&tradeNos).Error
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, tradeNo := range tradeNos {
		var before int64
		if err := DB.Model(&AffCommission{}).Where("order_no = ?", tradeNo).Count(&before).Error; err != nil {
			common.SysError(common.LogText("subscription aff compensate count failed trade=%s: %s", tradeNo, err.Error()))
			continue
		}
		if err := ProcessSubscriptionAffCommission(tradeNo); err != nil {
			common.SysError(common.LogText("subscription aff compensate failed trade=%s: %s", tradeNo, err.Error()))
			continue
		}
		var after int64
		if err := DB.Model(&AffCommission{}).Where("order_no = ?", tradeNo).Count(&after).Error; err != nil {
			common.SysError(common.LogText("subscription aff compensate recount failed trade=%s: %s", tradeNo, err.Error()))
			continue
		}
		if after > before {
			processed++
		}
	}
	return processed, nil
}

// isSubscriptionBalancePayment 余额买订阅不计佣（PaymentProvider/Method=balance）。
func isSubscriptionBalancePayment(order *SubscriptionOrder) bool {
	if order == nil {
		return false
	}
	if order.PaymentProvider == PaymentProviderBalance || order.PaymentMethod == PaymentMethodBalance {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(order.PaymentProvider), "balance") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(order.PaymentMethod), "balance") {
		return true
	}
	return false
}

// ProcessSubscriptionAffCommission 订阅直接付款入账后计佣（可同步调；CompleteSubscriptionOrder 异步投递）。
// 只按订单 Money 实付；余额买订阅直接跳过。失败可重试；同 trade_no 幂等。
// 费率读 GetAffSubscriptionCommissionRate（独立于充值 GetAffTopupCommissionRate）；冻结期仍用 GetAffFreezeDays。
func ProcessSubscriptionAffCommission(tradeNo string) error {
	if tradeNo == "" {
		return ErrSubscriptionOrderNotFound
	}
	order := GetSubscriptionOrderByTradeNo(tradeNo)
	if order == nil {
		return ErrSubscriptionOrderNotFound
	}
	if order.Status != common.TopUpStatusSuccess {
		return nil
	}
	// 余额买订阅：明确不计佣
	if isSubscriptionBalancePayment(order) {
		return nil
	}
	// 幂等：已有佣金行（含 blocked）则跳过
	var existing AffCommission
	if err := DB.Where("order_no = ?", tradeNo).First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	invitee, err := GetUserById(order.UserId, false)
	if err != nil {
		return err
	}
	if invitee.InviterId <= 0 {
		return nil
	}
	inviterId := invitee.InviterId

	paidUSD := order.Money
	if paidUSD <= 0 {
		return nil
	}

	// 订阅订单无 device_fp/ip 字段，自邀比对走用户注册信号
	if blocked, reason := DetectAffSelfInvite(invitee.Id, inviterId, "", ""); blocked {
		commQuota, qerr := DollarsToQuota(paidUSD * GetAffSubscriptionCommissionRate())
		if qerr != nil {
			commQuota = 0
		}
		return DB.Transaction(func(tx *gorm.DB) error {
			row := AffCommission{
				InviterId:       inviterId,
				InviteeId:       invitee.Id,
				OrderNo:         tradeNo,
				Source:          AffCommissionSourceSubscription,
				PaidAmountUSD:   paidUSD,
				RateSnapshot:    GetAffSubscriptionCommissionRate(),
				CommissionQuota: int64(commQuota),
				FreezeDaysSnap:  GetAffFreezeDays(),
				Status:          AffCommissionStatusBlocked,
				UnfreezeAt:      time.Now(),
				BlockReason:     reason,
			}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return nil
			}
			return InsertRiskReviewTx(tx, RiskReviewKindAffSelfInvite, invitee.Id, tradeNo, paidUSD, reason)
		})
	}

	rate := GetAffSubscriptionCommissionRate()
	if rate <= 0 {
		return nil
	}
	commUSD := paidUSD * rate
	commQuota, err := DollarsToQuota(commUSD)
	if err != nil {
		return err
	}
	if commQuota <= 0 {
		return nil
	}
	freezeDays := GetAffFreezeDays()
	if freezeDays < 0 {
		freezeDays = 0
	}
	now := time.Now()
	unfreezeAt := now.Add(time.Duration(freezeDays) * 24 * time.Hour)
	status := AffCommissionStatusFrozen
	if freezeDays == 0 {
		status = AffCommissionStatusAvailable
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		ids := []int{inviterId}
		if err := lockAffUsersInOrder(tx, ids); err != nil {
			return err
		}
		row := AffCommission{
			InviterId:       inviterId,
			InviteeId:       invitee.Id,
			OrderNo:         tradeNo,
			Source:          AffCommissionSourceSubscription,
			PaidAmountUSD:   paidUSD,
			RateSnapshot:    rate,
			CommissionQuota: int64(commQuota),
			FreezeDaysSnap:  freezeDays,
			Status:          status,
			UnfreezeAt:      unfreezeAt,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if status == AffCommissionStatusAvailable {
			if err := creditAffQuotaTx(tx, inviterId, int64(commQuota), "aff_commission", tradeNo); err != nil {
				return err
			}
		}
		return nil
	})
}

func lockAffUsersInOrder(tx *gorm.DB, userIds []int) error {
	// unique + sort ascending
	seen := map[int]struct{}{}
	var ids []int
	for _, id := range userIds {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	for _, id := range ids {
		if err := EnsureUserBonusWallet(tx, id); err != nil {
			return err
		}
		var w UserBonusWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", id).First(&w).Error; err != nil {
			return err
		}
	}
	for _, id := range ids {
		var u User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "quota", "aff_quota", "aff_history").Where("id = ?", id).First(&u).Error; err != nil {
			return err
		}
	}
	return nil
}

func creditAffQuotaTx(tx *gorm.DB, userId int, delta int64, reason, refId string) error {
	if delta == 0 {
		return nil
	}
	if err := tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]any{
		"aff_quota":   gorm.Expr("aff_quota + ?", delta),
		"aff_history": gorm.Expr("aff_history + ?", delta),
	}).Error; err != nil {
		return err
	}
	var after int
	if err := tx.Model(&User{}).Where("id = ?", userId).Select("aff_quota").Scan(&after).Error; err != nil {
		return err
	}
	return writeLedgerEntryTx(tx, userId, "aff", reason, delta, int64(after), reason, "aff_commission", refId, 0)
}

func debitAffQuotaTx(tx *gorm.DB, userId int, delta int64, reason, refId string, operatorId int) error {
	if delta == 0 {
		return nil
	}
	res := tx.Model(&User{}).Where("id = ? AND aff_quota >= ?", userId, delta).
		Update("aff_quota", gorm.Expr("aff_quota - ?", delta))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAffWithdrawInsufficient
	}
	var after int
	if err := tx.Model(&User{}).Where("id = ?", userId).Select("aff_quota").Scan(&after).Error; err != nil {
		return err
	}
	return writeLedgerEntryTx(tx, userId, "aff", reason, -delta, int64(after), reason, "aff_withdrawal", refId, operatorId)
}

// ReleaseDueAffCommissions 将到期冻结返佣转为可提现并入 aff_quota。
func ReleaseDueAffCommissions(userId int) (int, error) {
	now := time.Now()
	var rows []AffCommission
	q := DB.Where("status = ? AND unfreeze_at <= ?", AffCommissionStatusFrozen, now)
	if userId > 0 {
		q = q.Where("inviter_id = ?", userId)
	}
	if err := q.Order("id ASC").Limit(200).Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		err := DB.Transaction(func(tx *gorm.DB) error {
			// 锁序：users 先，返佣行最后
			if err := lockAffUsersInOrder(tx, []int{row.InviterId}); err != nil {
				return err
			}
			var c AffCommission
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", row.Id).First(&c).Error; err != nil {
				return err
			}
			if c.Status != AffCommissionStatusFrozen || c.UnfreezeAt.After(time.Now()) {
				return nil
			}
			c.Status = AffCommissionStatusAvailable
			if err := tx.Save(&c).Error; err != nil {
				return err
			}
			return creditAffQuotaTx(tx, c.InviterId, c.CommissionQuota, "aff_unfreeze", c.OrderNo)
		})
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ReleaseAllDueAffCommissions 循环解冻，直到本轮没有到期行或达到轮次上限。
func ReleaseAllDueAffCommissions() (int, error) {
	total := 0
	for round := 0; round < affMaintenanceRoundCap; round++ {
		n, err := ReleaseDueAffCommissions(0)
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, nil
		}
	}
	return total, nil
}

func reclaimableAffWithdrawStatuses() []string {
	return []string{
		AffWithdrawStatusPending,
		AffWithdrawStatusApproved,
		AffWithdrawStatusPayFailed,
		AffWithdrawStatusPayFailedFinal,
	}
}

// reclaimInFlightAffWithdrawalsTx 把尚未打出的提现退回 aff_quota，直到余额覆盖 need。
// 调用方必须已经按锁序持有该用户行。pay_unknown 可能已经打款，不自动取消。
// 不要调用 RejectAffWithdrawal：它另开事务。当前拒绝、改状态都是先锁用户再锁提现，和退款同序。
func reclaimInFlightAffWithdrawalsTx(tx *gorm.DB, userId int, need int64, operatorId int, reason string) error {
	if tx == nil || userId <= 0 || need <= 0 {
		return nil
	}
	var aff int
	if err := tx.Model(&User{}).Where("id = ?", userId).Select("aff_quota").Scan(&aff).Error; err != nil {
		return err
	}
	if int64(aff) >= need {
		return nil
	}
	var rows []AffWithdrawal
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND status IN ?", userId, reclaimableAffWithdrawStatuses()).
		Order("id DESC").
		Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		if int64(aff) >= need {
			break
		}
		w := rows[i]
		if w.AmountQuota <= 0 {
			continue
		}
		if err := tx.Model(&User{}).Where("id = ?", userId).
			Update("aff_quota", gorm.Expr("aff_quota + ?", w.AmountQuota)).Error; err != nil {
			return err
		}
		if err := tx.Model(&User{}).Where("id = ?", userId).Select("aff_quota").Scan(&aff).Error; err != nil {
			return err
		}
		if err := writeLedgerEntryTx(tx, userId, "aff", "aff_withdraw_unfreeze", w.AmountQuota, int64(aff),
			reason, "aff_withdrawal", fmt.Sprintf("%d", w.Id), operatorId); err != nil {
			return err
		}
		now := time.Now()
		res := tx.Model(&AffWithdrawal{}).Where("id = ? AND status = ?", w.Id, w.Status).Updates(map[string]any{
			"status":        AffWithdrawStatusRejected,
			"reject_reason": reason,
			"operator_id":   operatorId,
			"resolved_at":   now,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("withdrawal status changed during reclaim")
		}
	}
	return nil
}

// StartAffMaintenanceLoop 主节点定时解冻到期返佣并补漏。立即跑一轮，之后每分钟一次。
func StartAffMaintenanceLoop() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		runAffMaintenance()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			runAffMaintenance()
		}
	}()
}

func runAffMaintenance() {
	if n, err := ReleaseAllDueAffCommissions(); err != nil {
		common.SysError(common.LogText("aff release loop failed after %d: %s", n, err.Error()))
	} else if n > 0 {
		common.SysLog(common.LogText("aff release loop released %d", n))
	}
	if n, err := CompensateAllMissingAffCommissions(); err != nil {
		common.SysError(common.LogText("aff compensate loop failed after %d: %s", n, err.Error()))
	} else if n > 0 {
		common.SysLog(common.LogText("aff compensate loop processed %d", n))
	}
}

// VoidAffCommission 管理端作废：冲正可提现余额 + 审计由 controller 写。需在外层 confirm。
// ErrAffCommissionAlreadyWithdrawn 返佣已被提现，可用返佣余额不足以冲正（审查 N-P1-6）。
var ErrAffCommissionAlreadyWithdrawn = errors.New("aff commission already withdrawn; available aff balance insufficient to reverse")

func VoidAffCommission(id int64, operatorId int) (*AffCommission, error) {
	var out *AffCommission
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 锁序：先 peek → lock users → 再锁返佣行
		var peek AffCommission
		if err := tx.Where("id = ?", id).First(&peek).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAffCommissionNotFound
			}
			return err
		}
		if peek.Status == AffCommissionStatusVoided {
			out = &peek
			return nil
		}
		if peek.Status == AffCommissionStatusBlocked {
			return ErrAffCommissionStatusInvalid
		}
		if err := lockAffUsersInOrder(tx, []int{peek.InviterId}); err != nil {
			return err
		}
		var c AffCommission
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&c).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAffCommissionNotFound
			}
			return err
		}
		if c.Status == AffCommissionStatusVoided {
			out = &c
			return nil
		}
		if c.Status == AffCommissionStatusBlocked {
			return ErrAffCommissionStatusInvalid
		}
		if c.Status == AffCommissionStatusAvailable {
			// 先收回尚未打出的提现，再判断余额够不够冲正。pay_unknown 不自动取消。
			// 仍不够则拒绝，不把 aff_quota 扣成负数，也不去扣用户实充。
			if err := reclaimInFlightAffWithdrawalsTx(tx, c.InviterId, c.CommissionQuota, operatorId, "void reclaimed unpaid withdrawal"); err != nil {
				return err
			}
			res := tx.Model(&User{}).Where("id = ? AND aff_quota >= ?", c.InviterId, c.CommissionQuota).Updates(map[string]any{
				"aff_quota": gorm.Expr("aff_quota - ?", c.CommissionQuota),
			})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrAffCommissionAlreadyWithdrawn
			}
			var after int
			if err := tx.Model(&User{}).Where("id = ?", c.InviterId).Select("aff_quota").Scan(&after).Error; err != nil {
				return err
			}
			if err := writeLedgerEntryTx(tx, c.InviterId, "aff", "aff_void", -c.CommissionQuota, int64(after),
				"void aff commission", "aff_commission", fmt.Sprintf("%d", c.Id), operatorId); err != nil {
				return err
			}
		}
		now := time.Now()
		c.Status = AffCommissionStatusVoided
		c.VoidedBy = operatorId
		c.VoidedAt = &now
		if err := tx.Save(&c).Error; err != nil {
			return err
		}
		out = &c
		return nil
	})
	return out, err
}

// CreateAffWithdrawal 用户申请提现；idem_key 唯一；只动返佣账户。
// 锁序：user_bonus_wallets → users →（创建提现单）。
// 持用户锁后重查 idem；Create 撞唯一则整单回滚，再读回已有单（不再扣款）。
func CreateAffWithdrawal(userId int, amountQuota int64, idemKey, payAccount string) (*AffWithdrawal, error) {
	if userId <= 0 || amountQuota <= 0 || strings.TrimSpace(idemKey) == "" {
		return nil, fmt.Errorf("invalid withdraw request")
	}
	idemKey = strings.TrimSpace(idemKey)
	if _, err := ReleaseDueAffCommissions(userId); err != nil {
		common.SysError(common.LogText("ReleaseDueAffCommissions: %s", err.Error()))
	}

	// 幂等：已存在同 key 直接返回（无锁快路径）
	var existing AffWithdrawal
	if err := DB.Where("idem_key = ?", idemKey).First(&existing).Error; err == nil {
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	minUSD := GetAffMinWithdrawAmount()
	minQuota, err := DollarsToQuota(minUSD)
	if err != nil {
		return nil, err
	}
	if minQuota > 0 && amountQuota < int64(minQuota) {
		return nil, ErrAffWithdrawBelowMin
	}
	feeRate := GetAffWithdrawFeeRate()
	if feeRate < 0 {
		feeRate = 0
	}
	feeQuota := int64(float64(amountQuota) * feeRate)
	if feeQuota < 0 {
		feeQuota = 0
	}
	if feeQuota >= amountQuota {
		feeQuota = amountQuota - 1
		if feeQuota < 0 {
			feeQuota = 0
		}
	}
	netQuota := amountQuota - feeQuota

	var out *AffWithdrawal
	err = DB.Transaction(func(tx *gorm.DB) error {
		// 先持用户锁，再查 idem，避免「先扣款再撞唯一并 commit」双扣
		if err := lockAffUsersInOrder(tx, []int{userId}); err != nil {
			return err
		}
		var again AffWithdrawal
		if err := tx.Where("idem_key = ?", idemKey).First(&again).Error; err == nil {
			out = &again
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var u User
		if err := tx.Select("id", "aff_quota").Where("id = ?", userId).First(&u).Error; err != nil {
			return err
		}
		if int64(u.AffQuota) < amountQuota {
			return ErrAffWithdrawInsufficient
		}
		if err := debitAffQuotaTx(tx, userId, amountQuota, "aff_withdraw_freeze", idemKey, 0); err != nil {
			return err
		}
		row := AffWithdrawal{
			UserId:          userId,
			AmountQuota:     amountQuota,
			FeeQuota:        feeQuota,
			NetQuota:        netQuota,
			FeeRateSnapshot: feeRate,
			MinAmountSnap:   minUSD,
			IdemKey:         idemKey,
			Status:          AffWithdrawStatusPending,
			PayAccount:      payAccount,
		}
		if err := tx.Create(&row).Error; err != nil {
			// 撞唯一 → 返回错误使整单回滚（含已扣款），禁止 commit 半扣
			if isUniqueViolation(err) {
				return ErrAffWithdrawIdempotent
			}
			return err
		}
		out = &row
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrAffWithdrawIdempotent) {
			var got AffWithdrawal
			if e2 := DB.Where("idem_key = ?", idemKey).First(&got).Error; e2 == nil {
				return &got, nil
			}
		}
		return nil, err
	}
	return out, nil
}

func GetAffWithdrawalById(id int64) (*AffWithdrawal, error) {
	var row AffWithdrawal
	if err := DB.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAffWithdrawNotFound
		}
		return nil, err
	}
	return &row, nil
}

func ListAffWithdrawalsByUser(userId int, limit, offset int) ([]AffWithdrawal, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := DB.Model(&AffWithdrawal{}).Where("user_id = ?", userId)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []AffWithdrawal
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func ListAffWithdrawalsAdmin(status string, limit, offset int) ([]AffWithdrawal, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := DB.Model(&AffWithdrawal{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []AffWithdrawal
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func ListAffCommissionsAdmin(status string, inviterId int, limit, offset int) ([]AffCommission, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := DB.Model(&AffCommission{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if inviterId > 0 {
		q = q.Where("inviter_id = ?", inviterId)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []AffCommission
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func ListAffCommissionsByInviter(inviterId int, limit, offset int) ([]AffCommission, int64, error) {
	return ListAffCommissionsAdmin("", inviterId, limit, offset)
}

// --- 提现状态机 ---

func ApproveAffWithdrawal(id int64, operatorId int) (*AffWithdrawal, error) {
	return transitionAffWithdrawal(id, operatorId, func(w *AffWithdrawal) error {
		if w.Status != AffWithdrawStatusPending {
			return ErrAffWithdrawStatusInvalid
		}
		w.Status = AffWithdrawStatusApproved
		return nil
	})
}

func RejectAffWithdrawal(id int64, operatorId int, reason string) (*AffWithdrawal, error) {
	var out *AffWithdrawal
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 锁序：先 peek user_id → lock users → 再锁提现单
		var peek AffWithdrawal
		if err := tx.Where("id = ?", id).First(&peek).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAffWithdrawNotFound
			}
			return err
		}
		if err := lockAffUsersInOrder(tx, []int{peek.UserId}); err != nil {
			return err
		}
		var w AffWithdrawal
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&w).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAffWithdrawNotFound
			}
			return err
		}
		if w.Status == AffWithdrawStatusRejected {
			out = &w
			return nil
		}
		if w.Status != AffWithdrawStatusPending &&
			w.Status != AffWithdrawStatusPayFailed &&
			w.Status != AffWithdrawStatusPayFailedFinal {
			return ErrAffWithdrawStatusInvalid
		}
		// 解冻：退回冻结额
		if err := tx.Model(&User{}).Where("id = ?", w.UserId).Update("aff_quota", gorm.Expr("aff_quota + ?", w.AmountQuota)).Error; err != nil {
			return err
		}
		var after int
		if err := tx.Model(&User{}).Where("id = ?", w.UserId).Select("aff_quota").Scan(&after).Error; err != nil {
			return err
		}
		if err := writeLedgerEntryTx(tx, w.UserId, "aff", "aff_withdraw_unfreeze", w.AmountQuota, int64(after),
			"reject withdraw unfreeze", "aff_withdrawal", fmt.Sprintf("%d", w.Id), operatorId); err != nil {
			return err
		}
		now := time.Now()
		w.Status = AffWithdrawStatusRejected
		w.RejectReason = reason
		w.OperatorId = operatorId
		w.ResolvedAt = &now
		if err := tx.Save(&w).Error; err != nil {
			return err
		}
		out = &w
		return nil
	})
	return out, err
}

func MarkAffWithdrawalPaid(id int64, operatorId int) (*AffWithdrawal, error) {
	return transitionAffWithdrawal(id, operatorId, func(w *AffWithdrawal) error {
		if w.Status != AffWithdrawStatusApproved && w.Status != AffWithdrawStatusPayUnknown {
			return ErrAffWithdrawStatusInvalid
		}
		now := time.Now()
		w.Status = AffWithdrawStatusPaid
		w.ResolvedAt = &now
		// 金额已在申请时从 aff_quota 扣出；paid 不再动余额
		return nil
	})
}

func MarkAffWithdrawalPayFailed(id int64, operatorId int) (*AffWithdrawal, error) {
	return transitionAffWithdrawal(id, operatorId, func(w *AffWithdrawal) error {
		switch w.Status {
		case AffWithdrawStatusApproved:
			// 打款明确失败，可 retry 回 approved
			w.Status = AffWithdrawStatusPayFailed
		case AffWithdrawStatusPayUnknown:
			// 自 unknown 核实为失败：终态不可 retry，堵住 unknown→failed→retry 绕开
			w.Status = AffWithdrawStatusPayFailedFinal
		default:
			return ErrAffWithdrawStatusInvalid
		}
		// 继续冻结，不退回
		return nil
	})
}

func MarkAffWithdrawalPayUnknown(id int64, operatorId int) (*AffWithdrawal, error) {
	return transitionAffWithdrawal(id, operatorId, func(w *AffWithdrawal) error {
		if w.Status != AffWithdrawStatusApproved {
			return ErrAffWithdrawStatusInvalid
		}
		w.Status = AffWithdrawStatusPayUnknown
		return nil
	})
}

// RetryAffWithdrawal 仅 pay_failed → approved；pay_unknown / pay_failed_final 拒绝。
func RetryAffWithdrawal(id int64, operatorId int) (*AffWithdrawal, error) {
	var out *AffWithdrawal
	err := DB.Transaction(func(tx *gorm.DB) error {
		var w AffWithdrawal
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&w).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAffWithdrawNotFound
			}
			return err
		}
		if w.Status == AffWithdrawStatusPayUnknown || w.Status == AffWithdrawStatusPayFailedFinal {
			return ErrAffWithdrawRetryForbidden
		}
		if w.Status != AffWithdrawStatusPayFailed {
			return ErrAffWithdrawStatusInvalid
		}
		w.Status = AffWithdrawStatusApproved
		w.OperatorId = operatorId
		if err := tx.Save(&w).Error; err != nil {
			return err
		}
		out = &w
		return nil
	})
	return out, err
}

func transitionAffWithdrawal(id int64, operatorId int, mutate func(*AffWithdrawal) error) (*AffWithdrawal, error) {
	var out *AffWithdrawal
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 锁序与退款回收、拒绝提现一致：先窥探 user_id，锁用户，再锁提现单。
		var peek AffWithdrawal
		if err := tx.Select("id", "user_id").Where("id = ?", id).First(&peek).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAffWithdrawNotFound
			}
			return err
		}
		if err := lockAffUsersInOrder(tx, []int{peek.UserId}); err != nil {
			return err
		}
		var w AffWithdrawal
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&w).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAffWithdrawNotFound
			}
			return err
		}
		if err := mutate(&w); err != nil {
			return err
		}
		w.OperatorId = operatorId
		if err := tx.Save(&w).Error; err != nil {
			return err
		}
		out = &w
		return nil
	})
	return out, err
}

// GetAffSelfSummary 用户端推广概览。
func GetAffSelfSummary(userId int) (map[string]any, error) {
	if _, err := ReleaseDueAffCommissions(userId); err != nil {
		common.SysError(common.LogText("ReleaseDueAffCommissions: %s", err.Error()))
	}
	var u User
	if err := DB.Select("id", "aff_code", "aff_count", "aff_quota", "aff_history", "inviter_id").
		Where("id = ?", userId).First(&u).Error; err != nil {
		return nil, err
	}
	var frozenSum int64
	_ = DB.Model(&AffCommission{}).
		Where("inviter_id = ? AND status = ?", userId, AffCommissionStatusFrozen).
		Select("COALESCE(SUM(commission_quota),0)").Scan(&frozenSum)
	return map[string]any{
		"aff_code":          u.AffCode,
		"aff_count":         u.AffCount,
		"aff_quota":         u.AffQuota,
		"aff_history":       u.AffHistoryQuota,
		"frozen_quota":      frozenSum,
		"inviter_id":        u.InviterId,
		"min_withdraw_usd":  GetAffMinWithdrawAmount(),
		"withdraw_fee_rate": GetAffWithdrawFeeRate(),
		"freeze_days":       GetAffFreezeDays(),
		"topup_rate":        GetAffTopupCommissionRate(),
	}, nil
}
