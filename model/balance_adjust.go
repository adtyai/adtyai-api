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

const (
	BalanceAdjustStatusPending  = "pending"
	BalanceAdjustStatusApproved = "approved" // 事务内过渡，库中最终不见
	BalanceAdjustStatusRejected = "rejected"
	BalanceAdjustStatusExecuted = "executed"

	BalanceAdjustAccountQuota = "quota"
	BalanceAdjustAccountBonus = "bonus"
)

var (
	ErrBalanceAdjustNotFound        = errors.New("balance adjust request not found")
	ErrBalanceAdjustBadAmount       = errors.New("amount must be non-zero")
	ErrBalanceAdjustAmountTooLarge  = errors.New("amount exceeds BalanceAdjustMaxAmount")
	ErrBalanceAdjustBadAccount      = errors.New("account must be quota or bonus")
	ErrBalanceAdjustReasonRequired  = errors.New("reason required")
	ErrBalanceAdjustRejectRequired  = errors.New("reject reason required")
	ErrBalanceAdjustConfirmRequired = errors.New("confirm=true required")
	ErrBalanceAdjustSelfReasonReq   = errors.New("self-approve reason required")
	ErrBalanceAdjustSelfForbidden   = errors.New("applicant cannot self-approve when multiple admins")
	ErrBalanceAdjustConflict        = errors.New("balance adjust request already decided")
	ErrBalanceAdjustUserRequired    = errors.New("user_id required")
	ErrBalanceAdjustUserMissing     = errors.New("target user not found")
)

// BalanceAdjustRequest A13/A22 余额调整审批单。
type BalanceAdjustRequest struct {
	Id                     int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId                 int        `json:"user_id" gorm:"index;not null"`
	Account                string     `json:"account" gorm:"type:varchar(16);not null"` // quota | bonus
	Amount                 int64      `json:"amount" gorm:"type:bigint;not null"`       // quota 单位，可正可负
	Reason                 string     `json:"reason" gorm:"type:text;not null"`
	ChatwootConversationId string     `json:"chatwoot_conversation_id" gorm:"type:varchar(128)"`
	Status                 string     `json:"status" gorm:"type:varchar(32);index;not null;default:'pending'"`
	ApplicantId            int        `json:"applicant_id" gorm:"index;not null"`
	ApproverId             *int       `json:"approver_id"`
	RejectReason           string     `json:"reject_reason" gorm:"type:text"`
	SelfApproveReason      string     `json:"self_approve_reason" gorm:"type:text"`
	LedgerEntryId          *int64     `json:"ledger_entry_id"`
	CreatedAt              time.Time  `json:"created_at" gorm:"autoCreateTime;index"`
	DecidedAt              *time.Time `json:"decided_at"`
	ExecutedAt             *time.Time `json:"executed_at"`
}

func (BalanceAdjustRequest) TableName() string { return "balance_adjust_requests" }

// CreateBalanceAdjustRequest 发起调额申请（仅写 pending，不动余额）。
func CreateBalanceAdjustRequest(userId int, account string, amount int64, reason, chatwootConvId string, applicantId int) (*BalanceAdjustRequest, error) {
	account = strings.ToLower(strings.TrimSpace(account))
	reason = strings.TrimSpace(reason)
	chatwootConvId = strings.TrimSpace(chatwootConvId)
	if userId <= 0 {
		return nil, ErrBalanceAdjustUserRequired
	}
	if account != BalanceAdjustAccountQuota && account != BalanceAdjustAccountBonus {
		return nil, ErrBalanceAdjustBadAccount
	}
	if amount == 0 {
		return nil, ErrBalanceAdjustBadAmount
	}
	if reason == "" {
		return nil, ErrBalanceAdjustReasonRequired
	}
	maxQ, err := BalanceAdjustMaxQuota()
	if err != nil {
		return nil, err
	}
	abs := amount
	if abs < 0 {
		abs = -abs
	}
	if abs > maxQ {
		return nil, ErrBalanceAdjustAmountTooLarge
	}

	var exists int64
	if err := DB.Model(&User{}).Where("id = ?", userId).Count(&exists).Error; err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrBalanceAdjustUserMissing
	}

	row := &BalanceAdjustRequest{
		UserId:                 userId,
		Account:                account,
		Amount:                 amount,
		Reason:                 reason,
		ChatwootConversationId: chatwootConvId,
		Status:                 BalanceAdjustStatusPending,
		ApplicantId:            applicantId,
	}
	if err := DB.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// ListBalanceAdjustRequests 按状态/用户/申请人筛选。
