package model

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TopupRefund A20 退款记账行；request_id 唯一；系统只记账不调渠道真退。
type TopupRefund struct {
	Id             int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	OrderNo        string    `json:"order_no" gorm:"type:varchar(64);index;not null"`
	RequestId      string    `json:"request_id" gorm:"type:varchar(128);uniqueIndex;not null"`
	Amount         float64   `json:"amount" gorm:"type:decimal(18,6);not null"` // 本次退款美元
	Reason         string    `json:"reason" gorm:"type:text;not null"`
	OperatorId     int       `json:"operator_id" gorm:"not null;default:0"`
	BonusClawed    int64     `json:"bonus_clawed" gorm:"type:bigint;not null;default:0"`    // 实扣赠送（quota）
	BonusShortfall int64     `json:"bonus_shortfall" gorm:"type:bigint;not null;default:0"` // 应扣未扣差额（quota）
	AffClawed      int64     `json:"aff_clawed" gorm:"type:bigint;not null;default:0"`      // 作废+扣回返佣合计（quota）
	LedgerEntryIds string    `json:"ledger_entry_ids" gorm:"type:text"`                     // JSON []int64
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

func (TopupRefund) TableName() string { return "topup_refunds" }

// RefundTopupOrderResult 退款成功结果（含审计用字段）。
type RefundTopupOrderResult struct {
	Refund      *TopupRefund `json:"refund"`
	Order       *TopupOrder  `json:"order"`
	PaidClawed  int64        `json:"paid_clawed"`
	BonusClawed int64        `json:"bonus_clawed"`
	BonusShort  int64        `json:"bonus_shortfall"`
	AffClawed   int64        `json:"aff_clawed"`
	PaidBefore  int64        `json:"paid_before"`
	PaidAfter   int64        `json:"paid_after"`
	BonusBefore int64        `json:"bonus_before"`
	BonusAfter  int64        `json:"bonus_after"`
	AffBefore   int64        `json:"aff_before"`
	AffAfter    int64        `json:"aff_after"`
	LedgerIDs   []int64      `json:"ledger_entry_ids"`
	Refundable  float64      `json:"refundable_before"`
}

// RefundTopupOrder 管理员退款记账（A20）。
// paid/bonus 扣回经 AdjustQuota 写流水；aff 作废/扣回仍直写（AdjustQuota 不支持 aff）。
// 锁序：订单行 → user_bonus_wallets → users(id 升序) → aff_commissions 最后。
// amountUSD 为本次退款美元；request_id 幂等（ledger_idempotency + topup_refunds UNIQUE）。
func RefundTopupOrder(orderNo string, amountUSD float64, reason, requestId string, operatorId int) (*RefundTopupOrderResult, error) {
	orderNo = strings.TrimSpace(orderNo)
	reason = strings.TrimSpace(reason)
	requestId = strings.TrimSpace(requestId)
	if orderNo == "" {
		return nil, ErrTopupOrderNotFound
	}
	if requestId == "" {
		return nil, ErrRefundRequestIdRequired
	}
	if reason == "" {
		return nil, ErrRefundReasonRequired
	}
	if amountUSD <= 0 {
		return nil, ErrRefundAmountInvalid
	}

	idemKey := "refund:" + requestId
	var out *RefundTopupOrderResult

	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1) 锁订单行
		var order TopupOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopupOrderNotFound
			}
			return err
		}
		if order.Status != TopupOrderStatusPaid {
			return ErrRefundOrderNotPaid
		}

		// 2) 确定本人 + 佣金行上的邀请人（L3：不用用户当前 inviter_id），锁 bonus → users（id 升序）
		// 锁序仍是 users 先于 aff_commissions：先无锁窥探 InviterId，锁用户后再 FOR UPDATE 佣金行。
		userIds := []int{order.UserId}
		var peeksComm AffCommission
		if err := tx.Select("id", "inviter_id").Where("order_no = ?", orderNo).First(&peeksComm).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else if peeksComm.InviterId > 0 && peeksComm.InviterId != order.UserId {
			userIds = append(userIds, peeksComm.InviterId)
		}
		if err := lockRefundUsersInOrder(tx, userIds); err != nil {
			return err
		}

		// 3) 锁该单返佣行（最后）
		var comm AffCommission
		hasComm := false
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_no = ?", orderNo).First(&comm).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else {
			hasComm = true
		}
		inviterId := 0
		if hasComm && comm.InviterId > 0 && comm.InviterId != order.UserId {
			inviterId = comm.InviterId
		}

		// 4) ledger_idempotency：已存在 → 409
		var existIdem LedgerIdempotency
		if err := tx.Where("key = ?", idemKey).First(&existIdem).Error; err == nil {
			return ErrRefundIdempotent
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// 也查 topup_refunds.request_id
		var existRefund TopupRefund
		if err := tx.Where("request_id = ?", requestId).First(&existRefund).Error; err == nil {
			return ErrRefundIdempotent
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// 5) 可退金额 = min(该单实付 − 已退, 用户当前实充余额)
		paidUSD := topupRefundBaseUSD(&order)
		alreadyRefunded := order.RefundedAmount
		if alreadyRefunded < 0 {
			alreadyRefunded = 0
		}
		orderRemainUSD := paidUSD - alreadyRefunded
		if orderRemainUSD < 0 {
			orderRemainUSD = 0
		}

		var userRow User
		if err := tx.Select("id", "quota", "aff_quota").Where("id = ?", order.UserId).First(&userRow).Error; err != nil {
			return err
		}
		paidBalUSD := float64(userRow.Quota) / common.QuotaPerUnit
		refundable := orderRemainUSD
		if paidBalUSD < refundable {
			refundable = paidBalUSD
		}
		if amountUSD > refundable+1e-9 {
			return &RefundExceedsAvailableError{Available: refundable, Amount: amountUSD}
		}

		refundQuota, err := DollarsToQuota(amountUSD)
		if err != nil {
			return err
		}
		if refundQuota <= 0 {
			return ErrRefundAmountInvalid
		}
		if int64(userRow.Quota) < int64(refundQuota) {
			return &RefundExceedsAvailableError{Available: refundable, Amount: amountUSD}
		}

		paidTotalQuota := order.PaidCreditedQuota
		if paidTotalQuota <= 0 {
			pq, qerr := DollarsToQuota(paidUSD)
			if qerr != nil {
				return qerr
			}
			paidTotalQuota = int64(pq)
		}
		bonusTotalQuota := order.BonusCreditedQuota
		if bonusTotalQuota < 0 {
			bonusTotalQuota = 0
		}
		// 历史已 claw 合计（quota）；L2：查询失败必须中止，不能当 0 继续（否则末笔会多收回）
		prevBonus, prevAff, sumErr := sumPriorRefundClawbacks(tx, orderNo)
		if sumErr != nil {
			return sumErr
		}

		// 返佣总额先还原写入时的净值，再按订单基数抬回。只按当前剩余会少追。
		affTotalQuota := int64(0)
		if hasComm && (comm.Status == AffCommissionStatusFrozen ||
			comm.Status == AffCommissionStatusAvailable) {
			affTotalQuota = affRefundSnapshotQuota(comm.CommissionQuota, prevAff, paidUSD, comm.PaidAmountUSD)
		}

		isLast := (alreadyRefunded+amountUSD) >= paidUSD-1e-9 || amountUSD >= orderRemainUSD-1e-9

		var wallet UserBonusWallet
		if err := tx.Where("user_id = ?", order.UserId).First(&wallet).Error; err != nil {
			return err
		}
		commQuota := int64(0)
		commStatus := ""
		if hasComm {
			commQuota = comm.CommissionQuota
			commStatus = comm.Status
		}
		plan := planRefundClaws(int64(refundQuota), paidTotalQuota, bonusTotalQuota, affTotalQuota, prevBonus, prevAff, wallet.Balance, commQuota, isLast, commStatus, hasComm)
		bonusBefore := wallet.Balance
		paidBefore := int64(userRow.Quota)
		affBefore := int64(0)
		if inviterId > 0 {
			var inv User
			if err := tx.Select("aff_quota").Where("id = ?", inviterId).First(&inv).Error; err == nil {
				affBefore = int64(inv.AffQuota)
			}
		}

		ledgerIDs := make([]int64, 0, 4)

		// 6) 实充原额扣回（经 AdjustQuota；idem 仍由本函数 refund:request_id 统一写，不传 idemKey）
		paidAdj, err := AdjustQuota(tx, order.UserId, "paid", -int64(refundQuota),
			reason, "topup_refund", requestId, "", operatorId, "topup_refund")
		if err != nil {
			if errors.Is(err, ErrAdjustQuotaInsufficient) {
				return &RefundExceedsAvailableError{Available: refundable, Amount: amountUSD}
			}
			return err
		}
		paidEntry := paidAdj.Entry
		paidAfter := int(paidAdj.BalanceAfter)
		ledgerIDs = append(ledgerIDs, paidEntry.Id)

		// 7) 赠送按比例扣回；不够扣到 0，记 shortfall（先算 claw 再调 AdjustQuota，避免不足整单失败）
		bonusClaw := plan.BonusClaw
		bonusShort := plan.BonusShort
		bonusAfter := wallet.Balance
		if bonusClaw > 0 {
			bAdj, err := AdjustQuota(tx, order.UserId, "bonus", -bonusClaw,
				reason, "topup_refund", requestId, "", operatorId, "topup_refund_bonus_claw")
			if err != nil {
				if errors.Is(err, ErrAdjustQuotaInsufficient) {
					return errors.New("bonus balance changed concurrently")
				}
				return err
			}
			bonusAfter = bAdj.BalanceAfter
			ledgerIDs = append(ledgerIDs, bAdj.Entry.Id)
		} else {
			if err := tx.Model(&UserBonusWallet{}).Where("user_id = ?", order.UserId).Select("balance").Scan(&bonusAfter).Error; err != nil {
				return err
			}
		}

		// 8) 返佣扣回：冻结作废；已可提现从邀请人 aff 扣（可为负）
		affClawed := int64(0)
		affAfter := affBefore
		if plan.AffWant > 0 && hasComm {
			switch comm.Status {
			case AffCommissionStatusFrozen:
				// 按比例作废：削减 CommissionQuota；归零则 voided
				reduce := plan.AffReduce
				comm.CommissionQuota -= reduce
				affClawed = reduce
				if comm.CommissionQuota <= 0 {
					comm.CommissionQuota = 0
					now := time.Now()
					comm.Status = AffCommissionStatusVoided
					comm.VoidedBy = operatorId
					comm.VoidedAt = &now
				}
				if err := tx.Save(&comm).Error; err != nil {
					return err
				}
				// 冻结未入 aff_quota，写一条审计流水到邀请人（delta=0 也可；记负的「应作废」）
				if inviterId > 0 && reduce > 0 {
					aEntry, err := createLedgerEntryTx(tx, inviterId, "aff", "aff_refund_void", -reduce, affBefore,
						"refund void frozen commission", "topup_refund", requestId, operatorId)
					if err != nil {
						return err
					}
					ledgerIDs = append(ledgerIDs, aEntry.Id)
					affAfter = affBefore // 未动余额
				}
			case AffCommissionStatusAvailable:
				reduce := plan.AffReduce
				// 先收回未打出的提现，再扣可提现。已划进实充且还没被退款追回的部分，只扣实充，不连可提现再扣一次。
				// 仍不够的记成负的 aff_quota。不拿无关实充顶已提现现金。pay_unknown 不自动取消。
				afterAff, err := clawAvailableAffCommissionTx(tx, comm.InviterId, reduce, requestId, operatorId, &ledgerIDs)
				if err != nil {
					return err
				}
				affAfter = afterAff
				affClawed = reduce
				comm.CommissionQuota -= reduce
				if comm.CommissionQuota < 0 {
					comm.CommissionQuota = 0
				}
				if err := tx.Save(&comm).Error; err != nil {
					return err
				}
			}
		}

		// 9) 写 topup_refunds + 更新订单
		idsJSON, _ := json.Marshal(ledgerIDs)
		row := TopupRefund{
			OrderNo:        orderNo,
			RequestId:      requestId,
			Amount:         amountUSD,
			Reason:         reason,
			OperatorId:     operatorId,
			BonusClawed:    bonusClaw,
			BonusShortfall: bonusShort,
			AffClawed:      affClawed,
			LedgerEntryIds: string(idsJSON),
		}
		if err := tx.Create(&row).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrRefundIdempotent
			}
			return err
		}

		newRefunded := alreadyRefunded + amountUSD
		status := RefundStatusPartial
		if newRefunded >= paidUSD-1e-9 {
			newRefunded = paidUSD
			status = RefundStatusFull
		} else if newRefunded <= 0 {
			status = RefundStatusNone
		}
		if err := tx.Model(&TopupOrder{}).Where("id = ?", order.Id).Updates(map[string]any{
			"refunded_amount": newRefunded,
			"refund_status":   status,
		}).Error; err != nil {
			return err
		}
		order.RefundedAmount = newRefunded
		order.RefundStatus = status

		// 10) ledger_idempotency（与流水同事务）
		if err := tx.Create(&LedgerIdempotency{
			Key:     idemKey,
			EntryId: paidEntry.Id,
		}).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrRefundIdempotent
			}
			return err
		}

		_ = invalidateUserCache(order.UserId)
		if inviterId > 0 {
			_ = invalidateUserCache(inviterId)
		}

		out = &RefundTopupOrderResult{
			Refund:      &row,
			Order:       &order,
			PaidClawed:  int64(refundQuota),
			BonusClawed: bonusClaw,
			BonusShort:  bonusShort,
			AffClawed:   affClawed,
			PaidBefore:  paidBefore,
			PaidAfter:   int64(paidAfter),
			BonusBefore: bonusBefore,
			BonusAfter:  bonusAfter,
			AffBefore:   affBefore,
			AffAfter:    affAfter,
			LedgerIDs:   ledgerIDs,
			Refundable:  refundable,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// sumPriorRefundClawbacks 汇总该单历史退款已收回的赠送/返佣（quota）。
// 查询失败必须向上返回，调用方不得按 0 继续（L2）。
func sumPriorRefundClawbacks(tx *gorm.DB, orderNo string) (prevBonus, prevAff int64, err error) {
	row := struct {
		Bonus int64 `gorm:"column:bonus"`
		Aff   int64 `gorm:"column:aff"`
	}{}
	if err := tx.Model(&TopupRefund{}).
		Select("COALESCE(SUM(bonus_clawed),0) as bonus, COALESCE(SUM(aff_clawed),0) as aff").
		Where("order_no = ?", orderNo).Scan(&row).Error; err != nil {
		return 0, 0, err
	}
	return row.Bonus, row.Aff, nil
}

// lockRefundUsersInOrder 锁序：各用户 user_bonus_wallets → users，均按 id 升序。
func lockRefundUsersInOrder(tx *gorm.DB, userIds []int) error {
	seen := map[int]struct{}{}
	var ids []int
	for _, id := range userIds {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	for _, id := range ids {
		if err := EnsureUserBonusWallet(tx, id); err != nil {
			return err
		}
		var w UserBonusWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", id).First(&w).Error; err != nil {
			return err
		}
	}
	for _, id := range ids {
		var u User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "quota", "aff_quota", "inviter_id").Where("id = ?", id).First(&u).Error; err != nil {
			return err
		}
	}
	return nil
}

