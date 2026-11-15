package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ErrAdtyFundAllocationMissing is returned when a settlement or release cannot
// find the pre-charge share it is supposed to act on.
var ErrAdtyFundAllocationMissing = errors.New("adty fund allocation is missing")

// AdtyFundAllocation is the per-request share of one fund batch. A.12.1 requires
// every batch to keep its 冻结/预扣 relation, and A.12.2.6 requires cancellations,
// pre-charge releases and refunds to return funds "按原分摊" instead of a
// merged balance. One batch can back many requests, so the operation identity is
// part of the key: without it a release cannot tell which share to give back.
//
// Capacity authority stays with adty_fund_batches.remaining: this step only
// records which batch each pre-charge draws from, following the A.12.2 order
// (principal first, then bonus, FIFO inside each kind). Settlement is what turns
// a share into real consumption, and that step is what starts subtracting from
// remaining. Because of that, shares are never subtracted from capacity here —
// doing so before settlement exists would make outstanding shares eat a user's
// usable quota forever.
type AdtyFundAllocation struct {
	Id          int64  `json:"id" gorm:"primaryKey"`
	UserId      int    `json:"user_id" gorm:"index;not null"`
	BatchId     int64  `json:"batch_id" gorm:"not null;uniqueIndex:ux_adty_fund_allocation_share,priority:1"`
	OperationId string `json:"operation_id" gorm:"type:varchar(191);not null;uniqueIndex:ux_adty_fund_allocation_share,priority:2;index"`
	Seq         int    `json:"seq" gorm:"not null;uniqueIndex:ux_adty_fund_allocation_share,priority:3"`
	Kind        string `json:"kind" gorm:"type:varchar(16)"` // mirrors the batch kind at allocation time
	Source      string `json:"source" gorm:"type:varchar(64)"`
	Held        int64  `json:"held" gorm:"not null;default:0"`    // pre-charged, not yet settled
	Settled     int64  `json:"settled" gorm:"not null;default:0"` // turned into real consumption
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyFundAllocation) TableName() string { return "adty_fund_allocations" }

