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

// adtyLegacyBatchSource 是「没有订单来源的额度」那一条批次的固定 source。
// 每个用户只有一条（唯一索引 user_id + source + kind），补额度是往上加，
// 不是新插一行——否则第二次补建就会撞唯一索引。
const adtyLegacyBatchSource = "legacy"

// ensureAdtyLegacyBatchTx 把「没有批次背书」的额度补进那条 legacy 批次。
//
// topUp 是本次要补的额度。已经存在就加在 remaining/original 上（幂等：补过之后
// 再算一次 short 会是 0）；不存在才新建。
//
// 走 SAVEPOINT：并发下两个事务可能同时判断「还没有 legacy 批次」，唯一索引会让
// 其中一个失败。冲突时重读并补额度，不算错误——否则并发的第二笔扣费会整笔失败。
func ensureAdtyLegacyBatchTx(tx *gorm.DB, userID int, topUp int64) error {
	if tx == nil || userID <= 0 || topUp <= 0 {
		return nil
	}
	apply := func(db *gorm.DB) error {
		result := db.Model(&AdtyFundBatch{}).
			Where("user_id = ? AND source = ? AND kind = ?", userID, adtyLegacyBatchSource, "principal").
			Updates(map[string]any{
				"remaining": gorm.Expr("remaining + ?", topUp),
				"original":  gorm.Expr("original + ?", topUp),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			return nil
		}
		return db.Create(&AdtyFundBatch{
			UserId: userID, Kind: "principal", Source: adtyLegacyBatchSource,
			Original: topUp, Remaining: topUp,
		}).Error
	}
	err := tx.Transaction(apply)
	if err != nil && isAdtyUniqueViolation(err) {
		// 另一个事务刚建好这一行：补同样的额度，然后照常消费。
		return tx.Transaction(apply)
	}
	return err
}

// AdtyAfterPrincipalSpentTx runs in the charge transaction after non-legacy
// principal lots are reduced. Gift consumption does not call it.
var AdtyAfterPrincipalSpentTx func(tx *gorm.DB, userID int, principalQuota int64) error

// AdtyBonusSpend is one source's bonus lot reduction in a usage charge.
type AdtyBonusSpend struct {
	Source string
	Quota  int64
}

// AdtyAfterBonusSpentTx runs after bonus lots are reduced, both on usage
// charges and on the quiet path used by official adjustments and staff
// approvals, so the matching site gift-guarantee can be released. Refunds
// that reclaim lots by source do not call it.
var AdtyAfterBonusSpentTx func(tx *gorm.DB, userID int, spends []AdtyBonusSpend) error

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
// It does fire the bonus-spent hook so an approved or official reduction that
// consumes top-up bonus lots can release the corresponding site gift guarantee.
func ConsumeAdtyFundBatchesQuietTx(tx *gorm.DB, userID int, amount, balanceBefore int64) error {
	_, err := consumeAdtyFundBatchesTx(tx, userID, amount, balanceBefore, false)
	return err
}

// consumeAdtyFundBatchesTx returns how much of the debit was backed by lots that
// existed before any legacy top-up, not how much was debited. The difference is
// the drift: money that sat on the account with no lot behind it. Callers that
// only need the error discard it; settlement records it as fund_batch_gap.
func consumeAdtyFundBatchesTx(tx *gorm.DB, userID int, amount, balanceBefore int64, notifyPrincipalSpent bool) (int64, error) {
	if tx == nil || userID <= 0 || amount <= 0 {
		return 0, nil
	}
	if err := adtyRequireFundBatchTable(tx); err != nil {
		return 0, err
	}
	var rows []AdtyFundBatch
	if err := tx.Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return 0, err
	}
	// 批次总量与「这次业务之前」的余额之间的差，是**没有批次背书的额度**：
	// 升级前的余额从来没有对应的行，退款（IncreaseUserQuota）也不会补行。
	// 这部分要落到那条唯一的 legacy 批次上，否则扣费一定失败。
	var lotTotal int64
	// realRemaining 记下**补建之前**每个批次真正有多少额度。补建之后 remaining
	// 里有一部分是刚凭空写进去的，不能算进「批次背书了多少」。
	realRemaining := make(map[int64]int64, len(rows))
	for i := range rows {
		lotTotal += rows[i].Remaining
		realRemaining[rows[i].Id] = rows[i].Remaining
	}
	if short := balanceBefore - lotTotal; short > 0 {
		// 补建是必须的：存量用户升级前的额度从来没有对应行，退款
		// （IncreaseUserQuota）也不会补行，不补建就一笔都扣不出去。
		//
		// 但**补建不等于「这批钱有批次背书」**。补出去的那部分正是账本漂移，
		// 必须由调用方作为 fund_batch_gap 记下来——否则「批次永远等于余额」，
		// 对账永远读到 0，这一列等于没加。所以返回值报的是补建前的真实覆盖，
		// 不是实际扣掉的额度。
		if err := ensureAdtyLegacyBatchTx(tx, userID, short); err != nil {
			return 0, err
		}
	}
	// 只消费还有剩余的批次；上面补过之后重新读，避免拿旧快照算。
	if err := tx.Where("user_id = ? AND remaining > 0", userID).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, adtyFundBatchShortfallError(amount)
	}
	sortAdtyFundBatches(rows)
	principalTaken := int64(0)
	taken := int64(0)
	backed := int64(0)
	bonusBySource := map[string]int64{}
	for i := range rows {
		if amount == 0 {
			break
		}
		take := rows[i].Remaining
		if take > amount {
			take = amount
		}
		// 本次从这一行扣走的额度里，有多少是补建之前就真在那儿的。
		// 逐行取 min 而不是「taken - short」：扣费是从真实批次先扣的，
		// 当本次扣费小于缺口时（比如余额 50、批次 20、只扣 10），
		// taken - short 会算成 -20，把一次完全有背书的扣费报成漂移。
		if real := realRemaining[rows[i].Id]; real > 0 {
			backed += take
			if take > real {
				backed -= take - real
			}
		}
		// 扣减交给 deductAdtyFundBatchTx 的同一套三件套（WHERE remaining >= ? +
		// gorm.Expr("remaining - ?") + RowsAffected 校验）：WHERE 挡「余额不足」，
		// Expr 挡「丢更新」——不把内存里读到的绝对值写回去。这里不再依赖
		// 「调用方已锁 users 行」这个未声明的约定，那只是当前的巧合，不是保证。
		if err := deductAdtyFundBatchTx(tx, rows[i].Id, take); err != nil {
			return backed, err
		}
		if rows[i].Kind == "principal" && rows[i].Source != "legacy" {
			principalTaken += take
		}
		if rows[i].Kind == "bonus" && rows[i].Source != "" && take > 0 {
			bonusBySource[rows[i].Source] += take
		}
		amount -= take
		taken += take
	}
	if amount > 0 {
		return backed, adtyFundBatchShortfallError(amount)
	}
	// notifyPrincipalSpent gates only the usage-charge hook. The bonus hook
	// still runs on the quiet path so manual/staff reductions release the
	// corresponding site gift-guarantee without debiting the operating wallet.
	if notifyPrincipalSpent && principalTaken > 0 && AdtyAfterPrincipalSpentTx != nil {
		if err := AdtyAfterPrincipalSpentTx(tx, userID, principalTaken); err != nil {
			return backed, err
		}
	}
	if len(bonusBySource) > 0 && AdtyAfterBonusSpentTx != nil {
		spends := make([]AdtyBonusSpend, 0, len(bonusBySource))
		for source, quota := range bonusBySource {
			spends = append(spends, AdtyBonusSpend{Source: source, Quota: quota})
		}
		return backed, AdtyAfterBonusSpentTx(tx, userID, spends)
	}
	_ = taken
	return backed, nil
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
		// 同上：条件更新 + 原子减法 + RowsAffected 校验，缺一件就分别是
		// 「扣成负数」或「并发覆盖掉别人已提交的扣减」。
		if err := deductAdtyFundBatchTx(tx, rows[i].Id, take); err != nil {
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
