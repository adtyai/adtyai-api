package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	AdtyEarningNeedsReview      = "needs_review"
	AdtyEarningOriginalReleased = "released"
	AdtyEarningOriginalSkipped  = "skipped"
	AdtyEarningReviewHoldReason = "legacy task earnings require finance review"
)

// A historical task without its original operation has no reliable earning
// disposition. A synthetic root records only that disposition, never money.
// The customer lock serializes root creation and finance decisions; later task
// adjustments use the same root even after customer transfer resets risk.
func freezeLegacyBillingEarningRiskTx(tx *gorm.DB, row *BillingOperation) error {
	var customers []AdtySiteCustomer
	if err := lockForUpdate(tx).Where("user_id = ?", row.UserId).Limit(1).Find(&customers).Error; err != nil {
		return err
	}
	family := "task"
	if row.Kind == "mj_refund" {
		family = "mj"
	}
	key := fmt.Sprintf("legacy:%s:%d", family, row.ReferenceId)
	var roots []BillingOperation
	if err := tx.Where("operation_key = ?", key).Limit(1).Find(&roots).Error; err != nil {
		return err
	}
	if len(roots) == 0 {
		root := BillingOperation{OperationKey: key, UserId: row.UserId, SiteId: row.SiteId,
			Kind: "earning_root", ReferenceId: row.ReferenceId, State: BillingSettled,
			EarningRootKey: key, EarningRiskFrozen: true, EarningReviewStatus: AdtyEarningNeedsReview}
		if len(customers) > 0 && customers[0].SiteId == row.SiteId {
			root.EarningRiskFlag = customers[0].RiskFlag
		}
		// An explicit historical conversion for this customer and original site
		// proves release. Other customers' reviews and aggregate positive rows do not.
		var chargeAt int64
		if row.Kind == "task" {
			var original Task
			if err := tx.Select("id", "created_at", "submit_time").First(&original, row.ReferenceId).Error; err != nil {
				return err
			}
			chargeAt = original.SubmitTime
			if chargeAt == 0 {
				chargeAt = original.CreatedAt
			}
		} else {
			var original Midjourney
			if err := tx.Select("id", "submit_time").First(&original, row.ReferenceId).Error; err != nil {
				return err
			}
			chargeAt = original.SubmitTime
			if chargeAt > 1e12 { // historical MJ submit times are sometimes milliseconds
				chargeAt /= 1000
			}
		}
		if chargeAt > 0 {
			flag, released, err := releasedBillingEarningAuditTx(tx, row.SiteId, row.UserId, 0, chargeAt)
			if err != nil {
				return err
			}
			if released {
				root.EarningRiskFlag, root.EarningReviewStatus = flag, AdtyEarningOriginalReleased
			}
		}
		if err := tx.Create(&root).Error; err != nil {
			return err
		}
		roots = append(roots, root)
	}
	if roots[0].UserId != row.UserId || roots[0].SiteId != row.SiteId || roots[0].Kind != "earning_root" {
		return ErrBillingOperationConflict
	}
	row.EarningRootKey, row.EarningRiskFlag = key, roots[0].EarningRiskFlag
	row.EarningReviewStatus, row.EarningRiskFrozen = roots[0].EarningReviewStatus, true
	return nil
}

// Called while holding the customer lock, after wallet/token mutation. Finance
// can resolve a synthetic root while an adjustment was waiting for that lock,
// so its current decision is re-read before the event is committed.
func applyBillingEarningReviewTx(tx *gorm.DB, row *BillingOperation, event *AdtySiteEarningEvent) error {
	if row.EarningReviewStatus != "" {
		var root BillingOperation
		if err := lockForUpdate(tx).Where("operation_key = ?", row.EarningRootKey).First(&root).Error; err != nil {
			return err
		}
		row.EarningReviewStatus, event.ReviewStatus = root.EarningReviewStatus, root.EarningReviewStatus
		event.RiskReleased = event.ReviewStatus == AdtyEarningOriginalReleased
	} else if event.RiskFlag != "" && !event.RiskReleased {
		// Review(true) can precede a customer transfer. The immutable root's
		// released events retain that proof after the current customer is reset.
		var released int64
		if err := tx.Model(&AdtySiteEarningEvent{}).Where("root_operation_key = ? AND risk_released = ?", row.EarningRootKey, true).Count(&released).Error; err != nil {
			return err
		}
		event.RiskReleased = released > 0
		if !event.RiskReleased {
			_, approved, err := releasedBillingEarningAuditTx(tx, row.SiteId, row.UserId, row.EarningRiskReviewId, 0)
			if err != nil {
				return err
			}
			event.RiskReleased = approved
		}
	}
	if event.ReviewStatus == AdtyEarningNeedsReview {
		return tx.Model(&AdtySite{}).Where("id = ? AND (settle_hold_reason = ? OR settle_hold_reason IS NULL)", row.SiteId, "").
			Update("settle_hold_reason", AdtyEarningReviewHoldReason).Error
	}
	return nil
}

func releasedBillingEarningAuditTx(tx *gorm.DB, siteId, userId int, afterId, chargeAt int64) (string, bool, error) {
	var audits []AdtySiteAudit
	if err := tx.Where("site_id = ? AND action = ? AND id > ? AND created_at >= ?", siteId, "customer_review", afterId, chargeAt).Order("id").Find(&audits).Error; err != nil {
		return "", false, err
	}
	for _, audit := range audits {
		var old struct {
			UserId   int    `json:"user_id"`
			RiskFlag string `json:"risk_flag"`
		}
		var updated struct {
			ConvertSkipped bool `json:"convert_skipped"`
		}
		if common.UnmarshalJsonStr(audit.OldValue, &old) == nil && old.UserId == userId &&
			common.UnmarshalJsonStr(audit.NewValue, &updated) == nil && updated.ConvertSkipped {
			return old.RiskFlag, true, nil
		}
	}
	return "", false, nil
}