// AllocateAdtyFundAllocationsTx records the pre-charge shares for one operation.
// amount is the cumulative quota this operation should have reserved: a repeated
// call only records the difference, so the top-up path can call it again with a
// larger target without double-claiming a batch.
//
// It spends principal before bonus and follows the A.12.2 FIFO order, so a later
// settlement can turn exactly these shares into real consumption and a release
// can hand them back without guessing.
//
// A user without lots gets no shares: those accounts still settle through the
// legacy path in ConsumeAdtyFundBatchesTx, which invents a lot on first spend.
// balanceBefore 是「这次业务开始消耗之前」的钱包余额。它用来判断账面缺多少额度：
// 预扣阶段补建 legacy 批次，才不会让每一笔「老用户 + 没有批次」的预扣都打一条
// ERROR（结算时本来就会补、会扣，那条 ERROR 是误报）。
func AllocateAdtyFundAllocationsTx(tx *gorm.DB, userID int, operationID string, amount, balanceBefore int64) error {
	if tx == nil || userID <= 0 || operationID == "" || amount <= 0 {
		return nil
	}
	if err := adtyRequireFundBatchTable(tx); err != nil {
		return err
	}
	// 同一 operation 可能多次预扣（发送前补扣、阶梯价上调）。amount 是累计应预扣
	// 总额，这里只补差额，重复调用不会重复登记。
	var taken struct {
		Held    int64
		Settled int64
	}
	if err := tx.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held),0) as held, coalesce(sum(settled),0) as settled").
		Where("operation_id = ? AND user_id = ?", operationID, userID).
		Scan(&taken).Error; err != nil {
		return err
	}
	amount -= taken.Held + taken.Settled
	if amount <= 0 {
		return nil
	}
	var batches []AdtyFundBatch
	if err := tx.Where("user_id = ?", userID).Find(&batches).Error; err != nil {
		return err
	}
	if len(batches) == 0 {
		// 一条批次都没有的账户（升级前就有额度、退款也不补行）在预扣阶段不登记。
		// **不要为了「顺手」在这里补建或登记**：那要等退款路径先能释放占用，
		// 否则会把这些账户也拖进「退款后占用残留」的池子里。他们现在由结算路径的
		// 补建兜住，是能正常消费的。
		return nil
	}
	// 同一批次上别人还占着的额度必须先扣掉，否则两个并发预扣会把同一笔
	// remaining 各登记一次；结算时条件更新只允许一个成功，另一个整事务回滚，
	// 那个请求已经调过上游、却再也结不了账。
	heldByBatch, err := outstandingAdtyFundAllocationByBatchTx(tx, userID)
	if err != nil {
		return err
	}
	var maxSeq struct{ Seq int }
	if err := tx.Model(&AdtyFundAllocation{}).
		Select("coalesce(max(seq),0) as seq").
		Where("operation_id = ? AND user_id = ?", operationID, userID).
		Scan(&maxSeq).Error; err != nil {
		return err
	}
	pickAdtyFundBatchOrder(batches)
	remaining := amount
	seq := maxSeq.Seq
	for _, batch := range batches {
		if remaining <= 0 {
			break
		}
		// Principal lots are spent before bonus lots (A.12.2 step 1 and 4).
		if batch.Kind != "principal" {
			continue
		}
		available := batch.Remaining - heldByBatch[batch.Id]
		if available <= 0 {
			continue
		}
		take := min(available, remaining)
		seq++
		if err := tx.Create(&AdtyFundAllocation{
			UserId:      userID,
			BatchId:     batch.Id,
			OperationId: operationID,
			Seq:         seq,
			Kind:        batch.Kind,
			Source:      batch.Source,
			Held:        take,
		}).Error; err != nil {
			if isAdtyUniqueViolation(err) {
				return ErrAdtyFundAllocationMissing
			}
			return err
		}
		remaining -= take
	}
	for _, batch := range batches {
		if remaining <= 0 {
			break
		}
		if batch.Kind != "bonus" {
			continue
		}
		available := batch.Remaining - heldByBatch[batch.Id]
		if available <= 0 {
			continue
		}
		take := min(available, remaining)
		seq++
		if err := tx.Create(&AdtyFundAllocation{
			UserId:      userID,
			BatchId:     batch.Id,
			OperationId: operationID,
			Seq:         seq,
			Kind:        batch.Kind,
			Source:      batch.Source,
			Held:        take,
		}).Error; err != nil {
			if isAdtyUniqueViolation(err) {
				return ErrAdtyFundAllocationMissing
			}
			return err
		}
		remaining -= take
	}
	if remaining > 0 && balanceBefore > 0 {
		// 批次不足以背书这次预扣。存量用户可能有一部分额度从来没有对应批次
		// （升级前就在账上，退款也不会补行），所以先把缺的那块补进唯一的
		// legacy 批次，再按新批次登记一次分摊。
		//
		// 为什么在预扣就补、而不是留到结算：留到结算也能算对，但每次「花完 →
		// 退款 → 再消费」都会在预扣阶段打一条 ERROR。生产上那条会被当成资金
		// 异常去查，而真正的缺口反而叫不出声。
		var lotTotal int64
		for i := range batches {
			lotTotal += batches[i].Remaining
		}
		// **只在批次总额为 0 时补**，而不是「只要有差额就补」。
		//
		// 这个边界是整个 drift 机制的前提：如果每次预扣都把 remaining 补齐到
		// 余额，批次表就永远等于余额，`fund_batch_gap` 永远是 0，对账失去意义。
		// 「有批次但少于余额」正是账本漂移的形状，它应当被**记录**，不是被悄悄修好。
		//
		// 覆盖：
		//   1. 唯一那条 legacy 批次已经被花光（退款回额度不会补批次）。
		// 不覆盖：
		//   2. 从来没有批次的老用户 —— 他们在上面的 `len(batches) == 0` 就返回了，
		//      走不到这里；那类账户靠**结算路径**的补建兜住，不是靠这一处。
		//
		// 这么写是因为原来那句「覆盖两种正当情形」把边界说大了一圈，而它正是
		// 讨论 drift 的前提。
		if lotTotal == 0 && balanceBefore > 0 {
			if err := ensureAdtyLegacyBatchTx(tx, userID, balanceBefore); err != nil {
				return err
			}
			var fresh []AdtyFundBatch
			if err := tx.Where("user_id = ? AND remaining > 0", userID).Find(&fresh).Error; err != nil {
				return err
			}
			pickAdtyFundBatchOrder(fresh)
			var maxSeq struct{ Seq int }
			if err := tx.Model(&AdtyFundAllocation{}).
				Select("coalesce(max(seq),0) as seq").
				Where("operation_id = ? AND user_id = ?", operationID, userID).
				Scan(&maxSeq).Error; err != nil {
				return err
			}
			seq := maxSeq.Seq
			for _, batch := range fresh {
				if remaining <= 0 {
					break
				}
				if batch.Kind != "principal" {
					continue
				}
				take := min(batch.Remaining, remaining)
				if take <= 0 {
					continue
				}
				seq++
				if err := tx.Create(&AdtyFundAllocation{
					UserId: userID, BatchId: batch.Id, OperationId: operationID, Seq: seq,
					Kind: batch.Kind, Source: batch.Source, Held: take,
				}).Error; err != nil {
					if isAdtyUniqueViolation(err) {
						return ErrAdtyFundAllocationMissing
					}
					return err
				}
				remaining -= take
			}
		}
	}
	if remaining > 0 {
		// 仍然不够：这里的余额确实没有可依据的来源（例如 balanceBefore 也为 0）。
		// 不拒绝预扣——users.quota 才是准入权威——但要留一条可查的痕迹。
		common.SysError(common.LogText("adty fund allocation short by %d quota for user=%d operation=%s: batches cannot back the whole reservation", remaining, userID, operationID))
	}
	return nil
}