// QuotaToDollars 内部 quota → 美元（展示/验收用）。
func QuotaToDollars(q int64) float64 {
	if common.QuotaPerUnit <= 0 {
		return 0
	}
	return float64(q) / common.QuotaPerUnit
}

// unrecoveredTransferredAffQuotaTx 还没被退款追回的划转实充。
// aff_transfer 是正数，aff_refund_claw_paid 是负数，合计就是还能从实充追回的额度。
func unrecoveredTransferredAffQuotaTx(tx *gorm.DB, userId int) (int64, error) {
	var sum int64
	err := tx.Model(&LedgerEntry{}).
		Where("user_id = ? AND account = ? AND kind IN ?", userId, "paid", []string{"aff_transfer", "aff_refund_claw_paid"}).
		Select("COALESCE(SUM(delta), 0)").
		Scan(&sum).Error
	if err != nil {
		return 0, err
	}
	if sum < 0 {
		return 0, nil
	}
	return sum, nil
}

// clawAvailableAffCommissionTx 追回已解冻返佣。调用方必须已经锁住邀请人。
// 返回扣完后的 aff_quota。aff 流水只记可提现账户的实际变动，划转追回另记实充流水。
func clawAvailableAffCommissionTx(tx *gorm.DB, inviterId int, reduce int64, requestId string, operatorId int, ledgerIDs *[]int64) (int64, error) {
	if inviterId <= 0 {
		return 0, errors.New("invalid inviter")
	}
	if reduce <= 0 {
		var aff int
		if err := tx.Model(&User{}).Where("id = ?", inviterId).Select("aff_quota").Scan(&aff).Error; err != nil {
			return 0, err
		}
		return int64(aff), nil
	}
	if err := reclaimInFlightAffWithdrawalsTx(tx, inviterId, reduce, operatorId, "refund claw reclaimed unpaid withdrawal"); err != nil {
		return 0, err
	}
	var inv User
	if err := tx.Select("id", "quota", "aff_quota").Where("id = ?", inviterId).First(&inv).Error; err != nil {
		return 0, err
	}
	fromAff := int64(0)
	if inv.AffQuota > 0 {
		fromAff = int64(inv.AffQuota)
		if fromAff > reduce {
			fromAff = reduce
		}
	}
	short := reduce - fromAff
	fromPaid := int64(0)
	if short > 0 {
		recoverable, err := unrecoveredTransferredAffQuotaTx(tx, inviterId)
		if err != nil {
			return 0, err
		}
		fromPaid = short
		if fromPaid > recoverable {
			fromPaid = recoverable
		}
		if fromPaid > int64(inv.Quota) {
			fromPaid = int64(inv.Quota)
		}
		if fromPaid < 0 {
			fromPaid = 0
		}
	}
	fromNegative := reduce - fromAff - fromPaid
	affDelta := fromAff + fromNegative
	if affDelta > 0 {
		if err := tx.Model(&User{}).Where("id = ?", inviterId).
			Update("aff_quota", gorm.Expr("aff_quota - ?", affDelta)).Error; err != nil {
			return 0, err
		}
	}
	var afterAff int
	if err := tx.Model(&User{}).Where("id = ?", inviterId).Select("aff_quota").Scan(&afterAff).Error; err != nil {
		return 0, err
	}
	if affDelta > 0 {
		entry, err := createLedgerEntryTx(tx, inviterId, "aff", "aff_refund_claw", -affDelta, int64(afterAff),
			"refund claw available commission", "topup_refund", requestId, operatorId)
		if err != nil {
			return 0, err
		}
		if ledgerIDs != nil {
			*ledgerIDs = append(*ledgerIDs, entry.Id)
		}
	}
	if fromPaid > 0 {
		adj, err := AdjustQuota(tx, inviterId, "paid", -fromPaid,
			"refund claw transferred commission", "topup_refund", requestId, "", operatorId, "aff_refund_claw_paid")
		if err != nil {
			return 0, err
		}
		if ledgerIDs != nil && adj != nil && adj.Entry != nil {
			*ledgerIDs = append(*ledgerIDs, adj.Entry.Id)
		}
	}
	return int64(afterAff), nil
}

