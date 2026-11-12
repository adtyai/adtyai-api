package service

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const adtyVoucherQuotaPerUSD int64 = 500000

var (
	ErrAdtyVoucherInvalid      = common.NewMessage("Voucher code is invalid")
	ErrAdtyVoucherUsed         = common.NewMessage("Voucher code has already been used")
	ErrAdtyVoucherFrozen       = common.NewMessage("Voucher code is frozen")
	ErrAdtyVoucherCancelled    = common.NewMessage("Voucher code is cancelled")
	ErrAdtyVoucherWrongAccount = common.NewMessage("This voucher cannot be used on this account")
	ErrAdtyVoucherLimit        = common.NewMessage("Issuance limit is not configured")
	ErrAdtyVoucherFunds        = common.NewMessage("Insufficient available funds")
	ErrAdtyVoucherRate         = common.NewMessage("Too many voucher attempts. Try again later")
)

type AdtyVoucherIssueInput struct {
	IssuerId  int
	Purpose   string
	SiteId    int
	AmountUSD decimal.Decimal
	Source    string
	Reason    string
}

type AdtyVoucherIssued struct {
	Id        int64           `json:"id"`
	Code      string          `json:"code"`
	CodeTail  string          `json:"code_tail"`
	Purpose   string          `json:"purpose"`
	AmountUSD decimal.Decimal `json:"amount_usd"`
	Quota     int64           `json:"quota"`
}

type AdtyVoucherRedeemed struct {
	TradeNo   string          `json:"trade_no"`
	Purpose   string          `json:"purpose"`
	AmountUSD decimal.Decimal `json:"amount_usd"`
	Quota     int64           `json:"quota"`
	Replayed  bool            `json:"replayed"`
}

var voucherGuess struct {
	mu    sync.Mutex
	fails map[int][]int64
}

func ResetAdtyVoucherGuesses() {
	voucherGuess.mu.Lock()
	voucherGuess.fails = nil
	voucherGuess.mu.Unlock()
}

func IssueAdtyVoucher(input AdtyVoucherIssueInput) (*AdtyVoucherIssued, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	input.Purpose = strings.TrimSpace(input.Purpose)
	input.Source = strings.TrimSpace(input.Source)
	quota, err := adtyVoucherQuota(input.AmountUSD)
	if err != nil || input.IssuerId <= 0 || input.Reason == "" || len(input.Reason) > 200 {
		return nil, common.NewMessage("Invalid parameters")
	}
	code, err := newAdtyVoucherCode()
	if err != nil {
		return nil, err
	}
	hash := model.AdtyVoucherCodeHash(code)
	tail := code[len(code)-4:]
	var issued *AdtyVoucherIssued
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		issuer, err := lockAdtyVoucherUser(tx, input.IssuerId)
		if err != nil {
			return err
		}
		row := model.AdtyVoucher{
			CodeHash: hash, CodeTail: tail, Purpose: input.Purpose, Status: model.AdtyVoucherStatusPending,
			AmountUSD: input.AmountUSD, Quota: quota, IssuerId: issuer.Id,
		}
		switch input.Purpose {
		case model.AdtyVoucherPurposeMainUser:
			if issuer.Role != common.RoleRootUser {
				return common.NewMessage("No permission to perform this action")
			}
			row.SourceKind = model.AdtyVoucherSourcePlatform
			if err := freezeAdtyVoucherLiabilityTx(tx, input.AmountUSD); err != nil {
				return err
			}
		case model.AdtyVoucherPurposeSiteMargin:
			if issuer.Role != common.RoleRootUser || input.SiteId <= 0 {
				return common.NewMessage("Invalid parameters")
			}
			if _, err := lockAdtyVoucherSite(tx, input.SiteId); err != nil {
				return err
			}
			row.SourceKind = model.AdtyVoucherSourcePlatform
			row.TargetSiteId = input.SiteId
			if err := freezeAdtyVoucherLiabilityTx(tx, input.AmountUSD); err != nil {
				return err
			}
		case model.AdtyVoucherPurposeSiteCustomer, model.AdtyVoucherPurposeSiteOwner:
			site, err := lockAdtyVoucherSite(tx, input.SiteId)
			if err != nil {
				return err
			}
			if site.OwnerUserId != issuer.Id || site.Status != model.AdtySiteStatusActive {
				return common.NewMessage("No permission to perform this action")
			}
			source := input.Source
			if source == "" {
				source = model.AdtyVoucherSourceEarnings
			}
			if source != model.AdtyVoucherSourceEarnings && source != model.AdtyVoucherSourceMargin {
				return common.NewMessage("Invalid parameters")
			}
			if source == model.AdtyVoucherSourceMargin && !adtyVoucherMarginAuthorized(tx, site.Id) {
				return common.NewMessage("No permission to perform this action")
			}
			row.SourceKind = source
			row.SourceSiteId = site.Id
			row.TargetSiteId = site.Id
			if err := freezeAdtyVoucherWalletTx(tx, site.Id, source, input.AmountUSD, hash); err != nil {
				return err
			}
		default:
			return common.NewMessage("Invalid parameters")
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		issued = &AdtyVoucherIssued{Id: row.Id, Code: code, CodeTail: tail, Purpose: row.Purpose, AmountUSD: row.AmountUSD, Quota: row.Quota}
		return nil
	})
	return issued, err
}

