package service

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 三种提现（任务 6.4）：转入余额（立即完成）、USDT（人工审核）、支付宝/微信收款码（人工审核，汇率提交时查库锁定）。

var (
	ErrReferralRateChanged   = errors.New("rate_changed")
	ErrReferralRateUnset     = errors.New("withdrawal exchange rate is not set")
	tronAddressPattern       = regexp.MustCompile(`^T[1-9A-HJ-NP-Za-km-z]{33}$`)
	evmAddressPattern        = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	referralWithdrawNetworks = map[string]*regexp.Regexp{
		"TRC20": tronAddressPattern, "BEP20": evmAddressPattern,
		"POLYGON": evmAddressPattern, "XLAYER": evmAddressPattern,
	}
)

// ReferralWithdrawalCreateInput 是一笔提现申请。
type ReferralWithdrawalCreateInput struct {
	UserId          int
	Amount          decimal.Decimal
	AccountType     string // balance | usdt | alipay | wechat
	AccountNetwork  string // TRC20 | BEP20 | POLYGON | XLAYER（只有 usdt 用）
	AccountNo       string // USDT 地址
	AccountName     string // 收款人姓名（支付宝、微信必填）
	QRImageURL      string
	ExpectedCNYRate decimal.Decimal
	IdempotencyKey  string
}

// ReferralRateChange 是 rate_changed 时返回给前端的新汇率。
type ReferralRateChange struct {
	Rate      string `json:"rate"`
	CNYAmount string `json:"cny_amount"`
	UpdatedAt int64  `json:"updated_at"`
}

// RateChangedError 带上新的汇率和人民币金额。
type RateChangedError struct{ Change ReferralRateChange }

func (e *RateChangedError) Error() string { return ErrReferralRateChanged.Error() }
func (e *RateChangedError) Unwrap() error { return ErrReferralRateChanged }

// CurrentWithdrawCNYRate 直接查库读提现汇率：行不存在、解析不了或不大于 0 都算未设置。
func CurrentWithdrawCNYRate() (rate decimal.Decimal, updatedAt int64, err error) {
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var readErr error
		rate, updatedAt, readErr = currentWithdrawCNYRateTx(tx)
		return readErr
	})
	return rate, updatedAt, err
}

// currentWithdrawCNYRateTx holds the rate row until the withdrawal or settings
// transaction commits, so an accepted application has one durable rate snapshot.
func currentWithdrawCNYRateTx(tx *gorm.DB) (decimal.Decimal, int64, error) {
	raw, ok, err := model.ReadOptionForUpdate(tx, adty_setting.ReferralKeyWithdrawCNYRate)
	if err != nil {
		return decimal.Zero, 0, err
	}
	if !ok {
		return decimal.Zero, 0, ErrReferralRateUnset
	}
	rate, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !rate.IsPositive() {
		return decimal.Zero, 0, ErrReferralRateUnset
	}
	updatedRaw, _, err := model.ReadOptionForUpdate(tx, adty_setting.ReferralKeyWithdrawCNYRateUpdatedAt)
	if err != nil {
		return decimal.Zero, 0, err
	}
	updated, _ := decimal.NewFromString(strings.TrimSpace(updatedRaw))
	return rate, updated.IntPart(), nil
}

// ValidateUSDTAddress 按链校验地址：TRC20 是 T 开头的 base58；BEP20、Polygon 和 X Layer 是 0x 开头的 40 位十六进制。
func ValidateUSDTAddress(network, address string) bool {
	pattern, ok := referralWithdrawNetworks[strings.ToUpper(network)]
	return ok && pattern.MatchString(strings.TrimSpace(address))
}

// validReferralQR 只接受本站上传的收款码路径，拒绝 javascript: 和外部链接。
func validReferralQR(raw string) bool {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "/api/referral-assets/") || strings.Contains(raw, "://") {
		return false
	}
	return !strings.ContainsAny(raw, "\\\r\n\t ")
}