type refundClawPlan struct {
	BonusWant  int64
	AffWant    int64
	BonusClaw  int64
	BonusShort int64
	AffReduce  int64
	AffAction  string
}

// topupRefundBaseUSD 与入账一致：先用实付入账额，没有才用支付额。预览必须走同一基数。
func topupRefundBaseUSD(order *TopupOrder) float64 {
	if order == nil {
		return 0
	}
	if order.CreditAmountUSD > 0 {
		return order.CreditAmountUSD
	}
	return order.PaidAmountUSD
}

// affRefundSnapshotQuota 把「当前剩余 + 已追回」抬回整单佣金。
// 写入时 PaidAmountUSD 小于订单基数，说明当时已经扣过退款，或自邀放行按剩余缩过。
// 只拿当前剩余当原额，后面的部分退款会少追。基数相同则与旧公式一致。
func affRefundSnapshotQuota(commQuota, prevAff int64, orderBaseUSD, basisPaidUSD float64) int64 {
	basis := commQuota + prevAff
	if basis < 0 {
		basis = 0
	}
	if basis == 0 || basisPaidUSD <= 0 || orderBaseUSD <= basisPaidUSD+1e-6 {
		return basis
	}
	orderMicro := usdToMicro(orderBaseUSD)
	paidMicro := usdToMicro(basisPaidUSD)
	if paidMicro <= 0 || orderMicro <= paidMicro {
		return basis
	}
	lifted := mulDivRound(basis, orderMicro, paidMicro)
	if lifted < basis {
		return basis
	}
	return lifted
}