// SumAdtyFundAllocationHeld returns the quota a user still has held in
// pre-charge shares, split by batch kind. It exists for the reconciliation check
// that compares share totals against the live balance.
func SumAdtyFundAllocationHeld(userID int) (principal, bonus int64, err error) {
	if DB == nil || userID <= 0 {
		return 0, 0, nil
	}
	if err := adtyRequireFundBatchTable(DB); err != nil {
		return 0, 0, err
	}
	var rows []struct {
		Kind string
		Held int64
	}
	if err := DB.Model(&AdtyFundAllocation{}).
		Select("kind, coalesce(sum(held - settled),0) as held").
		Where("user_id = ?", userID).
		Group("kind").
		Scan(&rows).Error; err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		if row.Kind == "principal" {
			principal = row.Held
		} else if row.Kind == "bonus" {
			bonus = row.Held
		}
	}
	return principal, bonus, nil
}

// outstandingAdtyFundAllocationByBatchTx 汇总每个批次上仍被占用（已登记未结算）的额度。
// 预扣时要从批次的可用量里扣掉它，否则同一笔 remaining 会被两个并发预扣各登记一次。
func outstandingAdtyFundAllocationByBatchTx(tx *gorm.DB, userID int) (map[int64]int64, error) {
	out := map[int64]int64{}
	if tx == nil || userID <= 0 {
		return out, nil
	}
	var rows []struct {
		BatchId int64
		Held    int64
	}
	if err := tx.Model(&AdtyFundAllocation{}).
		Select("batch_id, coalesce(sum(held - settled),0) as held").
		Where("user_id = ? AND held > settled", userID).
		Group("batch_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.BatchId] = row.Held
	}
	return out, nil
}