// CreateWithdrawal 建提现单。所有去向都在事务里锁佣金账户行；防超提靠条件更新 available_amount >= ?；
// 幂等键先查后写（审阅重点 3、10）。
func (s *ReferralService) CreateWithdrawal(input ReferralWithdrawalCreateInput) (*model.ReferralWithdrawal, error) {
	cfg := referralSetting()
	amount := input.Amount.Round(2)
	if !amount.IsPositive() {
		return nil, common.NewMessage("Enter a withdrawal amount")
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" || len(key) > 128 {
		return nil, common.NewMessage("Invalid idempotency key")
	}
	withdrawal := &model.ReferralWithdrawal{
		UserId: input.UserId, SettlementCurrency: "USD", Amount: moneyF(amount), NetAmount: moneyF(amount),
		AccountType: input.AccountType, SubmittedAt: time.Now().Unix(), Status: model.ReferralWithdrawalStatusPending,
	}
	switch input.AccountType {
	case model.ReferralWithdrawToBalance:
		withdrawal.Status = model.ReferralWithdrawalStatusPaid
		withdrawal.PaidAt = withdrawal.SubmittedAt
	case model.ReferralWithdrawToUSDT:
		if cfg.MinWithdrawUSD == nil || cfg.USDTWithdrawFee == nil {
			return nil, ErrReferralWithdrawUnavailable
		}
		if amount.LessThan(decimal.NewFromFloat(*cfg.MinWithdrawUSD)) {
			return nil, common.NewMessage("The minimum withdrawal is ${{amount}}", map[string]any{"amount": *cfg.MinWithdrawUSD})
		}
		network := strings.ToUpper(strings.TrimSpace(input.AccountNetwork))
		if !ValidateUSDTAddress(network, input.AccountNo) {
			return nil, common.NewMessage("The address does not match the {{network}} network", map[string]any{"network": network})
		}
		fee := decimal.NewFromFloat(*cfg.USDTWithdrawFee).Round(2)
		net := amount.Sub(fee)
		if !net.IsPositive() {
			return nil, common.NewMessage("The amount must be larger than the network fee")
		}
		withdrawal.AccountNetwork = network
		withdrawal.AccountNo = strings.TrimSpace(input.AccountNo)
		withdrawal.FeeAmount = moneyF(fee)
		withdrawal.NetAmount = moneyF(net)
	case model.ReferralWithdrawToAlipay, model.ReferralWithdrawToWechat:
		if cfg.MinWithdrawUSD == nil {
			return nil, ErrReferralWithdrawUnavailable
		}
		if amount.LessThan(decimal.NewFromFloat(*cfg.MinWithdrawUSD)) {
			return nil, common.NewMessage("The minimum withdrawal is ${{amount}}", map[string]any{"amount": *cfg.MinWithdrawUSD})
		}
		if !validReferralQR(input.QRImageURL) || strings.TrimSpace(input.AccountName) == "" {
			return nil, common.NewMessage("Upload the payment QR code and enter the payee name")
		}
		withdrawal.AccountName = strings.TrimSpace(input.AccountName)
		withdrawal.QRImageURL = strings.TrimSpace(input.QRImageURL)
	default:
		return nil, common.NewMessage("Unsupported withdrawal method")
	}

	var result *model.ReferralWithdrawal
	var creditQuota int64
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		affiliate, err := s.lockAffiliateByUserTx(tx, input.UserId)
		if err != nil {
			return err
		}
		if !affiliate.WithdrawalEnabled || affiliate.Status != model.ReferralAffiliateStatusApproved {
			return ErrReferralWithdrawBlocked
		}
		var lockedCommissions []model.ReferralCommission
		if err := model.AdtyLockForUpdate(tx).Where("affiliate_id = ? AND status IN ?", affiliate.Id,
			[]string{model.ReferralCommissionStatusAvailable, model.ReferralCommissionStatusFrozen}).
			Order("id").Find(&lockedCommissions).Error; err != nil {
			return err
		}
		account, err := lockReferralAccountTx(tx, affiliate.Id)
		if err != nil {
			return err
		}
		var existing []model.ReferralWithdrawal
		if err := tx.Where("idempotency_key = ?", key).Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > 0 {
			if existing[0].UserId != input.UserId || existing[0].AccountType != input.AccountType || !moneyDec(existing[0].Amount).Equal(amount) {
				return model.ErrAdtyIdempotencyConflict
			}
			result = &existing[0]
			return nil
		}
		if input.AccountType == model.ReferralWithdrawToAlipay || input.AccountType == model.ReferralWithdrawToWechat {
			rate, updatedAt, err := currentWithdrawCNYRateTx(tx)
			if err != nil {
				return err
			}
			cny := amount.Mul(rate).Round(2)
			if !rate.Equal(input.ExpectedCNYRate) {
				return &RateChangedError{Change: ReferralRateChange{Rate: rate.String(), CNYAmount: cny.StringFixed(2), UpdatedAt: updatedAt}}
			}
			withdrawal.CNYRate = rate.String()
			withdrawal.CNYAmount = cny.StringFixed(2)
		}
		var closing int64
		if err := tx.Model(&model.AdtySite{}).Where("owner_user_id = ? AND status = ?", input.UserId, model.AdtySiteStatusClosing).Count(&closing).Error; err != nil {
			return err
		}
		if closing > 0 {
			return common.NewMessage("Withdrawals are paused while the site is closing")
		}
		update := tx.Model(&model.ReferralCommissionAccount{}).Where("id = ? AND available_amount >= ?", account.Id, moneyF(amount))
		if withdrawal.AccountType == model.ReferralWithdrawToBalance {
			update = update.Updates(map[string]any{
				"available_amount": gorm.Expr("available_amount - ?", moneyF(amount)),
				"withdrawn_amount": gorm.Expr("withdrawn_amount + ?", moneyF(amount)),
			})
		} else {
			update = update.Updates(map[string]any{
				"available_amount": gorm.Expr("available_amount - ?", moneyF(amount)),
				"frozen_amount":    gorm.Expr("frozen_amount + ?", moneyF(amount)),
			})
		}
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return ErrReferralInsufficient
		}
		withdrawal.AffiliateId = affiliate.Id
		withdrawal.IdempotencyKey = &key
		if err := tx.Create(withdrawal).Error; err != nil {
			return err
		}
		if err := s.allocateWithdrawalItemsTx(tx, affiliate.Id, withdrawal.Id, amount); err != nil {
			return err
		}
		ledgerType, deltaFrozen, deltaWithdrawn := "withdraw_freeze", amount, decimal.Zero
		if withdrawal.AccountType == model.ReferralWithdrawToBalance {
			ledgerType, deltaFrozen, deltaWithdrawn = "withdraw_to_balance", decimal.Zero, amount
			if err := s.markWithdrawalItemsPaidTx(tx, withdrawal.Id); err != nil {
				return err
			}
			creditQuota = amount.Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Floor().IntPart()
			before, err := model.AdtyCreditUserQuotaTx(tx, input.UserId, creditQuota)
			if err != nil {
				return err
			}
			ref := fmt.Sprintf("referral-withdrawal-%d", withdrawal.Id)
			if err := model.InsertAdtyLedgerTx(tx, &model.AdtyBalanceLedger{
				UserId: input.UserId, DeltaQuota: creditQuota, BalanceBefore: before, BalanceAfter: before + creditQuota,
				Type: model.AdtyLedgerTypeCommissionTransfer, RefType: "referral_withdrawal", RefId: ref,
				Reason: "commission transferred to balance", IdempotencyKey: &ref,
			}); err != nil {
				return err
			}
		}
		if err := addReferralLedgerTx(tx, &model.ReferralCommissionLedger{
			AffiliateId: affiliate.Id, UserId: input.UserId, WithdrawalId: withdrawal.Id, Type: ledgerType,
			RefType: "withdrawal", RefId: fmt.Sprint(withdrawal.Id), ExternalRefId: fmt.Sprintf("withdraw:%d", withdrawal.Id),
			DeltaAvailable: moneyF(amount.Neg()), DeltaFrozen: moneyF(deltaFrozen), DeltaWithdrawn: moneyF(deltaWithdrawn),
			Remark: input.AccountType,
		}); err != nil {
			return err
		}
		result = withdrawal
		return nil
	})
	if err != nil {
		return nil, err
	}
	if creditQuota > 0 && result == withdrawal {
		model.AdtySyncCreditUserQuotaCache(input.UserId, creditQuota, "referral commission to balance")
		model.AdtyRecordUserLog(input.UserId, common.NewMessage("Commission transferred to balance: ${{amount}}", map[string]any{"amount": amount.StringFixed(2)}), "")
	}
	return result, nil
}