func usdToMicro(v float64) int64 {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	micro := math.Round(v * 1_000_000)
	if micro >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(micro)
}

func mulDivRound(a, b, c int64) int64 {
	if a <= 0 || b <= 0 || c <= 0 {
		return 0
	}
	num := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	den := big.NewInt(c)
	num.Add(num, new(big.Int).Div(new(big.Int).Set(den), big.NewInt(2)))
	num.Div(num, den)
	if !num.IsInt64() {
		return math.MaxInt64
	}
	return num.Int64()
}

// planRefundClaws is the only claw formula. Refund writes and the preview both call it.
func planRefundClaws(refundQuota, paidTotal, bonusTotal, affTotal, prevBonus, prevAff, walletBalance, commQuota int64, isLast bool, commStatus string, hasComm bool) refundClawPlan {
	var bonusWant, affWant int64
	if paidTotal > 0 {
		if isLast {
			bonusWant = bonusTotal - prevBonus
			if bonusWant < 0 {
				bonusWant = 0
			}
			affWant = affTotal - prevAff
			if affWant < 0 {
				affWant = 0
			}
		} else {
			bonusWant = refundQuota * bonusTotal / paidTotal
			affWant = refundQuota * affTotal / paidTotal
		}
	}
	bonusClaw := bonusWant
	if bonusClaw > walletBalance {
		bonusClaw = walletBalance
	}
	if bonusClaw < 0 {
		bonusClaw = 0
	}
	bonusShort := bonusWant - bonusClaw
	if bonusShort < 0 {
		bonusShort = 0
	}
	affReduce := int64(0)
	affAction := "none"
	if hasComm && affWant > 0 {
		switch commStatus {
		case AffCommissionStatusFrozen, AffCommissionStatusAvailable:
			affReduce = affWant
			if affReduce > commQuota {
				affReduce = commQuota
			}
			if commStatus == AffCommissionStatusFrozen {
				affAction = "void"
			} else {
				affAction = "claw"
			}
		}
	}
	return refundClawPlan{
		BonusWant: bonusWant, AffWant: affWant,
		BonusClaw: bonusClaw, BonusShort: bonusShort,
		AffReduce: affReduce, AffAction: affAction,
	}
}

