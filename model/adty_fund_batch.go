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
	_, err := consumeAdtyFundBatchesTx(tx, userID, amount, balanceBefore, true)
	return err
}

func consumeAdtyFundBatchesTx(tx *gorm.DB, userID int, amount, balanceBefore int64, notifyPrincipalSpent bool) (int64, error) {
	if tx == nil || userID <= 0 || amount <= 0 || !tx.Migrator().HasTable(&AdtyFundBatch{}) {
		return 0, nil
	}
	var rows []AdtyFundBatch
	if err := tx.Where("user_id = ? AND remaining > 0", userID).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 && balanceBefore > 0 {
		legacy := AdtyFundBatch{UserId: userID, Kind: "principal", Source: "legacy", Original: balanceBefore, Remaining: balanceBefore}
		if err := tx.Create(&legacy).Error; err != nil {
			return 0, err
		}
		rows = []AdtyFundBatch{legacy}
	}
	if len(rows) == 0 {
		return 0, nil
	}
	sortAdtyFundBatches(rows)
	principalTaken := int64(0)
	taken := int64(0)
	for i := range rows {
		if amount == 0 {
			break
		}
		take := rows[i].Remaining
		if take > amount {
			take = amount
		}
		if err := tx.Model(&AdtyFundBatch{}).Where("id = ?", rows[i].Id).Update("remaining", rows[i].Remaining-take).Error; err != nil {
			return taken, err
		}
		if rows[i].Kind == "principal" && rows[i].Source != "legacy" {
			principalTaken += take
		}
		amount -= take
		taken += take
	}
	if notifyPrincipalSpent && principalTaken > 0 && AdtyAfterPrincipalSpentTx != nil {
		return taken, AdtyAfterPrincipalSpentTx(tx, userID, principalTaken)
	}
	return taken, nil
}

// ReclaimAdtyFundBatchesBySourceTx reduces remaining on the lots of one credit
// source. Principal and bonus are reduced independently and capped at each
// kind's remaining. It does not invent lots when none were recorded.
func ReclaimAdtyFundBatchesBySourceTx(tx *gorm.DB, userID int, source string, principal, bonus int64) (int64, error) {
	if tx == nil || userID <= 0 || source == "" || (principal <= 0 && bonus <= 0) {
		return 0, nil
	}
	if !tx.Migrator().HasTable(&AdtyFundBatch{}) {
		return 0, nil
	}
	var rows []AdtyFundBatch
	if err := tx.Where("user_id = ? AND source = ? AND remaining > 0", userID, source).Find(&rows).Error; err != nil {
		return 0, err
	}
	taken := int64(0)
	for i := range rows {
		var want *int64
		switch rows[i].Kind {
		case "principal":
			want = &principal
		case "bonus":
			want = &bonus
		default:
			continue
		}
		if *want <= 0 {
			continue
		}
		take := rows[i].Remaining
		if take > *want {
			take = *want
		}
		if err := tx.Model(&AdtyFundBatch{}).Where("id = ?", rows[i].Id).Update("remaining", rows[i].Remaining-take).Error; err != nil {
			return taken, err
		}
		*want -= take
		taken += take
	}
	return taken, nil
}

// applyAdtyWalletDebitToBatchesTx keeps the batch ledger in step with a wallet
// debit. It first reclaims the named source's principal and bonus, then
// FIFO-consumes any shortfall so total remaining tracks the wallet. Refund
// shortfalls do not fire the consumption hook; that hook is for charges.
func applyAdtyWalletDebitToBatchesTx(tx *gorm.DB, userID int, source string, principal, bonus, deducted, balanceBefore int64) error {
	if deducted <= 0 {
		return nil
	}
	taken, err := ReclaimAdtyFundBatchesBySourceTx(tx, userID, source, principal, bonus)
	if err != nil {
		return err
	}
	leftover := deducted - taken
	if leftover <= 0 {
		return nil
	}
	remainingBefore := balanceBefore - taken
	if remainingBefore < 0 {
		remainingBefore = 0
	}
	_, err = consumeAdtyFundBatchesTx(tx, userID, leftover, remainingBefore, false)
	return err
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