func (s *ReferralService) lockAffiliateByUserTx(tx *gorm.DB, userId int) (*model.ReferralAffiliate, error) {
	var affiliate model.ReferralAffiliate
	if err := model.AdtyLockForUpdate(tx).Where("user_id = ?", userId).First(&affiliate).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReferralNotAffiliate
		}
		return nil, err
	}
	return &affiliate, nil
}

// CancelWithdrawal 在提交后 cancel_window_minutes 内撤回待审核的提现，撤回即解冻。
func (s *ReferralService) CancelWithdrawal(withdrawalId, userId int, now int64) error {
	window := int64(max(referralSetting().CancelWindowMinutes, 0)) * 60
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var withdrawal model.ReferralWithdrawal
		if err := model.AdtyLockForUpdate(tx).Where("id = ? AND user_id = ?", withdrawalId, userId).First(&withdrawal).Error; err != nil {
			return err
		}
		if withdrawal.Status != model.ReferralWithdrawalStatusPending {
			return common.NewMessage("Only pending withdrawals can be canceled")
		}
		if now-withdrawal.SubmittedAt > window {
			return common.NewMessage("The cancel window has passed")
		}
		return s.releaseWithdrawalTx(tx, &withdrawal, model.ReferralWithdrawalStatusCanceled, userId, "user canceled", now)
	})
}