// pickAdtyFundBatchOrder sorts lots the way A.12.2 consumes them: principal by
// creation time then id, then bonus by creation time then id.
func pickAdtyFundBatchOrder(rows []AdtyFundBatch) {
	for i := 1; i < len(rows); i++ {
		item := rows[i]
		j := i
		for j > 0 && adtyFundKindRank(rows[j-1]) > adtyFundKindRank(item) {
			rows[j] = rows[j-1]
			j--
		}
		for j > 0 && adtyFundKindRank(rows[j-1]) == adtyFundKindRank(item) && !adtyFundBatchLess(rows[j-1], item) {
			rows[j] = rows[j-1]
			j--
		}
		rows[j] = item
	}
}

func adtyFundKindRank(row AdtyFundBatch) int {
	if row.Kind == "principal" {
		return 0
	}
	return 1
}

// SettleAdtyFundAllocationsTx 把一次结算落到批次与分摊上。
//
// applied 是这次业务最终真正消耗的额度（= 预扣 base + 补扣 charged，全额退款时为 0）。
// 返回值 backed 是**批次真正支撑得起的那部分**；applied-backed 就是这笔业务的账本缺口，
// 调用方把它落进 billing_operations.fund_batch_gap 供按用户聚合对账。
//
// 规则：
//   - 已登记的分摊按 seq 顺序把 held 转成 settled，对应金额从批次 remaining 扣掉；
//   - applied 小于已登记额度时，剩下的部分只是释放（批次没被扣过，不需要回补）；
//   - applied 大于已登记额度时（升级前的旧 operation、或补扣没登记成功），
//     差额按 FIFO 现场补记一条已结算分摊再扣，保证 remaining 与 quota 同步。
//
// 扣批次用条件更新 + RowsAffected 校验：并发下扣不够就整事务回滚，
// 不做静默重试——那会把「这次没扣成」伪装成「扣到别的地方去了」。
func SettleAdtyFundAllocationsTx(tx *gorm.DB, userID int, operationID string, applied int64, balanceBefore int64) (int64, error) {
	if tx == nil || userID <= 0 {
		return 0, nil
	}
	if applied < 0 {
		return 0, ErrAdtyFundAllocationMissing
	}
	var shares []AdtyFundAllocation
	if operationID != "" {
		if err := lockForUpdate(tx).
			Where("user_id = ? AND operation_id = ?", userID, operationID).
			Order("seq ASC").Find(&shares).Error; err != nil {
			return 0, err
		}
	}
	if len(shares) == 0 {
		// 没有分摊：升级前的 operation 与 legacy 直接扣费仍然走原路径，
		// 保留「首次消费时为旧余额补建 legacy 批次」的行为。
		//
		// 但这条路径**必须报出真实的 backed**，不能直接 return applied。
		// 直接 return applied 会让 `fund_batch_gap` 恒等于 0，而这里恰恰是
		// 最需要观测漂移的一类账户（没有分摊的 legacy 用户）——那一列的全部
		// 意义就被这条 return 抵消掉了。
		if applied == 0 {
			return 0, nil
		}
		backed, err := consumeAdtyFundBatchesTx(tx, userID, applied, balanceBefore, true)
		if err != nil {
			return 0, err
		}
		return backed, nil
	}

	left := applied
	for i := range shares {
		held := shares[i].Held - shares[i].Settled
		if held <= 0 {
			continue
		}
		take := min(held, left)
		if take <= 0 {
			// 本次没用到这部分：只释放，不动 remaining。
			if err := tx.Model(&AdtyFundAllocation{}).
				Where("id = ? AND held = ?", shares[i].Id, shares[i].Held).
				Updates(map[string]any{"held": 0}).Error; err != nil {
				return 0, err
			}
			continue
		}
		if err := deductAdtyFundBatchTx(tx, shares[i].BatchId, take); err != nil {
			return 0, err
		}
		// 只消费了一部分时，把这份分摊收窄到实际消耗，held 与 settled 相等，
		// 未用到的部分随之释放（批次没有扣过它，不需要回补）。
		if err := tx.Model(&AdtyFundAllocation{}).
			Where("id = ? AND held = ?", shares[i].Id, shares[i].Held).
			Updates(map[string]any{"held": take, "settled": shares[i].Settled + take}).Error; err != nil {
			return 0, err
		}
		left -= take
	}
	if left <= 0 {
		return applied, nil
	}
	// 已登记额度不够覆盖实际消耗：补记一条已结算分摊，再按 FIFO 扣批次。
	if err := AllocateAdtyFundAllocationsTx(tx, userID, operationID, applied, balanceBefore); err != nil {
		return 0, err
	}
	if err := tx.Where("user_id = ? AND operation_id = ? AND settled = 0", userID, operationID).
		Order("seq ASC").Find(&shares).Error; err != nil {
		return 0, err
	}
	for i := range shares {
		if left <= 0 {
			break
		}
		held := shares[i].Held - shares[i].Settled
		if held <= 0 {
			continue
		}
		take := min(held, left)
		if err := deductAdtyFundBatchTx(tx, shares[i].BatchId, take); err != nil {
			return 0, err
		}
		if err := tx.Model(&AdtyFundAllocation{}).
			Where("id = ? AND held = ?", shares[i].Id, shares[i].Held).
			Updates(map[string]any{"held": take, "settled": shares[i].Settled + take}).Error; err != nil {
			return 0, err
		}
		left -= take
	}
	backed := applied - left
	if left > 0 {
		// 批次总量不足以覆盖本次消耗。这是**账面不全**，不是代码写错：升级前的
		// 余额可能有一部分从来没有对应批次。此时若让整笔结算失败，配额扣减也会
		// 跟着回滚——用户已经用完上游，却一分钱不扣，那比账面对不上更糟。
		// 所以扣掉能扣的部分，差额由调用方落库（fund_batch_gap），日志只作补充。
		common.SysError(common.LogText("adty fund settlement short by %d quota for user=%d operation=%s: batches could not back the whole charge", left, userID, operationID))
	}
	// 收尾校验只覆盖「已经被批次支撑的那部分」：剩余占用必须正好等于 backed。
	// 不等说明前面某处漏记或重复释放，那是代码问题，必须让事务回滚。
	var total struct{ Held int64 }
	if err := tx.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held),0) as held").
		Where("user_id = ? AND operation_id = ?", userID, operationID).
		Scan(&total).Error; err != nil {
		return 0, err
	}
	if total.Held != backed {
		return 0, adtyFundBatchShortfallError(total.Held - backed)
	}
	return backed, nil
}

