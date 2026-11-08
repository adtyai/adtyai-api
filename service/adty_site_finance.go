package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AdtySiteTierModeAuto   = "auto"
	AdtySiteTierModeManual = "manual"
)

var (
	ErrAdtySiteWalletInvalidKind         = errors.New("invalid site wallet kind")
	ErrAdtySiteWalletInvalidOperation    = errors.New("invalid site wallet operation")
	ErrAdtySiteWalletInsufficient        = errors.New("insufficient site wallet funds")
	ErrAdtySiteWalletIdempotencyConflict = errors.New("site wallet idempotency key reused for a different operation")
	ErrAdtySiteTierInvalid               = errors.New("invalid site tier")
	ErrAdtySiteTierNotFound              = errors.New("site tier not found")
	ErrAdtySiteTierMode                  = errors.New("invalid site tier mode")
	ErrAdtySiteWalletIdempotencyKey      = errors.New("site wallet idempotency key is required")
)

type AdtySiteWalletChange struct {
	Type           string
	RefType        string
	RefID          string
	Reason         string
	OperatorID     int
	IdempotencyKey string
	AllowOverdraft bool
}

func adtySiteWalletKinds() []string {
	return []string{model.AdtySiteWalletOperating, model.AdtySiteWalletEarnings, model.AdtySiteWalletMargin}
}

func validAdtySiteWalletKind(kind string) bool {
	for _, candidate := range adtySiteWalletKinds() {
		if kind == candidate {
			return true
		}
	}
	return false
}