// lockWithdrawalCommissionRowsTx 先按佣金 id 锁住这笔提现占用的佣金。
// 退款扣回是先锁佣金再锁账户；这里保持同一顺序，避免打款或驳回和退款互相锁死。
func lockWithdrawalCommissionRowsTx(tx *gorm.DB, withdrawalId int) error {
	var ids []int
	if err := tx.Model(&model.ReferralWithdrawalItem{}).
		Where("withdrawal_id = ? AND commission_id > 0", withdrawalId).
		Order("commission_id").
		Pluck("commission_id", &ids).Error; err != nil {
		return err
	}
	seen := map[int]struct{}{}
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		var row model.ReferralCommission
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", id).First(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// releaseWithdrawalTx 驳回或撤回：冻结的金额回到可提现，明细释放。
func (s *ReferralService) releaseWithdrawalTx(tx *gorm.DB, withdrawal *model.ReferralWithdrawal, status string, actor int, reason string, now int64) error {
	if err := lockWithdrawalCommissionRowsTx(tx, withdrawal.Id); err != nil {
		return err
	}
	account, err := lockReferralAccountTx(tx, withdrawal.AffiliateId)
	if err != nil {
		return err
	}
	amount := moneyDec(withdrawal.Amount)
	updates := map[string]any{"status": status, "reject_reason": truncateText(reason, 1000)}
	if status == model.ReferralWithdrawalStatusCanceled {
		updates["canceled_at"], updates["canceled_by"] = now, actor
	} else {
		updates["rejected_at"], updates["rejected_by"] = now, actor
	}
	if err := tx.Model(&model.ReferralWithdrawal{}).Where("id = ?", withdrawal.Id).Updates(updates).Error; err != nil {
		return err
	}
	if err := s.releaseWithdrawalItemsTx(tx, withdrawal.Id); err != nil {
		return err
	}
	return AdjustAccountTx(tx, account, decimal.Zero, amount, amount.Neg(), decimal.Zero, model.ReferralCommissionLedger{
		WithdrawalId: withdrawal.Id, Type: "withdraw_release", RefType: "withdrawal", RefId: fmt.Sprint(withdrawal.Id),
		ExternalRefId: fmt.Sprintf("release:%d", withdrawal.Id), Remark: status,
	})
}

// ReviewWithdrawal 审核通过或驳回（驳回会解冻并发邮件）。
func (s *ReferralService) ReviewWithdrawal(withdrawalId, adminId int, approve bool, reason, ip, ua string) error {
	now := time.Now().Unix()
	var notifyUser int
	var withdrawalCopy model.ReferralWithdrawal
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var withdrawal model.ReferralWithdrawal
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", withdrawalId).First(&withdrawal).Error; err != nil {
			return err
		}
		if withdrawal.Status != model.ReferralWithdrawalStatusPending {
			return common.NewMessage("Only pending withdrawals can be reviewed")
		}
		if approve {
			if err := tx.Model(&model.ReferralWithdrawal{}).Where("id = ?", withdrawal.Id).Updates(map[string]any{
				"status": model.ReferralWithdrawalStatusApproved, "approved_at": now, "reviewed_by": adminId,
			}).Error; err != nil {
				return err
			}
			return recordReferralAuditTx(tx, "withdrawal_approve", withdrawal.UserId, withdrawal.AffiliateId, adminId, reason, ip, ua, nil, map[string]any{"id": withdrawal.Id})
		}
		if err := s.releaseWithdrawalTx(tx, &withdrawal, model.ReferralWithdrawalStatusRejected, adminId, reason, now); err != nil {
			return err
		}
		notifyUser = withdrawal.UserId
		withdrawalCopy = withdrawal
		return recordReferralAuditTx(tx, "withdrawal_reject", withdrawal.UserId, withdrawal.AffiliateId, adminId, reason, ip, ua, nil, map[string]any{"id": withdrawal.Id})
	})
	if err == nil && notifyUser > 0 {
		sendReferralWithdrawalEmail(notifyUser, &withdrawalCopy, false, reason)
	}
	return err
}

