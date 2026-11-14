package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ErrAdtyFundAllocationMissing is returned when a settlement or release cannot
// find the pre-charge share it is supposed to act on.
var ErrAdtyFundAllocationMissing = errors.New("adty fund allocation is missing")

// AdtyFundAllocation is the per-request share of one fund batch. A.12.1 requires
// every batch to keep its 冻结/预扣 relation, and A.12.2.6 requires cancellations,
// pre-charge releases and refunds to return funds "按原分摊" instead of a
// merged balance. One batch can back many requests, so the operation identity is
// part of the key: without it a release cannot tell which share to give back.
//
// Capacity authority stays with adty_fund_batches.remaining: this step only
// records which batch each pre-charge draws from, following the A.12.2 order
// (principal first, then bonus, FIFO inside each kind). Settlement is what turns
// a share into real consumption, and that step is what starts subtracting from
// remaining. Because of that, shares are never subtracted from capacity here —
// doing so before settlement exists would make outstanding shares eat a user's
// usable quota forever.
type AdtyFundAllocation struct {
	Id          int64  `json:"id" gorm:"primaryKey"`
	UserId      int    `json:"user_id" gorm:"index;not null"`
	BatchId     int64  `json:"batch_id" gorm:"not null;uniqueIndex:ux_adty_fund_allocation_share,priority:1"`
	OperationId string `json:"operation_id" gorm:"type:varchar(191);not null;uniqueIndex:ux_adty_fund_allocation_share,priority:2;index"`
	Seq         int    `json:"seq" gorm:"not null;uniqueIndex:ux_adty_fund_allocation_share,priority:3"`
	Kind        string `json:"kind" gorm:"type:varchar(16)"` // mirrors the batch kind at allocation time
	Source      string `json:"source" gorm:"type:varchar(64)"`
	Held        int64  `json:"held" gorm:"not null;default:0"`    // pre-charged, not yet settled
	Settled     int64  `json:"settled" gorm:"not null;default:0"` // turned into real consumption
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyFundAllocation) TableName() string { return "adty_fund_allocations" }

// AllocateAdtyFundAllocationsTx records the pre-charge shares for one operation.
// It spends principal before bonus and follows the batch FIFO order used by
// ConsumeAdtyFundBatchesTx, so a later settlement can hand back exactly the
// shares this operation took.
//
// A user without lots gets no shares: those accounts still settle through the
// legacy path in ConsumeAdtyFundBatchesTx, which invents a lot on first spend.
func AllocateAdtyFundAllocationsTx(tx *gorm.DB, userID int, operationID string, amount int64) error {
	if tx == nil || userID <= 0 || operationID == "" || amount <= 0 {
		return nil
	}
	if err := adtyRequireFundBatchTable(tx); err != nil {
		return err
	}
	var existing int64
	if err := tx.Model(&AdtyFundAllocation{}).Where("operation_id = ? AND user_id = ?", operationID, userID).Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		// The operation already took its shares; re-reserving an operation must
		// not double-claim the batch.
		return nil
	}
	var batches []AdtyFundBatch
	if err := tx.Where("user_id = ?", userID).Find(&batches).Error; err != nil {
		return err
	}
	if len(batches) == 0 {
		return nil
	}
	pickAdtyFundBatchOrder(batches)
	remaining := amount
	seq := 0
	for _, batch := range batches {
		if remaining <= 0 {
			break
		}
		// Principal lots are spent before bonus lots (A.12.2 step 1 and 4).
		if batch.Kind != "principal" {
			continue
		}
		available := batch.Remaining
		if available <= 0 {
			continue
		}
		take := min(available, remaining)
		seq++
		if err := tx.Create(&AdtyFundAllocation{
			UserId:      userID,
			BatchId:     batch.Id,
			OperationId: operationID,
			Seq:         seq,
			Kind:        batch.Kind,
			Source:      batch.Source,
			Held:        take,
		}).Error; err != nil {
			if isAdtyUniqueViolation(err) {
				return ErrAdtyFundAllocationMissing
			}
			return err
		}
		remaining -= take
	}
	for _, batch := range batches {
		if remaining <= 0 {
			break
		}
		if batch.Kind != "bonus" {
			continue
		}
		available := batch.Remaining
		if available <= 0 {
			continue
		}
		take := min(available, remaining)
		seq++
		if err := tx.Create(&AdtyFundAllocation{
			UserId:      userID,
			BatchId:     batch.Id,
			OperationId: operationID,
			Seq:         seq,
			Kind:        batch.Kind,
			Source:      batch.Source,
			Held:        take,
		}).Error; err != nil {
			if isAdtyUniqueViolation(err) {
				return ErrAdtyFundAllocationMissing
			}
			return err
		}
		remaining -= take
	}
	if remaining > 0 {
		// The batches cannot cover this reservation. That is not fatal on the
		// pre-charge path (balances can still carry legacy quota), but the
		// mismatch must be visible instead of silently losing the trail.
		common.SysError(common.LogText("adty fund allocation short by %d quota for user=%d operation=%s", remaining, userID, operationID))
	}
	return nil
}

// SumAdtyFundAllocationHeld returns the quota a user still has held in
// pre-charge shares, split by batch kind. It exists for the reconciliation check
// that compares share totals against the live balance.
func SumAdtyFundAllocationHeld(userID int) (principal, bonus int64, err error) {
	if DB == nil || userID <= 0 {
		return 0, 0, nil
	}
	if err := adtyRequireFundBatchTable(DB); err != nil {
		return 0, 0, err
	}
	var rows []struct {
		Kind string
		Held int64
	}
	if err := DB.Model(&AdtyFundAllocation{}).
		Select("kind, coalesce(sum(held - settled),0) as held").
		Where("user_id = ?", userID).
		Group("kind").
		Scan(&rows).Error; err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		if row.Kind == "principal" {
			principal = row.Held
		} else if row.Kind == "bonus" {
			bonus = row.Held
		}
	}
	return principal, bonus, nil
}

// pickAdtyFundBatchOrder sorts lots the way A.12.2 consumes them: principal by
// creation time then id, then bonus by creation time then id.
func pickAdtyFundBatchOrder(rows []AdtyFundBatch) {
	for i := 1; i < len(rows); i++ {
		item := rows[i]
		j := i
		for j > 0 && adtyFundKindRank(rows[j-1]) > adtyFundKindRank(item) {
			rows[j] = rows[j-1]
			j--
		}
		for j > 0 && adtyFundKindRank(rows[j-1]) == adtyFundKindRank(item) && !adtyFundBatchLess(rows[j-1], item) {
			rows[j] = rows[j-1]
			j--
		}
		rows[j] = item
	}
}

func adtyFundKindRank(row AdtyFundBatch) int {
	if row.Kind == "principal" {
		return 0
	}
	return 1
}