func ensureAdtySiteWalletsTx(tx *gorm.DB, siteID int) error {
	if siteID <= 0 {
		return ErrAdtySiteNotFound
	}
	var count int64
	if err := tx.Model(&model.AdtySite{}).Where("id = ?", siteID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrAdtySiteNotFound
	}
	rows := make([]model.AdtySiteWallet, 0, len(adtySiteWalletKinds()))
	for _, kind := range adtySiteWalletKinds() {
		rows = append(rows, model.AdtySiteWallet{SiteId: siteID, Kind: kind, Currency: "USD"})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// EnsureAdtySiteWallets creates the three site wallets and is safe to call for
// both newly created sites and sites upgraded from an older schema.
func EnsureAdtySiteWallets(siteID int) error {
	return model.DB.Transaction(func(tx *gorm.DB) error { return ensureAdtySiteWalletsTx(tx, siteID) })
}

// ListAdtySiteWallets returns all three wallets without creating financial
// state during a read. Sites predating the wallet migration show zero balances
// until a real business operation initializes their wallets.
func ListAdtySiteWallets(siteID int) ([]model.AdtySiteWallet, error) {
	if _, err := GetAdtySite(siteID); err != nil {
		return nil, err
	}
	var stored []model.AdtySiteWallet
	if err := model.DB.Where("site_id = ?", siteID).Find(&stored).Error; err != nil {
		return nil, err
	}
	byKind := make(map[string]model.AdtySiteWallet, len(stored))
	for _, wallet := range stored {
		byKind[wallet.Kind] = wallet
	}
	wallets := make([]model.AdtySiteWallet, 0, len(adtySiteWalletKinds()))
	for _, kind := range adtySiteWalletKinds() {
		wallet, found := byKind[kind]
		if !found {
			wallet = model.AdtySiteWallet{SiteId: siteID, Kind: kind, Currency: "USD"}
		}
		wallets = append(wallets, wallet)
	}
	return wallets, nil
}

// ListAdtySiteWalletLedger always applies the server-selected site boundary,
// including to the total count. An empty kind selects all three wallets.
func ListAdtySiteWalletLedger(siteID int, kind string, offset, limit int) ([]model.AdtySiteWalletLedger, int64, error) {
	if kind != "" && !validAdtySiteWalletKind(kind) {
		return nil, 0, ErrAdtySiteWalletInvalidKind
	}
	if offset < 0 || limit <= 0 {
		return nil, 0, ErrAdtySiteWalletInvalidOperation
	}
	if _, err := GetAdtySite(siteID); err != nil {
		return nil, 0, err
	}
	query := model.DB.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ?", siteID)
	if kind != "" {
		query = query.Where("wallet_kind = ?", kind)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]model.AdtySiteWalletLedger, 0)
	err := query.Order("id DESC").Offset(offset).Limit(min(limit, 100)).Find(&items).Error
	return items, total, err
}

func sameAdtySiteWalletOperation(entry *model.AdtySiteWalletLedger, siteID int, kind string, delta decimal.Decimal, change AdtySiteWalletChange) bool {
	return entry.SiteId == siteID && entry.WalletKind == kind && entry.Delta.Equal(delta) && entry.Type == change.Type &&
		entry.RefType == change.RefType && entry.RefId == change.RefID && entry.Reason == change.Reason &&
		entry.OperatorId == change.OperatorID && entry.AllowOverdraft == change.AllowOverdraft
}

// Wallet amounts must be exactly representable by the shared decimal(28,8)
// columns; otherwise some databases silently round and others reject writes.
func validAdtySiteWalletAmount(amount decimal.Decimal) bool {
	return amount.Equal(amount.Truncate(8)) && amount.Abs().LessThan(decimal.New(1, 20))
}

func applyAdtySiteWalletChangeTx(tx *gorm.DB, siteID int, kind string, delta decimal.Decimal, change AdtySiteWalletChange) (*model.AdtySiteWalletLedger, error) {
	if delta.IsZero() || !validAdtySiteWalletAmount(delta) {
		return nil, ErrAdtySiteWalletInvalidOperation
	}
	var existing model.AdtySiteWalletLedger
	lookup := tx.Where("idempotency_key = ?", change.IdempotencyKey).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected > 0 {
		if !sameAdtySiteWalletOperation(&existing, siteID, kind, delta, change) {
			return nil, ErrAdtySiteWalletIdempotencyConflict
		}
		return &existing, nil
	}
	if err := ensureAdtySiteWalletsTx(tx, siteID); err != nil {
		return nil, err
	}
	var wallet model.AdtySiteWallet
	if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", siteID, kind).First(&wallet).Error; err != nil {
		return nil, err
	}
	// Another request may have committed this operation while this one
	// waited for the wallet lock. Recheck before testing the new balance;
	// a successful debit replay must still succeed when funds are now zero.
	// A locking read also sees current committed rows under MySQL's
	// repeatable-read isolation instead of the initial lookup snapshot.
	lookup = model.AdtyLockForUpdate(tx).Where("idempotency_key = ?", change.IdempotencyKey).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected > 0 {
		if !sameAdtySiteWalletOperation(&existing, siteID, kind, delta, change) {
			return nil, ErrAdtySiteWalletIdempotencyConflict
		}
		return &existing, nil
	}
	before := wallet.Balance
	after := before.Add(delta)
	if !validAdtySiteWalletAmount(after) {
		return nil, ErrAdtySiteWalletInvalidOperation
	}
	minimum := decimal.Zero
	if kind == model.AdtySiteWalletMargin {
		minimum = wallet.RequiredBalance
	}
	if delta.IsNegative() && !change.AllowOverdraft && after.LessThan(minimum) {
		return nil, ErrAdtySiteWalletInsufficient
	}
	if err := tx.Model(&model.AdtySiteWallet{}).Where("id = ?", wallet.Id).Update("balance", after).Error; err != nil {
		return nil, err
	}
	result := &model.AdtySiteWalletLedger{
		SiteId: siteID, WalletId: wallet.Id, WalletKind: kind, Delta: delta,
		BalanceBefore: before, BalanceAfter: after, Type: change.Type,
		RefType: change.RefType, RefId: change.RefID, Reason: change.Reason,
		OperatorId: change.OperatorID, AllowOverdraft: change.AllowOverdraft, IdempotencyKey: change.IdempotencyKey,
	}
	if err := tx.Create(result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

// ApplyAdtySiteWalletChange changes one wallet and appends one immutable ledger
// row. The idempotency key is mandatory because every financial mutation must
// have a stable replay boundary.
func ApplyAdtySiteWalletChange(siteID int, kind string, delta decimal.Decimal, change AdtySiteWalletChange) (*model.AdtySiteWalletLedger, error) {
	if !validAdtySiteWalletKind(kind) {
		return nil, ErrAdtySiteWalletInvalidKind
	}
	if siteID <= 0 || delta.IsZero() || !validAdtySiteWalletAmount(delta) || strings.TrimSpace(change.Type) == "" {
		return nil, ErrAdtySiteWalletInvalidOperation
	}
	change.IdempotencyKey = strings.TrimSpace(change.IdempotencyKey)
	if change.IdempotencyKey == "" || len(change.IdempotencyKey) > 128 {
		return nil, ErrAdtySiteWalletIdempotencyKey
	}
	change.RefType = strings.TrimSpace(change.RefType)
	change.RefID = strings.TrimSpace(change.RefID)

	var result *model.AdtySiteWalletLedger
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = applyAdtySiteWalletChangeTx(tx, siteID, kind, delta, change)
		return err
	})
	if err != nil {
		if adtyUniqueViolation(err) {
			var existing model.AdtySiteWalletLedger
			lookup := model.DB.Where("idempotency_key = ?", change.IdempotencyKey).Limit(1).Find(&existing)
			if lookup.Error == nil && lookup.RowsAffected > 0 && sameAdtySiteWalletOperation(&existing, siteID, kind, delta, change) {
				return &existing, nil
			}
		}
		return nil, err
	}
	return result, nil
}