// MarkWithdrawalPaid 标记已打款：冻结转已提现，明细标 withdrawn，佣金推到 paid；USDT 要填交易哈希。
func (s *ReferralService) MarkWithdrawalPaid(withdrawalId, adminId int, txnNo, proofURL, note, ip, ua string) error {
	now := time.Now().Unix()
	var withdrawalCopy model.ReferralWithdrawal
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var withdrawal model.ReferralWithdrawal
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", withdrawalId).First(&withdrawal).Error; err != nil {
			return err
		}
		if withdrawal.Status != model.ReferralWithdrawalStatusApproved {
			return common.NewMessage("This withdrawal cannot be marked as paid")
		}
		if withdrawal.AccountType == model.ReferralWithdrawToUSDT && strings.TrimSpace(txnNo) == "" {
			return common.NewMessage("Enter the transaction hash")
		}
		if err := lockWithdrawalCommissionRowsTx(tx, withdrawal.Id); err != nil {
			return err
		}
		account, err := lockReferralAccountTx(tx, withdrawal.AffiliateId)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.ReferralWithdrawal{}).Where("id = ?", withdrawal.Id).Updates(map[string]any{
			"status": model.ReferralWithdrawalStatusPaid, "paid_at": now, "payment_txn_no": strings.TrimSpace(txnNo),
			"payment_proof_url": strings.TrimSpace(proofURL), "admin_note": truncateText(note, 1000), "reviewed_by": adminId,
		}).Error; err != nil {
			return err
		}
		if err := s.markWithdrawalItemsPaidTx(tx, withdrawal.Id); err != nil {
			return err
		}
		amount := moneyDec(withdrawal.Amount)
		if err := AdjustAccountTx(tx, account, decimal.Zero, decimal.Zero, amount.Neg(), amount, model.ReferralCommissionLedger{
			WithdrawalId: withdrawal.Id, Type: "withdraw_paid", RefType: "withdrawal", RefId: fmt.Sprint(withdrawal.Id),
			ExternalRefId: fmt.Sprintf("paid:%d", withdrawal.Id), Remark: strings.TrimSpace(txnNo), Operator: fmt.Sprint(adminId),
		}); err != nil {
			return err
		}
		withdrawalCopy = withdrawal
		withdrawalCopy.PaymentTxnNo = strings.TrimSpace(txnNo)
		return recordReferralAuditTx(tx, "withdrawal_pay", withdrawal.UserId, withdrawal.AffiliateId, adminId, note, ip, ua, nil, map[string]any{"id": withdrawal.Id, "txn": txnNo})
	})
	if err == nil {
		sendReferralWithdrawalEmail(withdrawalCopy.UserId, &withdrawalCopy, true, "")
	}
	return err
}