func RedeemAdtyVoucher(userId int, code string) (*AdtyVoucherRedeemed, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if userId <= 0 || len(code) < 8 || len(code) > 64 {
		return nil, ErrAdtyVoucherInvalid
	}
	if !allowAdtyVoucherGuess(userId, time.Now().Unix()) {
		return nil, ErrAdtyVoucherRate
	}
	hash := model.AdtyVoucherCodeHash(code)
	var result *AdtyVoucherRedeemed
	var credit int64
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		user, err := lockAdtyVoucherUser(tx, userId)
		if err != nil {
			return err
		}
		var voucher model.AdtyVoucher
		lookup := model.AdtyLockForUpdate(tx).Where("code_hash = ?", hash).Limit(1).Find(&voucher)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			return ErrAdtyVoucherInvalid
		}
		if voucher.Status == model.AdtyVoucherStatusRedeemed {
			if voucher.RedeemerId == user.Id {
				result = &AdtyVoucherRedeemed{TradeNo: voucher.TradeNo, Purpose: voucher.Purpose, AmountUSD: voucher.AmountUSD, Quota: voucher.Quota, Replayed: true}
				return nil
			}
			return ErrAdtyVoucherUsed
		}
		if voucher.Status == model.AdtyVoucherStatusFrozen {
			return ErrAdtyVoucherFrozen
		}
		if voucher.Status == model.AdtyVoucherStatusCancelled {
			return ErrAdtyVoucherCancelled
		}
		if voucher.Status != model.AdtyVoucherStatusPending {
			return ErrAdtyVoucherInvalid
		}
		if err := adtyVoucherRedeemAllowed(tx, user, &voucher); err != nil {
			return err
		}
		trade := "VCH" + strings.ToUpper(common.Sha1([]byte(hash + ":" + time.Now().Format(time.RFC3339Nano))))[:20]
		now := time.Now().Unix()
		updated := tx.Model(&model.AdtyVoucher{}).Where("id = ? AND status = ?", voucher.Id, model.AdtyVoucherStatusPending).
			Updates(map[string]any{"status": model.AdtyVoucherStatusRedeemed, "redeemer_id": user.Id, "trade_no": trade, "redeemed_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrAdtyVoucherUsed
		}
		if err := spendAdtyVoucherSourceTx(tx, &voucher); err != nil {
			return err
		}
		if voucher.Purpose == model.AdtyVoucherPurposeSiteMargin {
			if err := creditAdtyVoucherMarginTx(tx, voucher.TargetSiteId, voucher.AmountUSD, trade); err != nil {
				return err
			}
		} else {
			before, err := model.AdtyCreditUserQuotaTx(tx, user.Id, voucher.Quota)
			if err != nil {
				return err
			}
			key := "voucher:" + trade
			if err := model.InsertAdtyLedgerTx(tx, &model.AdtyBalanceLedger{
				UserId: user.Id, DeltaQuota: voucher.Quota, BalanceBefore: before, BalanceAfter: before + voucher.Quota,
				Type: model.AdtyLedgerTypeVoucher, RefType: "voucher", RefId: trade, IdempotencyKey: &key,
			}); err != nil {
				return err
			}
			credit = voucher.Quota
		}
		if err := insertAdtyVoucherOrderTx(tx, user.Id, trade, &voucher, now); err != nil {
			return err
		}
		result = &AdtyVoucherRedeemed{TradeNo: trade, Purpose: voucher.Purpose, AmountUSD: voucher.AmountUSD, Quota: voucher.Quota}
		return nil
	})
	if err != nil {
		noteAdtyVoucherGuess(userId, time.Now().Unix())
		return nil, err
	}
	if credit > 0 {
		model.AdtySyncCreditUserQuotaCache(userId, credit, "voucher redemption")
	}
	return result, nil
}