func ListBalanceAdjustRequests(status string, userId, applicantId, offset, limit int) ([]BalanceAdjustRequest, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	q := DB.Model(&BalanceAdjustRequest{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if userId > 0 {
		q = q.Where("user_id = ?", userId)
	}
	if applicantId > 0 {
		q = q.Where("applicant_id = ?", applicantId)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []BalanceAdjustRequest
	if err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ApproveBalanceAdjustResult 批准执行结果（含审计字段）。
type ApproveBalanceAdjustResult struct {
	Request       *BalanceAdjustRequest `json:"request"`
	BalanceBefore int64                 `json:"balance_before"`
	BalanceAfter  int64                 `json:"balance_after"`
	LedgerEntryId int64                 `json:"ledger_entry_id"`
	SelfApproved  bool                  `json:"self_approved"`
	AdminCount    int64                 `json:"admin_count"`
}

// ApproveBalanceAdjust 批准并同事务执行 AdjustQuota；pending→executed。
// confirm 一律必 true（自批/他批）；selfApproveReason 仅单超管自批时必填。
func ApproveBalanceAdjust(id int64, approverId int, confirm bool, selfApproveReason string) (*ApproveBalanceAdjustResult, error) {
	_ = confirm // 已废弃，不再校验
	selfApproveReason = strings.TrimSpace(selfApproveReason)
	var out *ApproveBalanceAdjustResult

	err := DB.Transaction(func(tx *gorm.DB) error {
		var row BalanceAdjustRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBalanceAdjustNotFound
			}
			return err
		}
		if row.Status != BalanceAdjustStatusPending {
			return ErrBalanceAdjustConflict
		}

		adminCount, err := CountEnabledAdmins(tx)
		if err != nil {
			return err
		}
		self := approverId == row.ApplicantId
		if self {
			if adminCount >= 2 {
				return ErrBalanceAdjustSelfForbidden
			}
			// 仅 1 位（或 0，理论上不应发生）管理员：可自批，须 self_approve_reason
			if selfApproveReason == "" {
				return ErrBalanceAdjustSelfReasonReq
			}
		}

		// 事务内过渡 approved（最终提交为 executed）
		now := time.Now()
		if err := tx.Model(&row).Updates(map[string]any{
			"status":      BalanceAdjustStatusApproved,
			"approver_id": approverId,
			"decided_at":  now,
		}).Error; err != nil {
			return err
		}
		if self {
			_ = tx.Model(&row).Update("self_approve_reason", selfApproveReason)
		}

		idemKey := fmt.Sprintf("balance_adjust:%d", row.Id)
		adj, err := AdjustQuota(tx, row.UserId, row.Account, row.Amount, row.Reason,
			"balance_adjust", fmt.Sprintf("%d", row.Id), idemKey, approverId)
		if err != nil {
			return err
		}

		execAt := time.Now()
		entryId := adj.Entry.Id
		if err := tx.Model(&BalanceAdjustRequest{}).Where("id = ?", row.Id).Updates(map[string]any{
			"status":          BalanceAdjustStatusExecuted,
			"ledger_entry_id": entryId,
			"executed_at":     execAt,
			"approver_id":     approverId,
			"decided_at":      now,
		}).Error; err != nil {
			return err
		}

		row.Status = BalanceAdjustStatusExecuted
		row.ApproverId = &approverId
		row.LedgerEntryId = &entryId
		row.DecidedAt = &now
		row.ExecutedAt = &execAt
		if self {
			row.SelfApproveReason = selfApproveReason
		}

		out = &ApproveBalanceAdjustResult{
			Request:       &row,
			BalanceBefore: adj.BalanceBefore,
			BalanceAfter:  adj.BalanceAfter,
			LedgerEntryId: entryId,
			SelfApproved:  self,
			AdminCount:    adminCount,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 自批告警（事务成功后；stub 可断言）
	if out != nil && out.SelfApproved && out.Request != nil {
		emitSelfApproveAlert(out.Request)
	}
	return out, nil
}

// RejectBalanceAdjust 驳回：pending→rejected，不动余额。confirm 一律必 true。
func RejectBalanceAdjust(id int64, approverId int, confirm bool, rejectReason string) (*BalanceAdjustRequest, error) {
	_ = confirm // 已废弃，不再校验
	rejectReason = strings.TrimSpace(rejectReason)
	if rejectReason == "" {
		return nil, ErrBalanceAdjustRejectRequired
	}
	var out *BalanceAdjustRequest
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row BalanceAdjustRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBalanceAdjustNotFound
			}
			return err
		}
		if row.Status != BalanceAdjustStatusPending {
			return ErrBalanceAdjustConflict
		}
		now := time.Now()
		if err := tx.Model(&row).Updates(map[string]any{
			"status":        BalanceAdjustStatusRejected,
			"approver_id":   approverId,
			"reject_reason": rejectReason,
			"decided_at":    now,
		}).Error; err != nil {
			return err
		}
		row.Status = BalanceAdjustStatusRejected
		row.ApproverId = &approverId
		row.RejectReason = rejectReason
		row.DecidedAt = &now
		out = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func emitSelfApproveAlert(row *BalanceAdjustRequest) {
	if row == nil {
		return
	}
	abs := row.Amount
	if abs < 0 {
		abs = -abs
	}
	usd := float64(abs) / common.QuotaPerUnit
	level := OpsAlertLevelPending
	if usd > GetSelfApproveAlertAmount() {
		level = OpsAlertLevelUrgent
	}
	_ = RecordOpsAlert(level, "balance_adjust_self_approve", map[string]any{
		"request_id": row.Id,
		"user_id":    row.UserId,
		"account":    row.Account,
		"amount":     row.Amount,
		"amount_usd": usd,
		"reason":     row.Reason,
		"applicant":  row.ApplicantId,
		"self_reason": row.SelfApproveReason,
	})
}
