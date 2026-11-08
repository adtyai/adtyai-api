package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"time"

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
)

var (
	ErrBillingOperationConflict  = errors.New("billing operation conflicts with its durable intent")
	ErrBillingWalletInsufficient = errors.New("wallet quota insufficient")
	ErrBillingTokenInsufficient  = errors.New("token quota insufficient")
)

// BillingOperation is the main-database identity and recovery record for a
// reservation and its final disposition. It contains no credential or payload.
// Reserved/applied wallet amounts are distinct from token charges and shortfall.
type BillingOperation struct {
	OperationKey        string `gorm:"type:varchar(191);primaryKey"`
	UserId              int    `gorm:"index;not null"`
	TokenId             int
	TokenUnlimited      bool
	LeaseOwner          string `gorm:"type:varchar(64);index"`
	Kind                string `gorm:"type:varchar(24);not null"` // request, task, mj_charge, mj_refund
	ReferenceId         int64  `gorm:"index"`
	ReservedQuota       int
	TokenReservedQuota  int
	StreamWalletQuota   int // independently committed realtime chunks included in the final target
	StreamTokenQuota    int
	ActualQuota         int
	AppliedQuota        int
	TokenAppliedQuota   int
	Shortfall           int
	State               string `gorm:"type:varchar(24);index;not null"`
	SiteId              int
	SiteMarkup          float64
	SiteCostFactor      float64
	BillingContext      string `gorm:"type:text"`
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
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&spec).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("operation_key = ?", spec.OperationKey).First(&result).Error; err != nil {
			return err
		}
		if result.UserId != spec.UserId || result.TokenId != spec.TokenId || result.Kind != "request" || result.State != BillingReserved {
			return ErrBillingOperationConflict
		}
		if result.SiteId != spec.SiteId || result.SiteMarkup != spec.SiteMarkup || result.SiteCostFactor != spec.SiteCostFactor {
			result.SiteId, result.SiteMarkup, result.SiteCostFactor = spec.SiteId, spec.SiteMarkup, spec.SiteCostFactor
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
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&spec).Error; err != nil {
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
		encoded, err := common.Marshal(bc)
		if err != nil {
			return "", err
		}
		row.BillingContext = string(encoded)
	}
	err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
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
		EarningRootKey: task.BillingOperationKey}
	err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
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
		earningQuota := charged
		if row.Kind == "request" {
			earningQuota = row.AppliedQuota - row.StreamWalletQuota
			if row.State == BillingRefundPending {
				earningQuota = 0
			}
		}
		if row.SiteId > 0 && row.SiteMarkup >= 1 && !math.IsInf(row.SiteMarkup, 0) && row.SiteCostFactor > 0 && row.SiteCostFactor <= 1 && earningQuota != 0 {
			if err := freezeBillingEarningRiskTx(tx, &row); err != nil {
				return err
			}
			event := AdtySiteEarningEvent{OperationKey: row.OperationKey, SiteId: row.SiteId, UserId: row.UserId,
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
