package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const AdtyBillingHoldOpen = "open"

// ErrAdtyBillingHoldTableMissing is returned when shortfall holds cannot be
// recorded or checked because the hold table is absent.
var ErrAdtyBillingHoldTableMissing = errors.New("adty billing hold table is missing")

// AdtyBillingHold is the same-account shortfall waiting for finance review.
// An open hold pauses new paid requests for that user. It is not a negative balance.
type AdtyBillingHold struct {
	Id             int64  `json:"id"`
	UserId         int    `json:"user_id" gorm:"index"`
	RequestId      string `json:"request_id" gorm:"uniqueIndex;type:varchar(64)"`
	ShortfallQuota int    `json:"shortfall_quota"`
	Status         string `json:"status" gorm:"index;type:varchar(16)"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyBillingHold) TableName() string { return "adty_billing_holds" }

// RecordAdtyBillingHold stores one shortfall. Replays of the same request do not add another hold.
func RecordAdtyBillingHold(userID int, requestID string, shortfall int) error {
	if userID <= 0 || shortfall <= 0 {
		return nil
	}
	if err := adtyRequireBillingHoldTable(); err != nil {
		return err
	}
	if requestID == "" {
		requestID = common.NewRequestId()
	}
	row := AdtyBillingHold{UserId: userID, RequestId: requestID, ShortfallQuota: shortfall, Status: AdtyBillingHoldOpen}
	return DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_id"}}, DoNothing: true}).Create(&row).Error
}

// AdtyUserPaidPaused reports whether this user has an open shortfall hold.
func AdtyUserPaidPaused(userID int) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	if err := adtyRequireBillingHoldTable(); err != nil {
		return false, err
	}
	var count int64
	err := DB.Model(&AdtyBillingHold{}).Where("user_id = ? AND status = ?", userID, AdtyBillingHoldOpen).Count(&count).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}

func adtyRequireBillingHoldTable() error {
	if DB == nil || !DB.Migrator().HasTable(&AdtyBillingHold{}) {
		return ErrAdtyBillingHoldTableMissing
	}
	return nil
}