// sendReferralWithdrawalEmail 给用户发打款或驳回邮件，按账号语言。失败只记日志。
var sendReferralWithdrawalEmail = func(userId int, withdrawal *model.ReferralWithdrawal, paid bool, reason string) {
	user, err := model.GetUserById(userId, false)
	if err != nil || strings.TrimSpace(user.Email) == "" {
		return
	}
	lang := user.GetSetting().Language
	args := map[string]any{"Amount": moneyDec(withdrawal.Amount).StringFixed(2), "Reason": reason, "Txn": withdrawal.PaymentTxnNo}
	subjectKey, bodyKey := i18n.MsgAdtyReferralWithdrawRejectedSubject, i18n.MsgAdtyReferralWithdrawRejectedBody
	if paid {
		subjectKey, bodyKey = i18n.MsgAdtyReferralWithdrawPaidSubject, i18n.MsgAdtyReferralWithdrawPaidBody
	}
	go func() {
		// 二开：发件人按用户所属站点（任务 11.3）
		if err := AdtySendSiteEmail(AdtySiteById(user.SiteId), i18n.Translate(lang, subjectKey, args), user.Email, i18n.Translate(lang, bodyKey, args)); err != nil {
			common.SysError(common.LogText("referral withdrawal email failed user=%d: %s", userId, err.Error()))
		}
	}()
}

// allocateWithdrawalItemsTx 按可提现的佣金顺序冻结明细；后台调整出来的余额没有对应佣金，记一条 commission_id=0 的明细。
func (s *ReferralService) allocateWithdrawalItemsTx(tx *gorm.DB, affiliateId, withdrawalId int, amount decimal.Decimal) error {
	remaining := amount
	var commissions []model.ReferralCommission
	if err := model.AdtyLockForUpdate(tx).Where("affiliate_id = ? AND status IN ?", affiliateId,
		[]string{model.ReferralCommissionStatusAvailable, model.ReferralCommissionStatusFrozen}).
		Order("available_at, id").Find(&commissions).Error; err != nil {
		return err
	}
	for _, commission := range commissions {
		if !remaining.IsPositive() {
			break
		}
		frozen, withdrawn, err := commissionAllocatedTx(tx, commission.Id)
		if err != nil {
			return err
		}
		free := moneyDec(commission.CommissionAmount).Sub(frozen).Sub(withdrawn)
		if !free.IsPositive() {
			continue
		}
		use := decimal.Min(free, remaining)
		if err := tx.Create(&model.ReferralWithdrawalItem{WithdrawalId: withdrawalId, CommissionId: commission.Id, AllocatedAmount: moneyF(use), Status: model.ReferralWithdrawalItemStatusFrozen}).Error; err != nil {
			return err
		}
		if err := syncCommissionStatusTx(tx, commission.Id); err != nil {
			return err
		}
		remaining = remaining.Sub(use)
	}
	if remaining.IsPositive() {
		return tx.Create(&model.ReferralWithdrawalItem{WithdrawalId: withdrawalId, AllocatedAmount: moneyF(remaining), Status: model.ReferralWithdrawalItemStatusFrozen}).Error
	}
	return nil
}

func (s *ReferralService) releaseWithdrawalItemsTx(tx *gorm.DB, withdrawalId int) error {
	return s.updateWithdrawalItemsTx(tx, withdrawalId, model.ReferralWithdrawalItemStatusReleased)
}