func CancelAdtyVoucher(actorId int, voucherId int64) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := lockAdtyVoucherUser(tx, actorId)
		if err != nil {
			return err
		}
		var voucher model.AdtyVoucher
		if err := model.AdtyLockForUpdate(tx).First(&voucher, voucherId).Error; err != nil {
			return err
		}
		if actor.Role != common.RoleRootUser && voucher.IssuerId != actor.Id {
			return common.NewMessage("No permission to perform this action")
		}
		if voucher.Status != model.AdtyVoucherStatusPending && voucher.Status != model.AdtyVoucherStatusFrozen {
			return common.NewMessage("Invalid parameters")
		}
		updated := tx.Model(&voucher).Where("id = ? AND status IN ?", voucher.Id, []string{model.AdtyVoucherStatusPending, model.AdtyVoucherStatusFrozen}).
			Update("status", model.AdtyVoucherStatusCancelled)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return common.NewMessage("Invalid parameters")
		}
		return releaseAdtyVoucherSourceTx(tx, &voucher)
	})
}

func SetAdtyVoucherIssuanceLimit(actorId int, limit decimal.Decimal) error {
	if !limit.IsPositive() {
		return common.NewMessage("Invalid parameters")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := lockAdtyVoucherUser(tx, actorId)
		if err != nil {
			return err
		}
		if actor.Role != common.RoleRootUser {
			return common.NewMessage("No permission to perform this action")
		}
		row, err := lockAdtyVoucherLiabilityTx(tx)
		if err != nil {
			return err
		}
		if limit.LessThan(row.FrozenUSD.Add(row.SpentUSD)) {
			return ErrAdtyVoucherLimit
		}
		return tx.Model(row).Update("limit_usd", limit).Error
	})
}

func SetAdtyVoucherMarginAuthorization(actorId, siteId int, allowed bool) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := lockAdtyVoucherUser(tx, actorId)
		if err != nil {
			return err
		}
		if actor.Role != common.RoleRootUser || siteId <= 0 {
			return common.NewMessage("No permission to perform this action")
		}
		if _, err := lockAdtyVoucherSite(tx, siteId); err != nil {
			return err
		}
		var row model.AdtyVoucherAuthorization
		lookup := tx.Where("site_id = ?", siteId).Limit(1).Find(&row)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			return tx.Create(&model.AdtyVoucherAuthorization{SiteId: siteId, MarginIssue: allowed}).Error
		}
		return tx.Model(&row).Update("margin_issue", allowed).Error
	})
}

func adtyVoucherQuota(amount decimal.Decimal) (int64, error) {
	if common.QuotaPerUnit != float64(adtyVoucherQuotaPerUSD) || !amount.IsPositive() || amount.Exponent() < -2 || amount.GreaterThan(decimal.NewFromInt(100000)) {
		return 0, common.NewMessage("Invalid parameters")
	}
	quota := amount.Mul(decimal.NewFromInt(adtyVoucherQuotaPerUSD))
	if !quota.Equal(quota.Floor()) || quota.GreaterThan(decimal.NewFromInt(int64(common.MaxWalletQuota))) {
		return 0, common.NewMessage("Invalid parameters")
	}
	return quota.IntPart(), nil
}

func newAdtyVoucherCode() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), "="), nil
}

