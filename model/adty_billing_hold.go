package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

const AdtyBillingHoldOpen = "open"

// ErrAdtyBillingHoldTableMissing is returned when shortfall holds cannot be
// recorded or checked because the hold table is absent.
var ErrAdtyBillingHoldTableMissing = errors.New("adty billing hold table is missing")

// AdtyBillingHold is the same-account shortfall waiting for finance review.
// It is a record, not a gate: an open hold no longer pauses requests (ISS-R4-0084).
// It is not a negative balance.
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

// 这里原来有一个 `AdtyUserPaidPaused`：它回答「这个用户有没有未结的差额」，
// 而它唯一的消费者是 `PreConsumeBilling` 里那道**永久封锁**——老板已裁定
// 「额度不足就不能调用，补充余额以后再调用」，那道门禁整个是多余的（`ISS-R4-0084`）。
//
// **函数一起删掉，不留给测试用。** 留一个只有测试在调的函数，就是今天
// `taskAdjustFunding` 那个形状：一条测试撑着一段没有生产入口的代码，
// 而它的「过期」永远不会被发现。要看差额，直接查 `adty_billing_holds`
// （财务对账就是这么用的，见 `RecordAdtyBillingHold` 保留）。

func adtyRequireBillingHoldTable() error {
	if DB == nil || !DB.Migrator().HasTable(&AdtyBillingHold{}) {
		return ErrAdtyBillingHoldTableMissing
	}
	return nil
}