func ValidateAdtySiteTier(tier model.AdtySiteTier) error {
	if strings.TrimSpace(tier.Code) == "" || len(strings.TrimSpace(tier.Code)) > 32 || strings.TrimSpace(tier.Name) == "" {
		return ErrAdtySiteTierInvalid
	}
	if tier.SortOrder < 0 {
		return ErrAdtySiteTierInvalid
	}
	for _, amount := range []decimal.Decimal{tier.MinMonthlyUSD, tier.DepositUSD} {
		if amount.IsNegative() || !amount.Equal(amount.Truncate(8)) || amount.GreaterThanOrEqual(decimal.New(1, 12)) {
			return ErrAdtySiteTierInvalid
		}
	}
	if tier.CostFactor != nil && (!tier.CostFactor.IsPositive() || tier.CostFactor.GreaterThan(decimal.NewFromInt(1)) ||
		!tier.CostFactor.Equal(tier.CostFactor.Truncate(6))) {
		return ErrAdtySiteTierInvalid
	}
	if tier.MaxMarkup != nil && (tier.MaxMarkup.LessThan(decimal.NewFromInt(1)) ||
		tier.MaxMarkup.GreaterThanOrEqual(decimal.New(1, 6)) || !tier.MaxMarkup.Equal(tier.MaxMarkup.Truncate(4))) {
		return ErrAdtySiteTierInvalid
	}
	return nil
}

