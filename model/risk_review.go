package model

import (
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	RiskReviewKindFirstTopupExtra = "first_topup_extra"
	RiskReviewStatusPending       = "pending"
	RiskReviewStatusApproved      = "approved"
	RiskReviewStatusRejected      = "rejected"
)

var (
	ErrRiskReviewNotFound      = errors.New("risk review not found")
	ErrRiskReviewStatusInvalid = errors.New("risk review status invalid")
	ErrRiskReviewOrderRefunded = errors.New("risk_review_order_refunded")
)

// RiskReviewOrderRefundedError 订单已全额退款（或剩余实付为 0）时拒绝审批入账。
type RiskReviewOrderRefundedError struct {
	OrderNo      string
	RefundStatus string
}

func (e *RiskReviewOrderRefundedError) Error() string { return ErrRiskReviewOrderRefunded.Error() }
func (e *RiskReviewOrderRefundedError) Unwrap() error { return ErrRiskReviewOrderRefunded }

// RiskReview 风控人工待审（本块：首充多送 pending → 放行入赠送账 / 驳回）。
type RiskReview struct {
	Id         int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	Kind       string     `json:"kind" gorm:"type:varchar(64);index;not null"`
	UserId     int        `json:"user_id" gorm:"index;not null"`
	OrderNo    string     `json:"order_no" gorm:"type:varchar(64);index"`
	AmountUSD  float64    `json:"amount_usd" gorm:"type:decimal(18,6);not null;default:0"`
	Status     string     `json:"status" gorm:"type:varchar(32);index;not null;default:'pending'"`
	Reason     string     `json:"reason" gorm:"type:varchar(128)"`
	Payload    string     `json:"payload" gorm:"type:text"`
	ResolvedBy int        `json:"resolved_by" gorm:"default:0"`
	CreatedAt  time.Time  `json:"created_at" gorm:"autoCreateTime"`
	ResolvedAt *time.Time `json:"resolved_at"`
}

func (RiskReview) TableName() string { return "risk_reviews" }

func InsertRiskReviewTx(tx *gorm.DB, kind string, userId int, orderNo string, amountUSD float64, reason string) error {
	row := RiskReview{
		Kind:      kind,
		UserId:    userId,
		OrderNo:   orderNo,
		AmountUSD: amountUSD,
		Status:    RiskReviewStatusPending,
		Reason:    reason,
	}
	return tx.Create(&row).Error
}

// GetRiskReviewById 按主键取待审单。
func GetRiskReviewById(id int64) (*RiskReview, error) {
	var row RiskReview
	if err := DB.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRiskReviewNotFound
		}
		return nil, err
	}
	return &row, nil
}