// RefundClawPreview is a read-only quote. Submit still settles against the locked balances.
type RefundClawPreview struct {
	AmountUSD           float64 `json:"amount_usd"`
	PaidClawQuota       int64   `json:"paid_claw_quota"`
	PaidClawUSD         float64 `json:"paid_claw_usd"`
	BonusClawQuota      int64   `json:"bonus_claw_quota"`
	BonusClawUSD        float64 `json:"bonus_claw_usd"`
	BonusShortfallQuota int64   `json:"bonus_shortfall_quota"`
	BonusShortfallUSD   float64 `json:"bonus_shortfall_usd"`
	AffClawQuota        int64   `json:"aff_claw_quota"`
	AffClawUSD          float64 `json:"aff_claw_usd"`
	AffAction           string  `json:"aff_action"`
	RefundableUSD       float64 `json:"refundable_usd"`
}

// RefundPreviewEcho is the preview the operator saw. The refund handler must match it before writing.
type RefundPreviewEcho struct {
	AmountUSD         float64 `json:"amount_usd"`
	PaidClawUSD       float64 `json:"paid_claw_usd"`
	BonusClawUSD      float64 `json:"bonus_claw_usd"`
	BonusShortfallUSD float64 `json:"bonus_shortfall_usd"`
	AffClawUSD        float64 `json:"aff_claw_usd"`
	AffAction         string  `json:"aff_action"`
}