func (s *ReferralService) markWithdrawalItemsPaidTx(tx *gorm.DB, withdrawalId int) error {
	return s.updateWithdrawalItemsTx(tx, withdrawalId, model.ReferralWithdrawalItemStatusWithdrawn)
}

func (s *ReferralService) updateWithdrawalItemsTx(tx *gorm.DB, withdrawalId int, status string) error {
	var items []model.ReferralWithdrawalItem
	if err := tx.Where("withdrawal_id = ? AND status = ?", withdrawalId, model.ReferralWithdrawalItemStatusFrozen).Find(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		updates := map[string]any{"status": status}
		if status == model.ReferralWithdrawalItemStatusReleased {
			updates["allocated_amount"] = 0
		}
		if err := tx.Model(&model.ReferralWithdrawalItem{}).Where("id = ?", item.Id).Updates(updates).Error; err != nil {
			return err
		}
		if item.CommissionId > 0 {
			if err := syncCommissionStatusTx(tx, item.CommissionId); err != nil {
				return err
			}
		}
	}
	return nil
}

func commissionAllocatedTx(tx *gorm.DB, commissionId int) (frozen, withdrawn decimal.Decimal, err error) {
	var items []model.ReferralWithdrawalItem
	if err = tx.Where("commission_id = ?", commissionId).Find(&items).Error; err != nil {
		return
	}
	for _, item := range items {
		switch item.Status {
		case model.ReferralWithdrawalItemStatusFrozen:
			frozen = frozen.Add(moneyDec(item.AllocatedAmount))
		case model.ReferralWithdrawalItemStatusWithdrawn:
			withdrawn = withdrawn.Add(moneyDec(item.AllocatedAmount))
		}
	}
	return
}

// syncCommissionStatusTx 按提现明细推佣金状态。pending、void、clawed_back 不动，否则提现释放或打款时会把它们改回去。
func syncCommissionStatusTx(tx *gorm.DB, commissionId int) error {
	var commission model.ReferralCommission
	if err := tx.Where("id = ?", commissionId).First(&commission).Error; err != nil {
		return err
	}
	switch commission.Status {
	case model.ReferralCommissionStatusPending, model.ReferralCommissionStatusVoid, model.ReferralCommissionStatusClawedBack:
		return nil
	}
	frozen, withdrawn, err := commissionAllocatedTx(tx, commissionId)
	if err != nil {
		return err
	}
	status := model.ReferralCommissionStatusAvailable
	switch {
	case frozen.IsPositive():
		status = model.ReferralCommissionStatusFrozen
	case withdrawn.GreaterThanOrEqual(moneyDec(commission.CommissionAmount)):
		status = model.ReferralCommissionStatusPaid
	}
	frozenAt := commission.FrozenAt
	if status == model.ReferralCommissionStatusFrozen && frozenAt == 0 {
		frozenAt = time.Now().Unix()
	}
	if status != model.ReferralCommissionStatusFrozen {
		frozenAt = 0
	}
	return tx.Model(&model.ReferralCommission{}).Where("id = ?", commissionId).Updates(map[string]any{"status": status, "frozen_at": frozenAt}).Error
}

// referralProgramCanStayOn 用数据库里的当前值加上这次要写的字段，判断开着的时候是否还有默认比例。
func referralProgramCanStayOn(tx *gorm.DB, values map[string]string) error {
	rate, _, err := model.ReadOptionForUpdate(tx, "adty_referral_setting.default_rate")
	if err != nil {
		return err
	}
	enabled, _, err := model.ReadOptionForUpdate(tx, "adty_referral_setting.enabled")
	if err != nil {
		return err
	}
	if incoming, ok := values["adty_referral_setting.default_rate"]; ok {
		rate = incoming
	}
	if incoming, ok := values["adty_referral_setting.enabled"]; ok {
		enabled = incoming
	}
	rate = strings.TrimSpace(rate)
	if enabled == "true" && (rate == "" || rate == "null") {
		return common.NewMessage("Set the default commission rate before turning the referral program on")
	}
	return nil
}