// deductAdtyFundBatchTx 扣一个批次的 remaining。扣不动就报错，让调用方整事务回滚。
func deductAdtyFundBatchTx(tx *gorm.DB, batchID, amount int64) error {
	if amount <= 0 {
		return nil
	}
	result := tx.Model(&AdtyFundBatch{}).
		Where("id = ? AND remaining >= ?", batchID, amount).
		Update("remaining", gorm.Expr("remaining - ?", amount))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAdtyFundBatchShortfall
	}
	return nil
}

// ReleaseAdtyFundAllocationsTx 释放一次未成交业务的预扣分摊。
//
// 只动分摊行，不动批次：这些额度从来没有从 remaining 里扣走过，所以没有需要
// 回补的东西。A.12.2.6 说的「预扣释放按原分摊返还」，返还的是那一份占用关系。
func ReleaseAdtyFundAllocationsTx(tx *gorm.DB, userID int, operationID string) error {
	if tx == nil || userID <= 0 || operationID == "" {
		return nil
	}
	// 表不存在 = 从来没有登记过任何占用，所以没东西可释放。
	//
	// 这一条不能省成「报错」：退款路径和它同事务，报错会把退款一起回滚，
	// 用户的钱就退不回去了。钱正确优先于账本完整——而这里两者其实一致，
	// 因为登记和释放用的是同一张表，登记不进去的时候也就没有占用。
	//
	// 真实场景是迁移没跑全的库（以及各自 AutoMigrate 的测试夹具）。
	if !tx.Migrator().HasTable(&AdtyFundAllocation{}) {
		return nil
	}
	return tx.Model(&AdtyFundAllocation{}).
		Where("user_id = ? AND operation_id = ? AND held > 0", userID, operationID).
		Update("held", 0).Error
}
