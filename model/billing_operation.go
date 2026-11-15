package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	BillingReserved      = "reserved"
	BillingSettlePending = "settle_pending"
	BillingRefundPending = "refund_pending"
	BillingSettled       = "settled"
	BillingRefunded      = "refunded"
	BillingCanceled      = "canceled"
	// BillingDead 是重试预算耗尽的死信：结算永远成功不了的 op 停在这里等人工，
	// 不再参与恢复扫描，把队头让给还能成功的 op（ISS-R4-0094）。
	//
	// 人工处置有两条路，都不在自动恢复里替人决定：补齐前置条件（例如 `adty_fund_batch`
	// 补批次）后把行放回队列——
	// `UPDATE billing_operations SET state='settle_pending', attempts=0, next_retry_at=0 WHERE operation_key=$1`
	// ——或者按 attempts/last_error 对账后退钱。amounts 都还在行上，没有丢。
	BillingDead = "dead"
)

var (
	ErrBillingOperationConflict  = errors.New("billing operation conflicts with its durable intent")
	ErrBillingWalletInsufficient = errors.New("wallet quota insufficient")
	ErrBillingTokenInsufficient  = errors.New("token quota insufficient")
)

// BillingEarningFactorsUsable reports whether a stored markup k and supply
// cost factor d can book A.10 earnings or be kept on a pricing snapshot.
// k may be below 1 and d may be above 1; only non-positive or non-finite
// values are rejected.
func BillingEarningFactorsUsable(k, d float64) bool {
	return k > 0 && d > 0 && !math.IsNaN(k) && !math.IsNaN(d) && !math.IsInf(k, 0) && !math.IsInf(d, 0)
}

// BillingOperation is the main-database identity and recovery record for a
// reservation and its final disposition. It contains no credential or payload.
// Reserved/applied wallet amounts are distinct from token charges and shortfall.
type BillingOperation struct {
	OperationKey       string `gorm:"type:varchar(191);primaryKey"`
	UserId             int    `gorm:"index;not null"`
	TokenId            int
	TokenUnlimited     bool
	LeaseOwner         string `gorm:"type:varchar(64);index"`
	Kind               string `gorm:"type:varchar(24);not null"` // request, task, mj_charge, mj_refund
	ReferenceId        int64  `gorm:"index"`
	ReservedQuota      int
	TokenReservedQuota int
	StreamWalletQuota  int // independently committed realtime chunks included in the final target
	StreamTokenQuota   int
	ActualQuota        int
	AppliedQuota       int
	TokenAppliedQuota  int
	Shortfall          int
	// FundBatchGap 是「本次实扣」减去「批次真正支撑得起的额度」的差。
	// 它和 Shortfall 不是一回事：Shortfall 是用户余额不够，Gap 是批次账不全
	// （存量用户本来就有一部分额度没有批次）。两者都为 0 才说明账是平的。
	// 对账口径：`select user_id, sum(fund_batch_gap) ... group by user_id`，
	// 非零即有漂移，且能定位到具体 operation。
	FundBatchGap int64  `gorm:"not null;default:0"`
	State        string `gorm:"type:varchar(24);index;not null"`
	// 恢复重试预算。失败的结算整笔回滚，连 autoUpdateTime 写的 updated_at 一起
	// 撤销，所以只靠 `Order("updated_at")` 排序时同一条永久失败的 op 每轮都排在
	// 最前：积够 limit 条就把恢复链彻底饿死。三列让失败真正落到行上：
	// attempts 计连续失败次数，next_retry_at 是下次允许重试的时刻（0 = 立即，
	// 兼容存量行），用完预算的 op 转 BillingDead。
	Attempts            int    `gorm:"not null;default:0"`
	LastError           string `gorm:"type:text"`
	NextRetryAt         int64  `gorm:"not null;default:0;index"`
	SiteId              int
	SiteOwnerUserId     int `gorm:"not null;default:0"`
	SiteMarkup          float64
	SiteCostFactor      float64
	BillingContext      string `gorm:"type:text"`
	PriceSnapshot       string `gorm:"type:text"`
	EarningRootKey      string `gorm:"type:varchar(191)"`
	EarningRiskFlag     string `gorm:"type:varchar(16)"`
	EarningRiskFrozen   bool
	EarningReviewStatus string `gorm:"type:varchar(24)"`
	EarningRiskReviewId int64
	CreatedAt           int64 `gorm:"autoCreateTime"`
	UpdatedAt           int64 `gorm:"autoUpdateTime;index"`
}

