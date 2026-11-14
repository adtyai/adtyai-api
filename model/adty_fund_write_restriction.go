package model

import (
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// ErrAdtyFundWriteRestricted is returned when a reviewing fund-anomaly
// workbench item covers the write. The restriction lifts when the item is closed.
var ErrAdtyFundWriteRestricted = errors.New("related fund writes are restricted until the anomaly review is closed")

// AdtyFundWrite is the account, order, and/or site a fund write belongs to.
// Empty identifiers are omitted from matching so an order-scoped review does
// not block unrelated account or site writes.
type AdtyFundWrite struct {
	SiteID    int
	AccountID int
	OrderRef  string
}

// RejectRestrictedAdtyFundWrite blocks a write that falls in the scope of a
// reviewing workbench item. Open (detection) and closed items do not restrict.
// Scope is the narrowest recorded identifier: order, otherwise account,
// otherwise site. This is not a separate freeze product and does not reuse
// account-disable or global-freeze.
func RejectRestrictedAdtyFundWrite(tx *gorm.DB, write AdtyFundWrite) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil
	}
	write.OrderRef = strings.TrimSpace(write.OrderRef)
	if !tx.Migrator().HasTable(&AdtyAnomalyDisposition{}) {
		return nil
	}
	if err := resolveAdtyFundWriteSite(tx, &write); err != nil {
		return err
	}
	if write.SiteID <= 0 && write.AccountID <= 0 && write.OrderRef == "" {
		return nil
	}
	rows, err := loadReviewingAdtyFundRestrictions(tx, write)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if adtyFundWriteRestrictedBy(row, write) {
			return ErrAdtyFundWriteRestricted
		}
	}
	return nil
}

func resolveAdtyFundWriteSite(tx *gorm.DB, write *AdtyFundWrite) error {
	if write.SiteID > 0 || write.AccountID <= 0 {
		return nil
	}
	var siteID int
	err := tx.Model(&User{}).Select("site_id").Where("id = ?", write.AccountID).Limit(1).Scan(&siteID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	write.SiteID = siteID
	return nil
}

func loadReviewingAdtyFundRestrictions(tx *gorm.DB, write AdtyFundWrite) ([]AdtyAnomalyDisposition, error) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 6)
	if write.OrderRef != "" {
		clauses = append(clauses, "order_ref = ?")
		args = append(args, write.OrderRef)
	}
	if write.AccountID > 0 {
		clauses = append(clauses, "(order_ref = '' AND account_id = ?)")
		args = append(args, write.AccountID)
	}
	if write.SiteID > 0 {
		clauses = append(clauses, "(order_ref = '' AND account_id = 0 AND (site_id = ? OR (site_id = 0 AND ref = ?)))")
		args = append(args, write.SiteID, strconv.Itoa(write.SiteID))
	}
	if len(clauses) == 0 {
		return nil, nil
	}
	var rows []AdtyAnomalyDisposition
	err := tx.Where("status = ?", "reviewing").
		Where(strings.Join(clauses, " OR "), args...).
		Limit(100).
		Find(&rows).Error
	return rows, err
}

func adtyFundWriteRestrictedBy(row AdtyAnomalyDisposition, write AdtyFundWrite) bool {
	orderRef := strings.TrimSpace(row.OrderRef)
	if orderRef != "" {
		return write.OrderRef != "" && write.OrderRef == orderRef
	}
	if row.AccountId > 0 {
		return write.AccountID > 0 && write.AccountID == row.AccountId
	}
	siteID := row.SiteId
	if siteID <= 0 {
		parsed, err := strconv.Atoi(strings.TrimSpace(row.Ref))
		if err != nil || parsed <= 0 {
			return false
		}
		siteID = parsed
	}
	return write.SiteID > 0 && write.SiteID == siteID
}