// SaveAdtySiteTiers upserts definitions by their stable code. It intentionally
// does not delete omitted rows, so a partial admin update cannot silently
// remove a level that is still assigned to a site.
func SaveAdtySiteTiers(tiers []model.AdtySiteTier) error {
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		seen := map[string]struct{}{}
		for _, input := range tiers {
			input.Code = strings.TrimSpace(input.Code)
			input.Name = strings.TrimSpace(input.Name)
			if _, ok := seen[input.Code]; ok {
				return ErrAdtySiteTierInvalid
			}
			seen[input.Code] = struct{}{}
			if err := ValidateAdtySiteTier(input); err != nil {
				return err
			}
			var current model.AdtySiteTier
			find := tx.Where("code = ?", input.Code).Limit(1).Find(&current)
			if find.Error != nil {
				return find.Error
			}
			if find.RowsAffected == 0 {
				if err := tx.Create(&input).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Model(&model.AdtySiteTier{}).Where("id = ?", current.Id).Updates(map[string]any{
				"name": input.Name, "min_monthly_usd": input.MinMonthlyUSD, "cost_factor": input.CostFactor,
				"max_markup": input.MaxMarkup, "deposit_usd": input.DepositUSD, "enabled": input.Enabled,
				"sort_order": input.SortOrder,
			}).Error; err != nil {
				return err
			}
			// Fixed mode fixes the assigned level, not an obsolete copy of
			// that level's Root-managed financial terms.
			if err := tx.Model(&model.AdtySite{}).Where("tier_code = ?", input.Code).Updates(map[string]any{
				"tier_cost_factor": input.CostFactor, "tier_max_markup": input.MaxMarkup,
				"pending_tier_cost_factor": nil, "pending_tier_cost_factor_at": 0,
			}).Error; err != nil {
				return err
			}
			sites := tx.Model(&model.AdtySite{}).Select("id").Where("tier_code = ?", input.Code)
			if err := tx.Model(&model.AdtySiteWallet{}).
				Where("kind = ? AND site_id IN (?)", model.AdtySiteWalletMargin, sites).
				Update("required_balance", input.DepositUSD).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		InvalidateAdtySiteCache()
	}
	return err
}

func ListAdtySiteTiers() ([]model.AdtySiteTier, error) {
	var tiers []model.AdtySiteTier
	err := model.DB.Order("min_monthly_usd, sort_order, id").Find(&tiers).Error
	return tiers, err
}

func SelectAdtySiteTier(tiers []model.AdtySiteTier, monthlySpend decimal.Decimal) *model.AdtySiteTier {
	var selected *model.AdtySiteTier
	for i := range tiers {
		tier := &tiers[i]
		if !tier.Enabled || tier.MinMonthlyUSD.GreaterThan(monthlySpend) {
			continue
		}
		if selected == nil || tier.MinMonthlyUSD.GreaterThan(selected.MinMonthlyUSD) ||
			(tier.MinMonthlyUSD.Equal(selected.MinMonthlyUSD) && tier.SortOrder > selected.SortOrder) {
			selected = tier
		}
	}
	return selected
}

func EvaluateAdtySiteTier(siteID int, monthlySpend decimal.Decimal, now int64) (*model.AdtySite, error) {
	if siteID <= 0 || monthlySpend.IsNegative() {
		return nil, ErrAdtySiteTierInvalid
	}
	var site model.AdtySite
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteID).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdtySiteNotFound
			}
			return err
		}
		if err := ensureAdtySiteWalletsTx(tx, siteID); err != nil {
			return err
		}
		if site.TierMode == AdtySiteTierModeManual {
			return nil
		}
		var tiers []model.AdtySiteTier
		if err := tx.Where("enabled = ? AND min_monthly_usd <= ?", true, monthlySpend).
			Order("min_monthly_usd DESC, sort_order DESC, id DESC").Limit(1).Find(&tiers).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"tier_code": "", "tier_mode": AdtySiteTierModeAuto, "tier_updated_at": now,
			"pending_tier_cost_factor": nil, "pending_tier_cost_factor_at": 0,
		}
		if len(tiers) > 0 {
			updates["tier_code"] = tiers[0].Code
			updates["tier_cost_factor"] = tiers[0].CostFactor
			updates["tier_max_markup"] = tiers[0].MaxMarkup
			if err := tx.Model(&model.AdtySiteWallet{}).Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletMargin).
				Update("required_balance", tiers[0].DepositUSD).Error; err != nil {
				return err
			}
		} else {
			updates["tier_cost_factor"] = nil
			updates["tier_max_markup"] = nil
			if err := tx.Model(&model.AdtySiteWallet{}).Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletMargin).
				Update("required_balance", decimal.Zero).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.AdtySite{}).Where("id = ?", siteID).Updates(updates).Error; err != nil {
			return err
		}
		site = model.AdtySite{}
		return tx.Where("id = ?", siteID).First(&site).Error
	})
	if err != nil {
		return nil, err
	}
	InvalidateAdtySiteCache()
	return &site, nil
}

func SetAdtySiteTier(siteID int, code, mode string, now int64) (*model.AdtySite, error) {
	mode = strings.TrimSpace(mode)
	code = strings.TrimSpace(code)
	if mode != AdtySiteTierModeAuto && mode != AdtySiteTierModeManual {
		return nil, ErrAdtySiteTierMode
	}
	var site model.AdtySite
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteID).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdtySiteNotFound
			}
			return err
		}
		// Read the definition after acquiring the site lock so a concurrent
		// Root configuration update cannot be overwritten with an old copy.
		var selected *model.AdtySiteTier
		if code != "" {
			selected = &model.AdtySiteTier{}
			if err := tx.Where("code = ? AND enabled = ?", code, true).First(selected).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrAdtySiteTierNotFound
				}
				return err
			}
		}
		if err := ensureAdtySiteWalletsTx(tx, siteID); err != nil {
			return err
		}
		updates := map[string]any{
			"tier_code": code, "tier_mode": mode, "tier_updated_at": now,
			"pending_tier_cost_factor": nil, "pending_tier_cost_factor_at": 0,
		}
		if selected == nil {
			updates["tier_cost_factor"] = nil
			updates["tier_max_markup"] = nil
			if err := tx.Model(&model.AdtySiteWallet{}).Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletMargin).
				Update("required_balance", decimal.Zero).Error; err != nil {
				return err
			}
		} else {
			updates["tier_cost_factor"] = selected.CostFactor
			updates["tier_max_markup"] = selected.MaxMarkup
			if err := tx.Model(&model.AdtySiteWallet{}).Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletMargin).
				Update("required_balance", selected.DepositUSD).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.AdtySite{}).Where("id = ?", siteID).Updates(updates).Error; err != nil {
			return err
		}
		site = model.AdtySite{}
		return tx.Where("id = ?", siteID).First(&site).Error
	})
	if err != nil {
		return nil, err
	}
	InvalidateAdtySiteCache()
	return &site, nil
}
