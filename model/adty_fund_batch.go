package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// AdtyFundBatch is one recharge principal or permanent bonus lot.
type AdtyFundBatch struct {
	Id        int64  `json:"id"`
	UserId    int    `json:"user_id" gorm:"index"`
	Kind      string `json:"kind" gorm:"type:varchar(16);index"`
	Source    string `json:"source" gorm:"type:varchar(64);index"`
	Original  int64  `json:"original"`
	Remaining int64  `json:"remaining"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyFundBatch) TableName() string { return "adty_fund_batches" }

// CreateAdtyFundBatchesTx records the principal and bonus lots for one credit.
// A replay of the same source does not insert a second pair.
func CreateAdtyFundBatchesTx(tx *gorm.DB, userID int, source string, principal, bonus int64) error {
	if tx == nil || userID <= 0 || source == "" {
		return nil
	}
	if !tx.Migrator().HasTable(&AdtyFundBatch{}) {
		return nil
	}
	var existing int64
	if err := tx.Model(&AdtyFundBatch{}).Where("user_id = ? AND source = ?", userID, source).Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	now := common.GetTimestamp()
	rows := make([]AdtyFundBatch, 0, 2)
	if principal > 0 {
		rows = append(rows, AdtyFundBatch{UserId: userID, Kind: "principal", Source: source, Original: principal, Remaining: principal, CreatedAt: now})
	}
	if bonus > 0 {
		rows = append(rows, AdtyFundBatch{UserId: userID, Kind: "bonus", Source: source, Original: bonus, Remaining: bonus, CreatedAt: now})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

// AdtyAfterPrincipalSpentTx runs in the charge transaction after non-legacy
// principal lots are reduced. Gift consumption does not call it.
var AdtyAfterPrincipalSpentTx func(tx *gorm.DB, userID int, principalQuota int64) error

// ConsumeAdtyFundBatchesTx spends principal lots before bonus lots.
// A user with no lots gets one legacy principal lot for the pre-charge balance,
// then this charge consumes from that lot.
func ConsumeAdtyFundBatchesTx(tx *gorm.DB, userID int, amount, balanceBefore int64) error {
	if tx == nil || userID <= 0 || amount <= 0 || !tx.Migrator().HasTable(&AdtyFundBatch{}) {
		return nil
	}
	var rows []AdtyFundBatch
	if err := tx.Where("user_id = ? AND remaining > 0", userID).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 && balanceBefore > 0 {
		legacy := AdtyFundBatch{UserId: userID, Kind: "principal", Source: "legacy", Original: balanceBefore, Remaining: balanceBefore}
		if err := tx.Create(&legacy).Error; err != nil {
			return err
		}
		rows = []AdtyFundBatch{legacy}
	}
	if len(rows) == 0 {
		return nil
	}
	sortAdtyFundBatches(rows)
	principalTaken := int64(0)
	for i := range rows {
		if amount == 0 {
			break
		}
		take := rows[i].Remaining
		if take > amount {
			take = amount
		}
		if err := tx.Model(&AdtyFundBatch{}).Where("id = ?", rows[i].Id).Update("remaining", rows[i].Remaining-take).Error; err != nil {
			return err
		}
		if rows[i].Kind == "principal" && rows[i].Source != "legacy" {
			principalTaken += take
		}
		amount -= take
	}
	if principalTaken > 0 && AdtyAfterPrincipalSpentTx != nil {
		return AdtyAfterPrincipalSpentTx(tx, userID, principalTaken)
	}
	return nil
}

func sortAdtyFundBatches(rows []AdtyFundBatch) {
	for i := 1; i < len(rows); i++ {
		item := rows[i]
		j := i
		for j > 0 && adtyFundBatchLess(item, rows[j-1]) {
			rows[j] = rows[j-1]
			j--
		}
		rows[j] = item
	}
}

func adtyFundBatchLess(a, b AdtyFundBatch) bool {
	if a.Kind != b.Kind {
		return a.Kind == "principal"
	}
	if a.CreatedAt != b.CreatedAt {
		return a.CreatedAt < b.CreatedAt
	}
	return a.Id < b.Id
}
