package model

import "gorm.io/gorm"

// AdtySiteOwnership records each committed handover. Financial snapshots and
// these append-only records preserve ownership independently of management access.
type AdtySiteOwnership struct {
	Id             int64  `json:"id"`
	SiteId         int    `json:"site_id" gorm:"uniqueIndex:idx_site_ownership_version;index"`
	Version        int64  `json:"version" gorm:"uniqueIndex:idx_site_ownership_version"`
	OldOwnerUserId int    `json:"old_owner_user_id"`
	NewOwnerUserId int    `json:"new_owner_user_id"`
	OperatorId     int    `json:"operator_id"`
	Reason         string `json:"reason" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteOwnership) TableName() string { return "adty_site_ownerships" }

// CurrentAdtySiteFinancialOwnerTx freezes new business at its SQL acceptance
// point. A plain read avoids reversing the user -> site lock order in billing.
func CurrentAdtySiteFinancialOwnerTx(tx *gorm.DB, siteID int) (int, error) {
	if siteID <= 0 {
		return 0, nil
	}
	var site AdtySite
	if err := tx.Select("owner_user_id").First(&site, siteID).Error; err != nil {
		return 0, err
	}
	return site.OwnerUserId, nil
}

// ResolveAdtySiteFinancialOwnerTx interprets legacy zero snapshots as belonging
// to the owner before the first handover. New writers always freeze a positive
// owner, so second-resolution timestamps never decide ownership at a handover.
func ResolveAdtySiteFinancialOwnerTx(tx *gorm.DB, siteID, frozenOwnerID int) (int, error) {
	if siteID <= 0 || frozenOwnerID > 0 {
		return frozenOwnerID, nil
	}
	var history []AdtySiteOwnership
	if err := tx.Where("site_id = ?", siteID).Order("version").Limit(1).Find(&history).Error; err != nil {
		return 0, err
	}
	if len(history) > 0 {
		return history[0].OldOwnerUserId, nil
	}
	return CurrentAdtySiteFinancialOwnerTx(tx, siteID)
}

// freezeBillingSiteOwnerTx binds adjustments to their original operation or
// event. Legacy rows have zero snapshots; only a newly accepted root may use
// current management ownership.
func freezeBillingSiteOwnerTx(tx *gorm.DB, row *BillingOperation, newRoot bool) error {
	if row.SiteId <= 0 {
		return nil
	}
	if row.EarningRootKey != "" && row.EarningRootKey != row.OperationKey {
		var roots []BillingOperation
		if err := tx.Where("operation_key = ? AND user_id = ? AND site_id = ?", row.EarningRootKey, row.UserId, row.SiteId).Limit(1).Find(&roots).Error; err != nil {
			return err
		}
		if len(roots) > 0 {
			owner, err := ResolveAdtySiteFinancialOwnerTx(tx, row.SiteId, roots[0].SiteOwnerUserId)
			row.SiteOwnerUserId = owner
			return err
		}
		var events []AdtySiteEarningEvent
		if err := tx.Where("operation_key = ? AND user_id = ? AND site_id = ?", row.EarningRootKey, row.UserId, row.SiteId).Limit(1).Find(&events).Error; err != nil {
			return err
		}
		if len(events) > 0 {
			owner, err := ResolveAdtySiteFinancialOwnerTx(tx, row.SiteId, events[0].SiteOwnerUserId)
			row.SiteOwnerUserId = owner
			return err
		}
		newRoot = false
	}
	if row.SiteOwnerUserId > 0 {
		return nil
	}
	var err error
	if newRoot {
		row.SiteOwnerUserId, err = CurrentAdtySiteFinancialOwnerTx(tx, row.SiteId)
	} else {
		row.SiteOwnerUserId, err = ResolveAdtySiteFinancialOwnerTx(tx, row.SiteId, 0)
	}
	return err
}
