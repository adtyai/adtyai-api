package service

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func writeAdtyOfficialAudit(actorId, siteId int, objectType, objectID, action, reason, approvalRef string, before, after any, result string) {
	objectType = strings.TrimSpace(objectType)
	objectID = strings.TrimSpace(objectID)
	action = strings.TrimSpace(action)
	if objectType == "" || objectID == "" || action == "" || strings.TrimSpace(result) == "" {
		return
	}
	rec := model.OfficialAuditWrite{
		UserId:      actorId,
		SiteID:      model.OfficialAuditSiteIDPointer(siteId),
		ObjectType:  objectType,
		ObjectID:    objectID,
		Action:      action,
		Category:    model.AuditCategoryOperation,
		Content:     action,
		Before:      before,
		After:       after,
		Reason:      reason,
		ApprovalRef: approvalRef,
		Result:      result,
		Success:     result == model.AuditResultSuccess,
	}
	if actorId > 0 {
		if user, err := model.GetUserById(actorId, false); err == nil && user != nil {
			rec.Username = user.Username
			rec.ActorRole = user.Role
		}
	}
	if err := model.WriteOfficialAuditLog(nil, rec); err != nil {
		common.SysLog(common.LogText("official %s audit write failed: %s", action, err.Error()))
	}
}

func adtyWalletIdempotencyExists(tx *gorm.DB, keys ...string) (bool, error) {
	if tx == nil {
		return false, nil
	}
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			filtered = append(filtered, key)
		}
	}
	if len(filtered) == 0 {
		return false, nil
	}
	var n int64
	if err := tx.Model(&model.AdtySiteWalletLedger{}).Where("idempotency_key IN ?", filtered).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func writeAdtyGiftOfficialAudit(actorId, siteId int, tradeNo, action, reason string, before, after any) {
	writeAdtyOfficialAudit(actorId, siteId, "gift", tradeNo, action, reason, "", before, after, model.AuditResultSuccess)
}

func writeAdtyMarginWalletOfficialAudit(ledger *model.AdtySiteWalletLedger, change AdtySiteWalletChange, frozenBefore, frozenAfter, cycleUsed, cycleTier decimal.Decimal) {
	if ledger == nil || ledger.WalletKind != model.AdtySiteWalletMargin {
		return
	}
	switch change.Type {
	case "gift_reserve", "gift_release", "gift_spend", "voucher_margin":
		// Parent gift or voucher operations write the official record.
		return
	}
	writeAdtyOfficialAudit(change.OperatorID, ledger.SiteId, "margin", strconv.Itoa(ledger.SiteId), "adty.margin."+change.Type, change.Reason, "",
		map[string]any{
			"balance": ledger.BalanceBefore.String(),
			"frozen":  frozenBefore.String(),
		},
		map[string]any{
			"balance":    ledger.BalanceAfter.String(),
			"frozen":     frozenAfter.String(),
			"hold_delta": ledger.HoldDelta.String(),
			"delta":      ledger.Delta.String(),
			"type":       change.Type,
			"ref_type":   change.RefType,
			"ref_id":     change.RefID,
			"cycle_used": cycleUsed.String(),
			"cycle_tier": cycleTier.String(),
		},
		model.AuditResultSuccess)
}
