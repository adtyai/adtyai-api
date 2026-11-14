package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ErrAdtyFundBatchShortfall is returned when FIFO lots cannot cover a debit.
var ErrAdtyFundBatchShortfall = errors.New("fund batches cannot cover the debit")

// ErrAdtyFundBatchTableMissing is returned when fund batches cannot be written
// or consumed because the batch table is absent.
var ErrAdtyFundBatchTableMissing = errors.New("adty fund batch table is missing")

func adtyFundBatchShortfallError(uncovered int64) error {
	return fmt.Errorf("%w: uncovered %d", ErrAdtyFundBatchShortfall, uncovered)
}

// AdtyFundBatch is one recharge principal or permanent bonus lot.
type AdtyFundBatch struct {
	Id        int64  `json:"id"`
	UserId    int    `json:"user_id" gorm:"uniqueIndex:idx_adty_fund_batch_user_source_kind,priority:1;index"`
	Kind      string `json:"kind" gorm:"type:varchar(16);uniqueIndex:idx_adty_fund_batch_user_source_kind,priority:3;index"`
	Source    string `json:"source" gorm:"type:varchar(64);uniqueIndex:idx_adty_fund_batch_user_source_kind,priority:2;index"`
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
	if err := adtyRequireFundBatchTable(tx); err != nil {
		return err
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
	// Insert under a SAVEPOINT so a concurrent unique conflict does not
	// abort the caller's PostgreSQL transaction. A replay of the same
	// (user_id, source, kind) is idempotent success, same as Count>0.
	err := tx.Transaction(func(inner *gorm.DB) error {
		return inner.Create(&rows).Error
	})
	if err != nil && isAdtyUniqueViolation(err) {
		return nil
	}
	return err
}

// AdtyAfterPrincipalSpentTx runs in the charge transaction after non-legacy
// principal lots are reduced. Gift consumption does not call it.
var AdtyAfterPrincipalSpentTx func(tx *gorm.DB, userID int, principalQuota int64) error

// ConsumeAdtyFundBatchesTx spends principal lots before bonus lots.
// A user with no lots gets one legacy principal lot for the pre-charge balance,
// then this charge consumes from that lot. If lots still cannot cover the debit,
// it returns ErrAdtyFundBatchShortfall so the surrounding wallet transaction
// can roll back.
func ConsumeAdtyFundBatchesTx(tx *gorm.DB, userID int, amount, balanceBefore int64) error {
	_, err := consumeAdtyFundBatchesTx(tx, userID, amount, balanceBefore, true)
	return err
}

// ConsumeAdtyFundBatchesQuietTx spends lots in FIFO order so remaining tracks
// a non-charge wallet debit. It does not fire the principal-spent hook; that
// hook is for usage charges, not official adjustments or staff approvals.
func ConsumeAdtyFundBatchesQuietTx(tx *gorm.DB, userID int, amount, balanceBefore int64) error {
	_, err := consumeAdtyFundBatchesTx(tx, userID, amount, balanceBefore, false)
	return err
}

func consumeAdtyFundBatchesTx(tx *gorm.DB, userID int, amount, balanceBefore int64, notifyPrincipalSpent bool) (int64, error) {
	if tx == nil || userID <= 0 || amount <= 0 {
		return 0, nil
	}
	if err := adtyRequireFundBatchTable(tx); err != nil {
		return 0, err
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
		return 0, adtyFundBatchShortfallError(amount)
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
	if amount > 0 {
		return taken, adtyFundBatchShortfallError(amount)
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
	if err := adtyRequireFundBatchTable(tx); err != nil {
		return 0, err
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
// FIFO-consumes any leftover so total remaining tracks the wallet. If lots
// still cannot cover the debit, consume returns ErrAdtyFundBatchShortfall.
// A leftover after reclaim does not fire the consumption hook; that hook is
// for charges.
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

func adtyRequireFundBatchTable(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&AdtyFundBatch{}) {
		return ErrAdtyFundBatchTableMissing
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

// AdtyFundBatchKindRemaining is the remaining quota of one lot kind for one user.
type AdtyFundBatchKindRemaining struct {
	Principal int64
	Bonus     int64
	HasLots   bool
}

// SumAdtyFundBatchRemainingByKind totals remaining principal and permanent bonus lots.
// It does not invent lots and does not include promised or not-yet-granted bonus.
func SumAdtyFundBatchRemainingByKind(userID int) (AdtyFundBatchKindRemaining, error) {
	sums, err := SumAdtyFundBatchRemainingByKindForUsers([]int{userID})
	if err != nil {
		return AdtyFundBatchKindRemaining{}, err
	}
	return sums[userID], nil
}

// SumAdtyFundBatchRemainingByKindForUsers totals remaining principal and bonus lots per user.
func SumAdtyFundBatchRemainingByKindForUsers(userIDs []int) (map[int]AdtyFundBatchKindRemaining, error) {
	out := map[int]AdtyFundBatchKindRemaining{}
	if DB == nil || len(userIDs) == 0 {
		return out, nil
	}
	ids := uniquePositiveIDs(userIDs)
	if len(ids) == 0 {
		return out, nil
	}
	if err := adtyRequireFundBatchTable(DB); err != nil {
		return nil, err
	}
	type kindSum struct {
		UserId    int    `gorm:"column:user_id"`
		Kind      string `gorm:"column:kind"`
		Remaining int64  `gorm:"column:remaining"`
	}
	var rows []kindSum
	if err := DB.Model(&AdtyFundBatch{}).
		Select("user_id, kind, COALESCE(SUM(remaining), 0) AS remaining").
		Where("user_id IN ? AND kind IN ?", ids, []string{"principal", "bonus"}).
		Group("user_id, kind").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		current := out[row.UserId]
		current.HasLots = true
		switch row.Kind {
		case "principal":
			current.Principal = row.Remaining
		case "bonus":
			current.Bonus = row.Remaining
		}
		out[row.UserId] = current
	}
	return out, nil
}

func uniquePositiveIDs(ids []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