// BillingOperationMovement preserves every committed debit/credit stage even
// as the operation's recovery state advances. Its key deduplicates replay; the
// row is inserted in the same transaction as its wallet and token deltas.
type BillingOperationMovement struct {
	MovementKey  string `gorm:"type:varchar(64);primaryKey"`
	OperationKey string `gorm:"type:varchar(191);index;not null"`
	Stage        string `gorm:"type:varchar(32);not null"`
	UserId       int    `gorm:"index;not null"`
	TokenId      int
	WalletDelta  int
	TokenDelta   int
	CreatedAt    int64 `gorm:"autoCreateTime;index"`
}

func recordBillingMovementTx(tx *gorm.DB, op *BillingOperation, stage string, walletDelta, tokenDelta int) error {
	digest := sha256.Sum256([]byte(op.OperationKey + ":" + stage))
	return tx.Create(&BillingOperationMovement{MovementKey: fmt.Sprintf("%x", digest), OperationKey: op.OperationKey,
		Stage: stage, UserId: op.UserId, TokenId: op.TokenId, WalletDelta: walletDelta, TokenDelta: tokenDelta}).Error
}

// AdtySiteEarningEvent is committed with the applied wallet operation. Daily
// earnings and optional consume logs are projections, never recovery inputs.
type AdtySiteEarningEvent struct {
	OperationKey     string  `json:"operation_key" gorm:"type:varchar(191);primaryKey"`
	RootOperationKey string  `json:"root_operation_key" gorm:"type:varchar(191);index"`
	SiteId           int     `json:"site_id" gorm:"index;not null"`
	SiteOwnerUserId  int     `json:"site_owner_user_id" gorm:"not null;default:0"`
	UserId           int     `json:"user_id" gorm:"index;not null"`
	Quota            int     `json:"quota"`
	Markup           float64 `json:"markup"`
	CostFactor       float64 `json:"cost_factor"`
	RiskFlag         string  `json:"risk_flag" gorm:"type:varchar(16)"`
	RiskReleased     bool    `json:"risk_released"`
	ReviewStatus     string  `json:"review_status" gorm:"type:varchar(24);index"`
	Day              string  `json:"day" gorm:"type:varchar(16);index"`
	Projected        bool    `json:"projected" gorm:"index"`
	CreatedAt        int64   `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteEarningEvent) TableName() string { return "adty_site_earning_events" }

func GetBillingOperation(key string) (*BillingOperation, error) {
	var row BillingOperation
	err := DB.Where("operation_key = ?", key).First(&row).Error
	return &row, err
}

// BillingOperationRetryBackoff 是恢复链的退避阶梯：每一步都比上一步长一个
// 数量级，走完还在失败就进死信。15 秒是扫描周期本身，所以第一次失败只是
// 「下一轮再试」，之后才真正拉开距离。
var BillingOperationRetryBackoff = []time.Duration{
	15 * time.Second, time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour,
}

// BillingOperationMaxAttempts 是单条 op 的重试上限，含第一次尝试的失败。
const BillingOperationMaxAttempts = 7

// BillingOperationRetryDelay 返回第 attempts 次失败之后要等多久再试。
func BillingOperationRetryDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return 0
	}
	if attempts > len(BillingOperationRetryBackoff) {
		return BillingOperationRetryBackoff[len(BillingOperationRetryBackoff)-1]
	}
	return BillingOperationRetryBackoff[attempts-1]
}

// RecordBillingOperationRetryFailure 把一次失败的结算记到行上：attempts+1、
// last_error、next_retry_at，用完预算就转 BillingDead。返回 dead 表示这条 op
// 已经进死信，调用方应当把它报出来。
//
// 它必须单独提交：结算事务已经整笔回滚，这次记录是唯一留下来的证据。也因此
// 它用 UpdateColumns 而不是 Save——updated_at 由 autoUpdateTime 维护，这里想
// 让它继续只表示「最后一次真正动过财务状态」的时刻，排序交给 next_retry_at。
func RecordBillingOperationRetryFailure(key string, failure error, now int64) (bool, error) {
	if key == "" || failure == nil {
		return false, nil
	}
	dead := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row BillingOperation
		if err := lockForUpdate(tx).Where("operation_key = ?", key).First(&row).Error; err != nil {
			return err
		}
		attempts := row.Attempts + 1
		state, nextRetryAt := row.State, int64(0)
		if attempts >= BillingOperationMaxAttempts {
			state, dead = BillingDead, true
		} else {
			nextRetryAt = now + int64(BillingOperationRetryDelay(attempts)/time.Second)
		}
		return tx.Model(&BillingOperation{}).Where("operation_key = ?", key).UpdateColumns(map[string]any{
			"attempts": attempts, "last_error": truncateBillingOperationError(failure.Error()),
			"next_retry_at": nextRetryAt, "state": state,
		}).Error
	})
	if err != nil {
		return false, err
	}
	return dead, nil
}

// 一行的 last_error 只需要够定位问题，不该把整棵错误树塞进主库。
const billingOperationLastErrorLimit = 2000

// 按 rune 边界截断：错误里带中文时，切在多字节字符中间会让 PostgreSQL 直接
// 拒绝这条 UPDATE（invalid byte sequence），而这条 UPDATE 正是失败的唯一证据。
func truncateBillingOperationError(message string) string {
	if len(message) <= billingOperationLastErrorLimit {
		return message
	}
	cut := billingOperationLastErrorLimit
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return message[:cut]
}

// freezeBillingEarningRiskTx binds later adjustments to the original request's
// disposition. Review(false) permits new requests while preserving old skips.
func freezeBillingEarningRiskTx(tx *gorm.DB, row *BillingOperation) error {
	if row.SiteId <= 0 || row.EarningRiskFrozen {
		return nil
	}
	if row.EarningRootKey == "" {
		row.EarningRootKey = row.OperationKey
	}
	if row.EarningRootKey != row.OperationKey {
		var roots []BillingOperation
		if err := tx.Where("operation_key = ? AND user_id = ? AND site_id = ?", row.EarningRootKey, row.UserId, row.SiteId).Limit(1).Find(&roots).Error; err != nil {
			return err
		}
		if len(roots) > 0 && roots[0].EarningRiskFrozen {
			row.EarningRiskFlag, row.EarningRiskFrozen = roots[0].EarningRiskFlag, true
			row.EarningReviewStatus = roots[0].EarningReviewStatus
			row.EarningRiskReviewId = roots[0].EarningRiskReviewId
			return nil
		}
		// An event written before risk snapshots were added is still evidence
		// for this exact original operation, unlike a user's aggregate earnings.
		var original []AdtySiteEarningEvent
		if err := tx.Where("operation_key = ? AND user_id = ? AND site_id = ?", row.EarningRootKey, row.UserId, row.SiteId).Limit(1).Find(&original).Error; err != nil {
			return err
		}
		if len(original) > 0 {
			row.EarningRiskFlag, row.EarningRiskFrozen = original[0].RiskFlag, true
			row.EarningReviewStatus = original[0].ReviewStatus
			return nil
		}
	}
	if row.Kind == "task" || row.Kind == "mj_refund" {
		return freezeLegacyBillingEarningRiskTx(tx, row)
	}
	var customers []AdtySiteCustomer
	if err := lockForUpdate(tx).Where("user_id = ?", row.UserId).Limit(1).Find(&customers).Error; err != nil {
		return err
	}
	if len(customers) > 0 && !customers[0].Reviewed {
		row.EarningRiskFlag = customers[0].RiskFlag
	}
	// Record which review audits already existed when this request froze its
	// risk. An old review must not release a newly flagged request on a later visit.
	if row.EarningRiskFlag != "" {
		if err := tx.Model(&AdtySiteAudit{}).Where("site_id = ? AND action = ?", row.SiteId, "customer_review").
			Select("COALESCE(MAX(id),0)").Scan(&row.EarningRiskReviewId).Error; err != nil {
			return err
		}
	}
	row.EarningRiskFrozen = true
	return nil
}

// ReserveBillingOperation atomically reserves both wallet and token. A rejected
// request leaves neither a debit nor a pending job that could charge it later.
func ReserveBillingOperation(spec BillingOperation, target int) (*BillingOperation, error) {
	if spec.OperationKey == "" || spec.UserId <= 0 || target < 0 {
		return nil, ErrBillingOperationConflict
	}
	var result BillingOperation
	err := DB.Transaction(func(tx *gorm.DB) error {
		spec.State, spec.Kind = BillingReserved, "request"
		spec.SiteOwnerUserId = 0
		if err := freezeBillingSiteOwnerTx(tx, &spec, true); err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&spec).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("operation_key = ?", spec.OperationKey).First(&result).Error; err != nil {
			return err
		}
		if result.UserId != spec.UserId || result.TokenId != spec.TokenId || result.Kind != "request" || result.State != BillingReserved {
			return ErrBillingOperationConflict
		}
		snapshotWritten := false
		if result.SiteId != spec.SiteId || result.SiteMarkup != spec.SiteMarkup || result.SiteCostFactor != spec.SiteCostFactor {
			result.SiteId, result.SiteMarkup, result.SiteCostFactor = spec.SiteId, spec.SiteMarkup, spec.SiteCostFactor
			snapshotWritten = true
		}
		// The G.2-11 snapshot is immutable once written. Fill it only when the
		// reserved row was created without one.
		if result.PriceSnapshot == "" && spec.PriceSnapshot != "" {
			result.PriceSnapshot = spec.PriceSnapshot
			snapshotWritten = true
		}
		if snapshotWritten {
			if err := tx.Save(&result).Error; err != nil {
				return err
			}
		}
		if target <= result.ReservedQuota {
			if err := freezeBillingEarningRiskTx(tx, &result); err != nil {
				return err
			}
			return tx.Save(&result).Error
		}
		delta := target - result.ReservedQuota
		var user User
		if err := lockForUpdate(tx).First(&user, result.UserId).Error; err != nil {
			return err
		}
		balance, err := userWalletBalance(user.Quota, user.QuotaPending)
		if err != nil {
			return err
		}
		if balance < delta && (AdtyTeamReserveHook == nil || result.TokenId <= 0) {
			return ErrBillingWalletInsufficient
		}
		if AdtyTeamReserveHook != nil && result.TokenId > 0 {
			handled, hookErr := AdtyTeamReserveHook(tx, result.TokenId, result.UserId, delta, result.OperationKey)
			if hookErr != nil {
				return hookErr
			}
			if handled {
				result.ReservedQuota = target
				if err := freezeBillingEarningRiskTx(tx, &result); err != nil {
					return err
				}
				return tx.Save(&result).Error
			}
		}
		if balance < delta {
			return ErrBillingWalletInsufficient
		}
		if result.TokenId > 0 {
			var token Token
			if err := lockForUpdate(tx).First(&token, result.TokenId).Error; err != nil {
				return err
			}
			if token.UserId != result.UserId {
				return ErrBillingOperationConflict
			}
			remain, _, err := tokenQuotaBalance(&token)
			if err != nil {
				return err
			}
			if !result.TokenUnlimited && remain < delta {
				return ErrBillingTokenInsufficient
			}
			if err := applyTokenQuotaDeltaTx(tx, &token, -delta, common.BatchUpdateEnabled && common.RedisEnabled); err != nil {
				return err
			}
			result.TokenReservedQuota += delta
		}
		if err := applyUserQuotaDeltaTx(tx, &user, -delta, common.BatchUpdateEnabled && common.RedisEnabled); err != nil {
			return err
		}
		// 补扣同样要登记分摊：结算时按「预扣 + 补扣」总额扣批次，漏登记会让
		// 实际消耗大于已登记额度。传累计 target，函数内部只补差额。
		if err := AllocateAdtyFundAllocationsTx(tx, result.UserId, result.OperationKey, int64(target), int64(balance)); err != nil {
			return err
		}
		result.ReservedQuota = target
		if err := freezeBillingEarningRiskTx(tx, &result); err != nil {
			return err
		}
		tokenDelta := 0
		if result.TokenId > 0 {
			tokenDelta = -delta
		}
		if err := recordBillingMovementTx(tx, &result, fmt.Sprintf("reserve:%d", target), -delta, tokenDelta); err != nil {
			return err
		}
		return tx.Save(&result).Error
	})
	if err != nil {
		return nil, err
	}
	refreshBillingProjections(&result)
	return &result, nil
}

// QueueBillingSettlement commits the decision before any asynchronous work.
// A refund and a successful settlement cannot both win the same reservation.
func QueueBillingSettlement(key string, target int, refund bool, streamQuota ...int) error {
	walletStream, tokenStream := 0, 0
	if len(streamQuota) != 0 {
		if len(streamQuota) != 2 {
			return ErrBillingOperationConflict
		}
		walletStream, tokenStream = streamQuota[0], streamQuota[1]
	}
	if target < 0 || walletStream < 0 || tokenStream < 0 || walletStream > common.MaxWalletQuota || tokenStream > common.MaxWalletQuota || refund && (target != 0 || walletStream != 0 || tokenStream != 0) {
		return ErrBillingOperationConflict
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var row BillingOperation
		if err := lockForUpdate(tx).Where("operation_key = ?", key).First(&row).Error; err != nil {
			return err
		}
		state, final := BillingSettlePending, BillingSettled
		if refund {
			state, final = BillingRefundPending, BillingRefunded
		}
		if row.State != BillingReserved {
			if (row.State == state || row.State == final) && row.ActualQuota == target && row.StreamWalletQuota == walletStream && row.StreamTokenQuota == tokenStream {
				return nil
			}
			return ErrBillingOperationConflict
		}
		return tx.Model(&row).Updates(map[string]any{"actual_quota": target, "state": state, "stream_wallet_quota": walletStream, "stream_token_quota": tokenStream}).Error
	})
}

// QueueExpiredBillingRefund uses a conditional update against the heartbeat.
// An active request that renewed after the scan cannot be reclaimed.
func QueueExpiredBillingRefund(key string, cutoff int64) error {
	return DB.Model(&BillingOperation{}).Where("operation_key = ? AND state = ? AND updated_at < ? AND reference_id = 0", key, BillingReserved, cutoff).
		Updates(map[string]any{"state": BillingRefundPending, "actual_quota": 0}).Error
}

// QueueDirectBillingDelta records a stable request child operation for legacy
// direct charges and realtime chunks. Negative deltas represent a refund of
// the explicitly recorded base, never arithmetic overflow.
func QueueDirectBillingDelta(spec BillingOperation, delta int) error {
	spec.Kind, spec.State = "direct", BillingSettlePending
	if delta < 0 {
		if delta == -int(^uint(0)>>1)-1 {
			return ErrBillingOperationConflict
		}
		spec.ReservedQuota = -delta
		if spec.TokenId > 0 {
			spec.TokenReservedQuota = -delta
		}
	} else {
		spec.ActualQuota = delta
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := freezeBillingSiteOwnerTx(tx, &spec, true); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&spec).Error
	}); err != nil {
		return err
	}
	existing, err := GetBillingOperation(spec.OperationKey)
	if err != nil {
		return err
	}
	if existing.UserId != spec.UserId || existing.TokenId != spec.TokenId || existing.Kind != spec.Kind || existing.ActualQuota != spec.ActualQuota || existing.ReservedQuota != spec.ReservedQuota {
		return ErrBillingOperationConflict
	}
	return nil
}

// QueueTaskBillingOperation preserves the final target and normalized pricing
// snapshot independently of optional logs. The task row is re-read on apply.
func QueueTaskBillingOperation(task *Task, target int, refund bool) (string, error) {
	if task == nil || task.ID == 0 || target < 0 {
		return "", ErrBillingOperationConflict
	}
	stage := fmt.Sprintf("adjust:%d", target)
	if refund {
		stage = "refund"
	}
	row := BillingOperation{OperationKey: fmt.Sprintf("task:%d:%s", task.ID, stage), UserId: task.UserId,
		TokenId: task.PrivateData.TokenId, Kind: "task", ReferenceId: task.ID, ActualQuota: target, State: BillingSettlePending,
		EarningRootKey: task.PrivateData.BillingOperationKey}
	if refund {
		row.State = BillingRefundPending
	}
	if bc := task.PrivateData.BillingContext; bc != nil {
		row.SiteId, row.SiteMarkup, row.SiteCostFactor = bc.SiteId, bc.SiteMarkup, bc.SiteCostFactor
		row.SiteOwnerUserId = bc.SiteOwnerUserId
		if encoded, err := EncodeRequestPriceSnapshot(bc.PriceSnapshot); err != nil {
			return "", err
		} else if encoded != "" {
			row.PriceSnapshot = encoded
		}
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := freezeBillingSiteOwnerTx(tx, &row, false); err != nil {
			return err
		}
		if bc := task.PrivateData.BillingContext; bc != nil {
			snapshot := *bc
			snapshot.SiteOwnerUserId = row.SiteOwnerUserId
			encoded, err := common.Marshal(snapshot)
			if err != nil {
				return err
			}
			row.BillingContext = string(encoded)
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	})
	return row.OperationKey, err
}

func QueueMidjourneyBillingOperation(task *Midjourney, refund bool) (string, error) {
	if task == nil || task.Id == 0 {
		return "", ErrBillingOperationConflict
	}
	key, kind, state, target := task.BillingOperationKey, "mj_charge", BillingSettlePending, task.Quota
	if key == "" {
		key = fmt.Sprintf("mj:%d:charge", task.Id)
	}
	if refund {
		key, kind, state, target = fmt.Sprintf("mj:%d:refund", task.Id), "mj_refund", BillingRefundPending, 0
	}
	row := BillingOperation{OperationKey: key, UserId: task.UserId, TokenId: task.TokenId, Kind: kind, ReferenceId: int64(task.Id),
		ActualQuota: target, State: state, SiteId: task.BillingSiteId, SiteMarkup: task.BillingSiteMarkup, SiteCostFactor: task.BillingSiteCostFactor,
		EarningRootKey: task.BillingOperationKey, PriceSnapshot: task.BillingPriceSnapshot}
	row.SiteOwnerUserId = task.BillingSiteOwnerUserId
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := freezeBillingSiteOwnerTx(tx, &row, false); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	})
	return key, err
}

// ApplyBillingSettlement is safe after process death or an ambiguous commit.
// Wallet, token, task marker, operation outcome and earning event commit once.
func ApplyBillingSettlement(key string) (*BillingOperation, bool, error) {
	var row BillingOperation
	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("operation_key = ?", key).First(&row).Error; err != nil {
			return err
		}
		if row.State == BillingSettled || row.State == BillingRefunded || row.State == BillingCanceled {
			return nil
		}
		if row.State != BillingSettlePending && row.State != BillingRefundPending {
			return ErrBillingOperationConflict
		}
		base, tokenBase := row.ReservedQuota, row.TokenReservedQuota
		if row.Kind == "request" {
			if base > common.MaxWalletQuota-row.StreamWalletQuota || tokenBase > common.MaxWalletQuota-row.StreamTokenQuota {
				return ErrBillingOperationConflict
			}
			base += row.StreamWalletQuota
			tokenBase += row.StreamTokenQuota
		}
		var task Task
		var mj Midjourney
		if row.Kind == "task" || row.Kind == "request" && row.ReferenceId > 0 {
			if err := lockForUpdate(tx).First(&task, row.ReferenceId).Error; err != nil {
				return err
			}
			if task.UserId != row.UserId {
				return ErrBillingOperationConflict
			}
			if row.Kind == "task" {
				if task.BillingInitialPending {
					return ErrBillingOperationConflict
				}
				if row.State == BillingSettlePending {
					var refunds int64
					if err := tx.Model(&BillingOperation{}).Where("operation_key = ? AND state <> ?", fmt.Sprintf("task:%d:refund", task.ID), BillingCanceled).Count(&refunds).Error; err != nil {
						return err
					}
					if refunds > 0 {
						// A failure refund is final, including when an old success
						// adjustment was queued before it and resumes after restart.
						// The refund operation gives the reserved quota back, so this
						// operation consumed nothing: release its shares without
						// touching the batches.
						if err := ReleaseAdtyFundAllocationsTx(tx, row.UserId, row.OperationKey); err != nil {
							return err
						}
						row.State = BillingCanceled
						return tx.Save(&row).Error
					}
				}
				base, tokenBase = task.Quota, 0
				if row.TokenId > 0 {
					tokenBase = task.Quota
					if task.PrivateData.TokenConsumedQuota != nil {
						tokenBase = *task.PrivateData.TokenConsumedQuota
					}
				}
			}
		}
		if row.Kind == "mj_charge" || row.Kind == "mj_refund" {
			if err := lockForUpdate(tx).First(&mj, row.ReferenceId).Error; err != nil {
				return err
			}
			if mj.UserId != row.UserId {
				return ErrBillingOperationConflict
			}
			base, tokenBase = 0, 0
			if row.Kind == "mj_refund" && !mj.BillingChargePending {
				base = mj.Quota
				if row.TokenId > 0 {
					tokenBase = mj.Quota
					if mj.BillingTokenQuota > 0 {
						tokenBase = mj.BillingTokenQuota
					}
				}
			}
			if row.Kind == "mj_charge" && !mj.BillingChargePending {
				row.State = BillingCanceled
				return tx.Save(&row).Error
			}
		}
		var user User
		if err := lockForUpdate(tx).First(&user, row.UserId).Error; err != nil {
			return err
		}
		balance, err := userWalletBalance(user.Quota, user.QuotaPending)
		if err != nil {
			return err
		}
		delta := row.ActualQuota - base
		charged := delta
		if delta > 0 {
			charged = max(0, min(balance, delta))
		}
		if err := applyUserQuotaDeltaTx(tx, &user, -charged, common.BatchUpdateEnabled && common.RedisEnabled); err != nil {
			return err
		}
		// A.12.1/A.12.2.6：预扣阶段登记的分摊在这里转成实扣，同时把批次 remaining
		// 扣掉。两者同一事务，失败一起回滚，不会出现「余额变了批次没变」。
		// applied 是这次业务最终真正消耗的额度：预扣 base + 补扣 charged。
		//
		// 团队 Key 由团队共享余额付款，成员的个人本金批次不参与——不跳过的话，
		// 团队消费会吃掉员工自己的本金，那是「扣错人」而不是「多扣」。
		teamPaid, err := AdtyTokenIsTeamKeyTx(tx, row.TokenId)
		if err != nil {
			return err
		}
		var backedByBatches int64
		if !teamPaid {
			// 传的是「这次业务消耗之前」的钱包余额，不是此刻的余额：
			// 预扣的 base 已经在 PreConsume 阶段扣掉了，若把此刻余额（可能已是 0）
			// 传给「无批次时补建 legacy 批次」的分支，补出来的批次不够扣本次消耗，
			// 结算会整笔失败——老用户（升级前就有额度、从来没批次）就再也结不了账。
			// initial = 此刻余额 + base（预扣已扣） = balance + base。
			backed, err := SettleAdtyFundAllocationsTx(tx, row.UserId, row.OperationKey, int64(base+charged), int64(balance+base))
			if err != nil {
				return err
			}
			backedByBatches = backed
		}
		// 差额落库，不能只写日志：日志不可聚合，事后无法回答「这个用户漂了多少、
		// 从哪一笔开始漂」。团队 Key 由团队余额付款，个人批次不参与，gap 记 0。
		row.FundBatchGap = int64(base+charged) - backedByBatches
		row.ReservedQuota, row.TokenReservedQuota = base, tokenBase
		row.Shortfall, row.AppliedQuota = delta-charged, base+charged
		row.TokenAppliedQuota = 0
		tokenDelta := 0
		if row.TokenId > 0 {
			var tokens []Token
			if err := lockForUpdate(tx).Where("id = ?", row.TokenId).Limit(1).Find(&tokens).Error; err != nil {
				return err
			}
			// Deleted tokens cannot be used again. They must not prevent a wallet
			// refund; an error writing an existing token still rolls everything back.
			if len(tokens) > 0 {
				if tokens[0].UserId != row.UserId {
					return ErrBillingOperationConflict
				}
				if tokenBase != row.ActualQuota {
					if err := applyTokenQuotaDeltaTx(tx, &tokens[0], tokenBase-row.ActualQuota, common.BatchUpdateEnabled && common.RedisEnabled); err != nil {
						return err
					}
				}
				row.TokenAppliedQuota = row.ActualQuota
				tokenDelta = tokenBase - row.ActualQuota
			}
		}
		if err := recordBillingMovementTx(tx, &row, "apply", -charged, tokenDelta); err != nil {
			return err
		}
		if row.Kind == "task" || row.Kind == "request" && row.ReferenceId > 0 {
			if row.BillingContext != "" {
				var bc TaskBillingContext
				if err := common.UnmarshalJsonStr(row.BillingContext, &bc); err != nil {
					return err
				}
				task.PrivateData.BillingContext = &bc
			}
			task.PrivateData.TokenConsumedQuota = &row.TokenAppliedQuota
			if err := tx.Model(&task).Updates(map[string]any{"quota": row.AppliedQuota, "private_data": task.PrivateData, "billing_pending": false, "billing_initial_pending": false}).Error; err != nil {
				return err
			}
		}
		if row.Kind == "task" && delta != 0 {
			if err := tx.Model(&User{}).Where("id = ?", row.UserId).UpdateColumn("used_quota", gorm.Expr("used_quota + ?", delta)).Error; err != nil {
				return err
			}
			if task.ChannelId > 0 {
				if err := tx.Model(&Channel{}).Where("id = ?", task.ChannelId).UpdateColumn("used_quota", gorm.Expr("used_quota + ?", delta)).Error; err != nil {
					return err
				}
			}
		}
		if row.Kind == "mj_charge" || row.Kind == "mj_refund" {
			if err := tx.Model(&mj).Updates(map[string]any{"quota": row.AppliedQuota, "token_id": row.TokenId,
				"billing_token_quota": row.TokenAppliedQuota, "billing_charge_pending": false, "billing_pending": false}).Error; err != nil {
				return err
			}
			if row.Kind == "mj_refund" && base > 0 {
				if err := tx.Model(&User{}).Where("id = ?", row.UserId).UpdateColumn("used_quota", gorm.Expr("used_quota - ?", base)).Error; err != nil {
					return err
				}
				if channelId := mj.GetBillingChannelId(); channelId > 0 {
					if err := tx.Model(&Channel{}).Where("id = ?", channelId).UpdateColumn("used_quota", gorm.Expr("used_quota - ?", base)).Error; err != nil {
						return err
					}
				}
			}
		}
		stampConsumedAmountOnSnapshot(&row)
		earningQuota := charged
		if row.Kind == "request" {
			earningQuota = row.AppliedQuota - row.StreamWalletQuota
			if row.State == BillingRefundPending {
				earningQuota = 0
			}
		}
		if row.SiteId > 0 && BillingEarningFactorsUsable(row.SiteMarkup, row.SiteCostFactor) && earningQuota != 0 {
			if err := freezeBillingSiteOwnerTx(tx, &row, false); err != nil {
				return err
			}
			if err := freezeBillingEarningRiskTx(tx, &row); err != nil {
				return err
			}
			event := AdtySiteEarningEvent{OperationKey: row.OperationKey, SiteId: row.SiteId, SiteOwnerUserId: row.SiteOwnerUserId, UserId: row.UserId,
				Quota: earningQuota, Markup: row.SiteMarkup, CostFactor: row.SiteCostFactor, Day: time.Now().Format("2006-01-02"),
				RootOperationKey: row.EarningRootKey, RiskFlag: row.EarningRiskFlag, ReviewStatus: row.EarningReviewStatus}
			var customers []AdtySiteCustomer
			if err := lockForUpdate(tx).Where("user_id = ?", row.UserId).Limit(1).Find(&customers).Error; err != nil {
				return err
			}
			if len(customers) > 0 && customers[0].SiteId == row.SiteId {
				event.RiskReleased = customers[0].EarningRiskReleased
			}
			if err := applyBillingEarningReviewTx(tx, &row, &event); err != nil {
				return err
			}
			if err := tx.Create(&event).Error; err != nil {
				return err
			}
		}
		refunding := row.State == BillingRefundPending
		row.State = BillingSettled
		if refunding {
			row.State = BillingRefunded
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	refreshBillingProjections(&row)
	return &row, applied, nil
}

func refreshBillingProjections(row *BillingOperation) {
	if common.BatchUpdateEnabled && common.RedisEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, row.UserId, 0)
		if row.TokenId > 0 {
			addNewRecord(BatchUpdateTypeTokenQuota, row.TokenId, 0)
		}
	}
	if err := invalidateUserCache(row.UserId); err != nil {
		common.SysError(common.LogText("failed to invalidate committed wallet projection: user=%d error=%s", row.UserId, err.Error()))
	}
}

func stampConsumedAmountOnSnapshot(row *BillingOperation) {
	if row == nil {
		return
	}
	snap := DecodeRequestPriceSnapshot(row.PriceSnapshot)
	if snap == nil {
		return
	}
	stamped := snap.WithConsumedQuota(row.AppliedQuota)
	encoded, err := EncodeRequestPriceSnapshot(stamped)
	if err != nil || encoded == "" {
		return
	}
	row.PriceSnapshot = encoded
}

// RequestPriceSnapshot returns the frozen G.2-11 snapshot when the complete
// official field set is present. Old rows without it return nil; callers must
// keep previously stored amounts and must not reprice them from live config.
func (row *BillingOperation) RequestPriceSnapshot() *RequestPriceSnapshot {
	if row == nil {
		return nil
	}
	return DecodeRequestPriceSnapshot(row.PriceSnapshot)
}