func quotaAmountToUSD(q int64) float64 {
	if common.QuotaPerUnit == 0 {
		return 0
	}
	return float64(q) / common.QuotaPerUnit
}

// PreviewTopupRefund quotes the same claw numbers RefundTopupOrder would write. It does not write.
func PreviewTopupRefund(orderNo string, amountUSD float64) (*RefundClawPreview, error) {
	if strings.TrimSpace(orderNo) == "" {
		return nil, ErrTopupOrderNotFound
	}
	if amountUSD <= 0 {
		return nil, ErrRefundAmountInvalid
	}
	order, err := GetTopupOrderByNo(orderNo)
	if err != nil {
		return nil, err
	}
	if order.Status != TopupOrderStatusPaid {
		return nil, ErrTopupOrderStatusInvalid
	}
	paidUSD := topupRefundBaseUSD(order)
	already := order.RefundedAmount
	orderRemain := paidUSD - already
	if orderRemain < 0 {
		orderRemain = 0
	}
	if amountUSD > orderRemain+1e-9 {
		return nil, ErrRefundExceedsAvailable
	}
	var user User
	if err := DB.Select("id", "quota").First(&user, order.UserId).Error; err != nil {
		return nil, err
	}
	var wallet UserBonusWallet
	if err := DB.Where("user_id = ?", order.UserId).First(&wallet).Error; err != nil {
		return nil, err
	}
	paidBalUSD := quotaAmountToUSD(int64(user.Quota))
	refundable := orderRemain
	if paidBalUSD+1e-9 < refundable {
		refundable = paidBalUSD
	}
	if refundable < 0 {
		refundable = 0
	}
	if amountUSD > refundable+1e-9 {
		return nil, ErrRefundExceedsAvailable
	}
	refundQuota, err := DollarsToQuota(amountUSD)
	if err != nil {
		return nil, err
	}
	paidTotalQuota := order.PaidCreditedQuota
	if paidTotalQuota <= 0 {
		if q, qerr := DollarsToQuota(paidUSD); qerr == nil {
			paidTotalQuota = int64(q)
		}
	}
	bonusTotalQuota := order.BonusCreditedQuota
	if bonusTotalQuota < 0 {
		bonusTotalQuota = 0
	}
	prevBonus, prevAff, err := sumPriorRefundClawbacks(DB, order.OrderNo)
	if err != nil {
		return nil, err
	}
	var comm AffCommission
	hasComm := DB.Where("order_no = ?", order.OrderNo).First(&comm).Error == nil
	affTotal := int64(0)
	commQuota := int64(0)
	commStatus := ""
	if hasComm && (comm.Status == AffCommissionStatusFrozen || comm.Status == AffCommissionStatusAvailable) {
		affTotal = affRefundSnapshotQuota(comm.CommissionQuota, prevAff, paidUSD, comm.PaidAmountUSD)
		commQuota = comm.CommissionQuota
		commStatus = comm.Status
	}
	isLast := (already+amountUSD) >= paidUSD-1e-9 || amountUSD >= orderRemain-1e-9
	plan := planRefundClaws(int64(refundQuota), paidTotalQuota, bonusTotalQuota, affTotal, prevBonus, prevAff, wallet.Balance, commQuota, isLast, commStatus, hasComm)
	return &RefundClawPreview{
		AmountUSD:           amountUSD,
		PaidClawQuota:       int64(refundQuota),
		PaidClawUSD:         quotaAmountToUSD(int64(refundQuota)),
		BonusClawQuota:      plan.BonusClaw,
		BonusClawUSD:        quotaAmountToUSD(plan.BonusClaw),
		BonusShortfallQuota: plan.BonusShort,
		BonusShortfallUSD:   quotaAmountToUSD(plan.BonusShort),
		AffClawQuota:        plan.AffReduce,
		AffClawUSD:          quotaAmountToUSD(plan.AffReduce),
		AffAction:           plan.AffAction,
		RefundableUSD:       refundable,
	}, nil
}

// RefundPreviewMatches reports whether the submitted preview is still the current quote.
func RefundPreviewMatches(current *RefundClawPreview, echo RefundPreviewEcho) bool {
	if current == nil {
		return false
	}
	return refundUSDNear(current.AmountUSD, echo.AmountUSD) &&
		refundUSDNear(current.PaidClawUSD, echo.PaidClawUSD) &&
		refundUSDNear(current.BonusClawUSD, echo.BonusClawUSD) &&
		refundUSDNear(current.BonusShortfallUSD, echo.BonusShortfallUSD) &&
		refundUSDNear(current.AffClawUSD, echo.AffClawUSD) &&
		current.AffAction == echo.AffAction
}

func refundUSDNear(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= 1e-6
}