// classifyWithdrawCNYRate 只检查汇率，不写库。空串和 0 表示这次不改汇率。
func classifyWithdrawCNYRate(raw string) (rate decimal.Decimal, write bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return decimal.Zero, false, nil
	}
	rate, err = decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero, false, common.NewMessage("The exchange rate must be a number")
	}
	if rate.IsZero() {
		return decimal.Zero, false, nil
	}
	if !rate.IsPositive() {
		return decimal.Zero, false, common.NewMessage("The exchange rate must be greater than 0")
	}
	return rate, true, nil
}

// persistWithdrawCNYRateTx 在调用方事务里写汇率和审计。和库里相同则什么都不写。
func (s *ReferralService) persistWithdrawCNYRateTx(tx *gorm.DB, rate decimal.Decimal, adminId int, ip, ua string) (map[string]string, error) {
	old, _, err := model.ReadOptionForUpdate(tx, adty_setting.ReferralKeyWithdrawCNYRate)
	if err != nil {
		return nil, err
	}
	if oldRate, parseErr := decimal.NewFromString(strings.TrimSpace(old)); parseErr == nil && oldRate.Equal(rate) {
		return nil, nil
	}
	now := time.Now().Unix()
	values := map[string]string{
		adty_setting.ReferralKeyWithdrawCNYRate:          rate.String(),
		adty_setting.ReferralKeyWithdrawCNYRateUpdatedAt: fmt.Sprint(now),
	}
	if err := model.SaveOptionsTx(tx, values); err != nil {
		return nil, err
	}
	if err := tx.Create(&model.ReferralAdminAuditLog{
		Action: "withdraw_cny_rate", AdminUserId: adminId, Ip: ip, UserAgent: truncateText(ua, 512),
		OldValue: old, NewValue: rate.String(),
	}).Error; err != nil {
		return nil, err
	}
	return values, nil
}

// UpdateWithdrawCNYRate 改提现汇率。汇率和审计同一个事务，提交后才改内存。为空、为 0 或和库里相同都不写。
func (s *ReferralService) UpdateWithdrawCNYRate(raw string, adminId int, ip, ua string) error {
	rate, write, err := classifyWithdrawCNYRate(raw)
	if err != nil || !write {
		return err
	}
	var written map[string]string
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		written, txErr = s.persistWithdrawCNYRateTx(tx, rate, adminId, ip, ua)
		return txErr
	})
	if err != nil {
		return err
	}
	return model.ApplyOptionValues(written)
}

// SaveReferralSettings 一次提交推广设置和提现汇率。汇率不合法或后半段写库失败时，设置也不落库、内存也不改。
func (s *ReferralService) SaveReferralSettings(values map[string]string, rawRate string, adminId int, ip, ua string) error {
	rate, writeRate, err := classifyWithdrawCNYRate(rawRate)
	if err != nil {
		return err
	}
	written := map[string]string{}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := referralProgramCanStayOn(tx, values); err != nil {
			return err
		}
		if err := model.SaveOptionsTx(tx, values); err != nil {
			return err
		}
		for key, value := range values {
			written[key] = value
		}
		if !writeRate {
			return nil
		}
		extra, err := s.persistWithdrawCNYRateTx(tx, rate, adminId, ip, ua)
		if err != nil {
			return err
		}
		for key, value := range extra {
			written[key] = value
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := model.ApplyOptionValues(written); err != nil {
		return err
	}
	return refreshReferralGateOptions()
}

// refreshReferralGateOptions 把库里的默认比例和开关写回本进程，避免别的进程刚改过、这里还是旧值。
func refreshReferralGateOptions() error {
	values := map[string]string{}
	for _, key := range []string{"adty_referral_setting.default_rate", "adty_referral_setting.enabled"} {
		value, ok, err := model.ReadOptionFromDB(key)
		if err != nil {
			return err
		}
		if ok {
			values[key] = value
		}
	}
	return model.ApplyOptionValues(values)
}