func lockAdtyVoucherUser(tx *gorm.DB, id int) (*model.User, error) {
	var user model.User
	if err := model.AdtyLockForUpdate(tx).First(&user, id).Error; err != nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, common.NewMessage("No permission to perform this action")
	}
	return &user, nil
}

func lockAdtyVoucherSite(tx *gorm.DB, id int) (*model.AdtySite, error) {
	var site model.AdtySite
	if err := model.AdtyLockForUpdate(tx).First(&site, id).Error; err != nil {
		return nil, err
	}
	return &site, nil
}

func lockAdtyVoucherLiabilityTx(tx *gorm.DB) (*model.AdtyVoucherLiability, error) {
	var row model.AdtyVoucherLiability
	lookup := model.AdtyLockForUpdate(tx).Where("scope = ?", "main").Limit(1).Find(&row)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected == 0 {
		row = model.AdtyVoucherLiability{Scope: "main"}
		if err := tx.Create(&row).Error; err != nil {
			return nil, err
		}
	}
	return &row, nil
}

func freezeAdtyVoucherLiabilityTx(tx *gorm.DB, amount decimal.Decimal) error {
	row, err := lockAdtyVoucherLiabilityTx(tx)
	if err != nil {
		return err
	}
	if row.LimitUSD.LessThanOrEqual(decimal.Zero) || row.FrozenUSD.Add(row.SpentUSD).Add(amount).GreaterThan(row.LimitUSD) {
		return ErrAdtyVoucherLimit
	}
	return tx.Model(row).Update("frozen_usd", row.FrozenUSD.Add(amount)).Error
}

func spendAdtyVoucherLiabilityTx(tx *gorm.DB, amount decimal.Decimal) error {
	row, err := lockAdtyVoucherLiabilityTx(tx)
	if err != nil {
		return err
	}
	if row.FrozenUSD.LessThan(amount) {
		return ErrAdtyVoucherFunds
	}
	return tx.Model(row).Updates(map[string]any{
		"frozen_usd": row.FrozenUSD.Sub(amount),
		"spent_usd":  row.SpentUSD.Add(amount),
	}).Error
}

func releaseAdtyVoucherLiabilityTx(tx *gorm.DB, amount decimal.Decimal) error {
	row, err := lockAdtyVoucherLiabilityTx(tx)
	if err != nil {
		return err
	}
	if row.FrozenUSD.LessThan(amount) {
		return ErrAdtyVoucherFunds
	}
	return tx.Model(row).Update("frozen_usd", row.FrozenUSD.Sub(amount)).Error
}

func freezeAdtyVoucherWalletTx(tx *gorm.DB, siteId int, kind string, amount decimal.Decimal, hash string) error {
	if err := ensureAdtySiteWalletsTx(tx, siteId); err != nil {
		return err
	}
	_, err := applyAdtySiteWalletChangeTx(tx, siteId, kind, amount.Neg(), AdtySiteWalletChange{
		Type: "voucher_freeze", RefType: "voucher", RefID: hash[:16], Reason: "voucher issuance freeze", IdempotencyKey: "voucher-freeze:" + hash,
	})
	if err == ErrAdtySiteWalletInsufficient {
		return ErrAdtyVoucherFunds
	}
	return err
}

func releaseAdtyVoucherSourceTx(tx *gorm.DB, voucher *model.AdtyVoucher) error {
	if voucher.SourceKind == model.AdtyVoucherSourcePlatform {
		return releaseAdtyVoucherLiabilityTx(tx, voucher.AmountUSD)
	}
	_, err := applyAdtySiteWalletChangeTx(tx, voucher.SourceSiteId, voucher.SourceKind, voucher.AmountUSD, AdtySiteWalletChange{
		Type: "voucher_release", RefType: "voucher", RefID: voucher.CodeHash[:16], Reason: "voucher cancellation release", IdempotencyKey: "voucher-release:" + voucher.CodeHash,
	})
	return err
}

func spendAdtyVoucherSourceTx(tx *gorm.DB, voucher *model.AdtyVoucher) error {
	if voucher.SourceKind == model.AdtyVoucherSourcePlatform {
		return spendAdtyVoucherLiabilityTx(tx, voucher.AmountUSD)
	}
	return nil
}