// ListRiskReviews 最小列表；status 空则不过滤。
func ListRiskReviews(status string, limit, offset int) ([]RiskReview, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := DB.Model(&RiskReview{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []RiskReview
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ApproveRiskReview 放行首充多送：同一事务经 AdjustQuota 写入 bonus + ledger（kind=topup_first_bonus），
// 更新 risk_review、first_topup_claims，并把入账额度累加进订单 bonus_credited_quota（退款按快照比例收回）。
// 订单已 full 退款（或剩余实付为 0）时拒绝，返回 risk_review_order_refunded；partial 按剩余实付比例入账。
// 锁序：risk_review → 订单行（若有）→ claim 行（若有）→（AdjustQuota 内）user_bonus_wallets → users。
// 幂等：已 approved 再调不重复加钱。
func ApproveRiskReview(id int64, operatorId int) (*RiskReview, error) {
	if id <= 0 {
		return nil, ErrRiskReviewNotFound
	}
	var out *RiskReview
	err := DB.Transaction(func(tx *gorm.DB) error {
		var rr RiskReview
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&rr).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRiskReviewNotFound
			}
			return err
		}
		if rr.Status == RiskReviewStatusApproved {
			out = &rr
			return nil // 幂等
		}
		if rr.Status != RiskReviewStatusPending {
			return ErrRiskReviewStatusInvalid
		}
		if rr.Kind != RiskReviewKindFirstTopupExtra {
			return ErrRiskReviewStatusInvalid
		}

		// 锁相关订单行（有则锁）；全额退款拒绝审批；部分退按剩余实付比例入账，并写入订单赠送快照。
		var order TopupOrder
		orderFound := false
		if rr.OrderNo != "" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("order_no = ?", rr.OrderNo).First(&order).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			} else {
				orderFound = true
			}
		}
		if orderFound {
			if order.RefundStatus == RefundStatusFull {
				return &RiskReviewOrderRefundedError{OrderNo: order.OrderNo, RefundStatus: order.RefundStatus}
			}
			paidUSD := order.CreditAmountUSD
			if paidUSD <= 0 {
				paidUSD = order.PaidAmountUSD
			}
			remainUSD := paidUSD - order.RefundedAmount
			if paidUSD > 0 && remainUSD <= 1e-9 {
				return &RiskReviewOrderRefundedError{OrderNo: order.OrderNo, RefundStatus: order.RefundStatus}
			}
		}

		// 锁 claim 行（按订单或用户）
		var claim FirstTopupClaim
		claimFound := false
		if rr.OrderNo != "" {
			cerr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("order_no = ?", rr.OrderNo).First(&claim).Error
			if cerr == nil {
				claimFound = true
			} else if !errors.Is(cerr, gorm.ErrRecordNotFound) {
				return cerr
			}
		}
		if !claimFound {
			cerr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND status = ?", rr.UserId, FirstTopupStatusPendingReview).First(&claim).Error
			if cerr == nil {
				claimFound = true
			} else if !errors.Is(cerr, gorm.ErrRecordNotFound) {
				return cerr
			}
		}

		bonusQuota, err := DollarsToQuota(rr.AmountUSD)
		if err != nil {
			return err
		}
		creditBonusQuota := int64(bonusQuota)
		if orderFound && creditBonusQuota > 0 && order.RefundStatus == RefundStatusPartial {
			paidUSD := order.CreditAmountUSD
			if paidUSD <= 0 {
				paidUSD = order.PaidAmountUSD
			}
			if paidUSD > 0 {
				remainUSD := paidUSD - order.RefundedAmount
				if remainUSD < 0 {
					remainUSD = 0
				}
				scaled, serr := DollarsToQuota(rr.AmountUSD * (remainUSD / paidUSD))
				if serr != nil {
					return serr
				}
				creditBonusQuota = int64(scaled)
			}
		}

		// 入账经 AdjustQuota（A5）：不直接改 bonus / 手写 ledger；锁序由 AdjustQuota 保证（bonus→users）。
		if creditBonusQuota > 0 {
			refType := "risk_review"
			refId := strconv.FormatInt(rr.Id, 10)
			if rr.OrderNo != "" {
				refType = "topup_order"
				refId = rr.OrderNo
			}
			if _, err := AdjustQuota(tx, rr.UserId, "bonus", creditBonusQuota,
				"adtyai first topup extra approved", refType, refId, "", operatorId, "topup_first_bonus"); err != nil {
				return err
			}
			// 写入订单赠送快照，退款按 BonusCreditedQuota 比例收回。
			if orderFound {
				if err := tx.Model(&TopupOrder{}).Where("id = ?", order.Id).
					Update("bonus_credited_quota", gorm.Expr("bonus_credited_quota + ?", creditBonusQuota)).Error; err != nil {
					return err
				}
			}
		}

		if claimFound && claim.Status == FirstTopupStatusPendingReview {
			if err := tx.Model(&FirstTopupClaim{}).Where("id = ?", claim.Id).
				Update("status", FirstTopupStatusClaimed).Error; err != nil {
				return err
			}
		}

		now := time.Now()
		rr.Status = RiskReviewStatusApproved
		rr.ResolvedBy = operatorId
		rr.ResolvedAt = &now
		if err := tx.Save(&rr).Error; err != nil {
			return err
		}
		out = &rr
		_ = invalidateUserCache(rr.UserId)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RejectRiskReview 驳回：不入账，更新状态与 claim；已 rejected 幂等。
func RejectRiskReview(id int64, operatorId int) (*RiskReview, error) {
	if id <= 0 {
		return nil, ErrRiskReviewNotFound
	}
	var out *RiskReview
	err := DB.Transaction(func(tx *gorm.DB) error {
		var rr RiskReview
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&rr).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRiskReviewNotFound
			}
			return err
		}
		if rr.Status == RiskReviewStatusRejected {
			out = &rr
			return nil // 幂等
		}
		if rr.Status != RiskReviewStatusPending {
			return ErrRiskReviewStatusInvalid
		}

		if rr.OrderNo != "" {
			var order TopupOrder
			_ = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("order_no = ?", rr.OrderNo).First(&order).Error
		}

		var claim FirstTopupClaim
		claimFound := false
		if rr.OrderNo != "" {
			cerr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("order_no = ?", rr.OrderNo).First(&claim).Error
			if cerr == nil {
				claimFound = true
			} else if !errors.Is(cerr, gorm.ErrRecordNotFound) {
				return cerr
			}
		}
		if !claimFound {
			cerr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND status = ?", rr.UserId, FirstTopupStatusPendingReview).First(&claim).Error
			if cerr == nil {
				claimFound = true
			} else if !errors.Is(cerr, gorm.ErrRecordNotFound) {
				return cerr
			}
		}

		if claimFound && claim.Status == FirstTopupStatusPendingReview {
			if err := tx.Model(&FirstTopupClaim{}).Where("id = ?", claim.Id).
				Update("status", FirstTopupStatusRejected).Error; err != nil {
				return err
			}
		}

		now := time.Now()
		rr.Status = RiskReviewStatusRejected
		rr.ResolvedBy = operatorId
		rr.ResolvedAt = &now
		if err := tx.Save(&rr).Error; err != nil {
			return err
		}
		out = &rr
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListRiskReviewsByKind 按 kind+status 列表。
func ListRiskReviewsByKind(kind, status string, limit, offset int) ([]RiskReview, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := DB.Model(&RiskReview{})
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []RiskReview
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