func creditAdtyVoucherMarginTx(tx *gorm.DB, siteId int, amount decimal.Decimal, trade string) error {
	if err := ensureAdtySiteWalletsTx(tx, siteId); err != nil {
		return err
	}
	_, err := applyAdtySiteWalletChangeTx(tx, siteId, model.AdtySiteWalletMargin, amount, AdtySiteWalletChange{
		Type: "voucher_margin", RefType: "voucher", RefID: trade, Reason: "voucher margin credit", IdempotencyKey: "voucher-margin:" + trade,
	})
	return err
}

func adtyVoucherMarginAuthorized(tx *gorm.DB, siteId int) bool {
	var row model.AdtyVoucherAuthorization
	lookup := tx.Where("site_id = ?", siteId).Limit(1).Find(&row)
	return lookup.Error == nil && lookup.RowsAffected == 1 && row.MarginIssue
}

func adtyVoucherRedeemAllowed(tx *gorm.DB, user *model.User, voucher *model.AdtyVoucher) error {
	switch voucher.Purpose {
	case model.AdtyVoucherPurposeMainUser:
		if user.SiteId != 0 || user.StaffSiteId != 0 || user.Role != common.RoleCommonUser {
			return ErrAdtyVoucherWrongAccount
		}
	case model.AdtyVoucherPurposeSiteMargin:
		var site model.AdtySite
		if err := tx.First(&site, voucher.TargetSiteId).Error; err != nil {
			return err
		}
		if site.OwnerUserId != user.Id {
			return ErrAdtyVoucherWrongAccount
		}
	case model.AdtyVoucherPurposeSiteCustomer:
		if user.SiteId != voucher.TargetSiteId || user.StaffSiteId != 0 {
			return ErrAdtyVoucherWrongAccount
		}
	case model.AdtyVoucherPurposeSiteOwner:
		if user.Id != voucher.IssuerId {
			return ErrAdtyVoucherWrongAccount
		}
	default:
		return ErrAdtyVoucherInvalid
	}
	return nil
}

func insertAdtyVoucherOrderTx(tx *gorm.DB, userId int, trade string, voucher *model.AdtyVoucher, now int64) error {
	topUp := model.TopUp{
		UserId: userId, Amount: voucher.AmountUSD.IntPart(), Money: voucher.AmountUSD.InexactFloat64(),
		TradeNo: trade, PaymentMethod: voucher.Purpose, PaymentProvider: model.AdtyVoucherProvider,
		CreateTime: now, CompleteTime: now, Status: common.TopUpStatusSuccess,
	}
	if err := tx.Create(&topUp).Error; err != nil {
		return err
	}
	return tx.Create(&model.AdtyTopupOrder{
		TradeNo: trade, UserId: userId, Provider: model.AdtyVoucherProvider, Method: voucher.Purpose,
		FaceUSD: voucher.AmountUSD, CreditQuota: voucher.Quota, PayAmount: voucher.AmountUSD, PayCurrency: "USD",
		PaidAmount: voucher.AmountUSD, PaidCurrency: "USD", PaidSource: "voucher", SiteId: voucher.TargetSiteId,
		SiteOwnerUserId: voucher.IssuerId, TopupGroupRatio: decimal.NewFromInt(1),
	}).Error
}

func allowAdtyVoucherGuess(userId int, now int64) bool {
	voucherGuess.mu.Lock()
	defer voucherGuess.mu.Unlock()
	return len(recentAdtyVoucherGuesses(userId, now)) < 8
}

func noteAdtyVoucherGuess(userId int, now int64) {
	voucherGuess.mu.Lock()
	defer voucherGuess.mu.Unlock()
	if voucherGuess.fails == nil {
		voucherGuess.fails = map[int][]int64{}
	}
	voucherGuess.fails[userId] = append(recentAdtyVoucherGuesses(userId, now), now)
}

func recentAdtyVoucherGuesses(userId int, now int64) []int64 {
	var kept []int64
	for _, at := range voucherGuess.fails[userId] {
		if now-at < 15*60 {
			kept = append(kept, at)
		}
	}
	return kept
}
