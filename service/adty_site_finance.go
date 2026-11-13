package service

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
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
	ErrAdtyOperatingWalletHidden         = errors.New("operating wallet hidden from site owner")
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
	Type             string
	RefType          string
	RefID            string
	Reason           string
	OperatorID       int
	IdempotencyKey   string
	AllowOverdraft   bool
	OpenMarginCycle  bool
	CycleTier        decimal.Decimal
	ReleaseCycleUsed decimal.Decimal
	HoldDelta        decimal.Decimal
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
	return listAdtySiteWalletLedger(siteID, kind, offset, limit, false)
}

// ListAdtyOwnerSiteWalletLedger is the site-owner projection. The operating
// wallet remains in the database and in Root responses.
func ListAdtyOwnerSiteWalletLedger(siteID int, kind string, offset, limit int) ([]model.AdtySiteWalletLedger, int64, error) {
	if kind == model.AdtySiteWalletOperating {
		return nil, 0, ErrAdtyOperatingWalletHidden
	}
	return listAdtySiteWalletLedger(siteID, kind, offset, limit, true)
}

func listAdtySiteWalletLedger(siteID int, kind string, offset, limit int, hideOperating bool) ([]model.AdtySiteWalletLedger, int64, error) {
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
	} else if hideOperating {
		query = query.Where("wallet_kind <> ?", model.AdtySiteWalletOperating)
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
	return entry.SiteId == siteID && entry.WalletKind == kind && entry.Delta.Equal(delta) && entry.HoldDelta.Equal(change.HoldDelta) && entry.Type == change.Type &&
		entry.RefType == change.RefType && entry.RefId == change.RefID && entry.Reason == change.Reason &&
		entry.OperatorId == change.OperatorID && entry.AllowOverdraft == change.AllowOverdraft
}

// Wallet amounts must be exactly representable by the shared decimal(28,8)
// columns; otherwise some databases silently round and others reject writes.
func validAdtySiteWalletAmount(amount decimal.Decimal) bool {
	return amount.Equal(amount.Truncate(8)) && amount.Abs().LessThan(decimal.New(1, 20))
}

func applyAdtySiteWalletChangeTx(tx *gorm.DB, siteID int, kind string, delta decimal.Decimal, change AdtySiteWalletChange) (*model.AdtySiteWalletLedger, error) {
	if (delta.IsZero() && change.HoldDelta.IsZero()) || (!delta.IsZero() && !validAdtySiteWalletAmount(delta)) || (!change.HoldDelta.IsZero() && !validAdtySiteWalletAmount(change.HoldDelta)) {
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
	frozenAfter := wallet.FrozenUSD.Add(change.HoldDelta)
	if !validAdtySiteWalletAmount(after) || frozenAfter.IsNegative() || !validAdtySiteWalletAmount(frozenAfter) {
		return nil, ErrAdtySiteWalletInvalidOperation
	}
	minimum := decimal.Zero
	if kind == model.AdtySiteWalletMargin {
		minimum = wallet.RequiredBalance
	}
	if (delta.IsNegative() || change.HoldDelta.IsPositive()) && !change.AllowOverdraft && after.Sub(frozenAfter).LessThan(minimum) {
		return nil, ErrAdtySiteWalletInsufficient
	}
	cycleUsed := wallet.CycleUsedUSD
	cycleTier := wallet.CycleTierUSD
	if kind == model.AdtySiteWalletMargin && delta.IsNegative() && cycleTier.IsPositive() && !change.AllowOverdraft {
		newDraw := delta.Abs()
		if change.HoldDelta.IsNegative() {
			consumed := decimal.Min(change.HoldDelta.Abs(), newDraw)
			newDraw = newDraw.Sub(consumed)
		}
		allowed := AdtyMarginNewDraw(AdtyMarginDrawInput{
			TierUSD: cycleTier, Balance: before, Used: cycleUsed, Frozen: wallet.FrozenUSD,
		})
		if newDraw.GreaterThan(allowed) {
			return nil, ErrAdtySiteWalletInsufficient
		}
		cycleUsed = cycleUsed.Add(delta.Abs())
	}
	if kind == model.AdtySiteWalletMargin && delta.IsZero() && change.HoldDelta.IsPositive() && cycleTier.IsPositive() && !change.AllowOverdraft {
		allowed := AdtyMarginNewDraw(AdtyMarginDrawInput{
			TierUSD: cycleTier, Balance: before, Used: cycleUsed, Frozen: wallet.FrozenUSD,
		})
		if change.HoldDelta.GreaterThan(allowed) {
			return nil, ErrAdtySiteWalletInsufficient
		}
	}
	if kind == model.AdtySiteWalletMargin && delta.IsPositive() && change.OpenMarginCycle && change.CycleTier.IsPositive() {
		cycleTier = change.CycleTier
		cycleUsed = decimal.Zero
	}
	if kind == model.AdtySiteWalletMargin && delta.IsPositive() && change.ReleaseCycleUsed.IsPositive() && !change.OpenMarginCycle {
		cycleUsed = cycleUsed.Sub(change.ReleaseCycleUsed)
		if cycleUsed.IsNegative() {
			cycleUsed = decimal.Zero
		}
	}
	if err := tx.Model(&model.AdtySiteWallet{}).Where("id = ?", wallet.Id).Updates(map[string]any{
		"balance": after, "frozen_usd": frozenAfter, "cycle_tier_usd": cycleTier, "cycle_used_usd": cycleUsed,
	}).Error; err != nil {
		return nil, err
	}
	result := &model.AdtySiteWalletLedger{
		SiteId: siteID, WalletId: wallet.Id, WalletKind: kind, Delta: delta, HoldDelta: change.HoldDelta,
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
// OpenAdtyMarginCycle credits the frozen tier and starts a 90% draw cycle.
func OpenAdtyMarginCycle(siteID int, tier decimal.Decimal, key string) error {
	if !tier.IsPositive() {
		return ErrAdtySiteWalletInvalidOperation
	}
	if err := EnsureAdtySiteWallets(siteID); err != nil {
		return err
	}
	_, err := ApplyAdtySiteWalletChange(siteID, model.AdtySiteWalletMargin, tier, AdtySiteWalletChange{
		Type: "margin_cycle_open", IdempotencyKey: key, OpenMarginCycle: true, CycleTier: tier,
	})
	return err
}

// CollectAdtyFundAnomalies compares wallet ledgers, open shortfall holds, and the margin cycle cap.
// It records a new open disposition for each detected kind and does not invent a tolerance.
func CollectAdtyFundAnomalies(siteID int) ([]string, error) {
	if siteID <= 0 {
		return nil, ErrAdtySiteWalletInvalidOperation
	}
	var wallets []model.AdtySiteWallet
	if err := model.DB.Where("site_id = ?", siteID).Find(&wallets).Error; err != nil {
		return nil, err
	}
	balanceGap := int64(0)
	marginGap := decimal.Zero
	var operating decimal.Decimal
	for _, wallet := range wallets {
		var deltas []decimal.Decimal
		if err := model.DB.Model(&model.AdtySiteWalletLedger{}).Where("wallet_id = ?", wallet.Id).Pluck("delta", &deltas).Error; err != nil {
			return nil, err
		}
		sum := decimal.Zero
		for _, delta := range deltas {
			sum = sum.Add(delta)
		}
		if !sum.Equal(wallet.Balance) {
			balanceGap = 1
		}
		if wallet.Kind == model.AdtySiteWalletOperating {
			operating = wallet.Balance
		}
		if wallet.Kind == model.AdtySiteWalletMargin && wallet.CycleTierUSD.IsPositive() {
			over := wallet.CycleUsedUSD.Sub(AdtyMarginCycleLimit(wallet.CycleTierUSD))
			if over.IsPositive() {
				marginGap = over
			}
		}
	}
	var quota int64
	if err := model.DB.Model(&model.User{}).Where("site_id = ?", siteID).Select("COALESCE(SUM(quota), 0)").Scan(&quota).Error; err != nil {
		return nil, err
	}
	principal := decimal.NewFromInt(quota).Div(common.QuotaPerUnitDecimal())
	if !ReconcileOperatingWallet(principal, operating).IsZero() {
		balanceGap = 1
	}
	var userIDs []int
	if err := model.DB.Model(&model.User{}).Where("site_id = ?", siteID).Pluck("id", &userIDs).Error; err != nil {
		return nil, err
	}
	var holds int64
	if len(userIDs) > 0 {
		if err := model.DB.Model(&model.AdtyBillingHold{}).Where("user_id IN ? AND status = ?", userIDs, model.AdtyBillingHoldOpen).Count(&holds).Error; err != nil {
			return nil, err
		}
	}
	kinds := AdtyFundAnomalies(balanceGap, int(holds), marginGap)
	dupCredit, err := adtyDuplicateEventTrades(siteID, "credited", "")
	if err != nil {
		return nil, err
	}
	dupRefund, err := adtyDuplicateEventTrades(siteID, "", "refund.succeeded")
	if err != nil {
		return nil, err
	}
	if dupCredit > 0 {
		kinds = append(kinds, "duplicate_credit")
	}
	if dupRefund > 0 {
		kinds = append(kinds, "duplicate_refund")
	}
	ref := strconv.Itoa(siteID)
	for _, kind := range kinds {
		var existing int64
		if err := model.DB.Model(&model.AdtyAnomalyDisposition{}).Where("kind = ? AND ref = ? AND status = ?", kind, ref, "open").Count(&existing).Error; err != nil {
			return nil, err
		}
		if existing == 0 {
			if err := RecordAdtyAnomalyDisposition(kind, ref, "open", "detected", 0); err != nil {
				return nil, err
			}
		}
	}
	return kinds, nil
}

func adtyDuplicateEventTrades(siteID int, outcome, eventType string) (int64, error) {
	query := model.DB.Model(&model.AdtyOrderPaymentEvent{}).Where("site_id = ?", siteID)
	if outcome != "" {
		query = query.Where("outcome = ?", outcome)
	}
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	var trades []string
	if err := query.Group("trade_no").Having("COUNT(*) > 1").Pluck("trade_no", &trades).Error; err != nil {
		return 0, err
	}
	return int64(len(trades)), nil
}

// ListAdtyAnomalyDispositions returns review rows for one site. It does not create any.
func ListAdtyAnomalyDispositions(siteID int) ([]model.AdtyAnomalyDisposition, error) {
	if siteID <= 0 {
		return nil, common.NewMessage("Invalid parameters")
	}
	var rows []model.AdtyAnomalyDisposition
	err := model.DB.Where("ref = ?", strconv.Itoa(siteID)).Order("id desc").Limit(50).Find(&rows).Error
	return rows, err
}

// ReviewAdtyAnomalyDisposition records a Root review. It does not move wallets or customer quota.
func ReviewAdtyAnomalyDisposition(id int64, operatorID int, status, note string) error {
	status, note = strings.TrimSpace(status), strings.TrimSpace(note)
	if id <= 0 || operatorID <= 0 || (status != "reviewing" && status != "closed") {
		return common.NewMessage("Invalid parameters")
	}
	if status == "closed" && note == "" {
		return common.NewMessage("Invalid parameters")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtyAnomalyDisposition
		if err := model.AdtyLockForUpdate(tx).First(&row, id).Error; err != nil {
			return err
		}
		if row.Status == "closed" || (row.Status == "reviewing" && status != "closed") || (row.Status != "open" && row.Status != "reviewing") {
			return common.NewMessage("Invalid parameters")
		}
		if err := tx.Model(&row).Updates(map[string]any{
			"status": status, "note": note, "operator_id": operatorID,
		}).Error; err != nil {
			return err
		}
		return notifyAdtyAnomalyDispositionTx(tx, row, operatorID, status, note)
	})
}

func notifyAdtyAnomalyDispositionTx(tx *gorm.DB, row model.AdtyAnomalyDisposition, operatorID int, status, note string) error {
	siteID, err := strconv.Atoi(strings.TrimSpace(row.Ref))
	if err != nil || siteID <= 0 {
		return nil
	}
	publisher := "root"
	var operator model.User
	if err := tx.Select("username").First(&operator, operatorID).Error; err == nil && operator.Username != "" {
		publisher = operator.Username
	}
	content := "资金异常处置已记录。Fund anomaly review recorded. " + status
	if note != "" {
		content += ": " + note
	}
	now := time.Now().Unix()
	notice := model.AdtyNotificationPublication{
		IssuerSiteId: 0, PublisherId: operatorID, PublisherName: publisher,
		Title: "资金异常处置", Content: content, Kind: "system", Audience: "site",
		TargetSiteId: siteID, TargetSiteName: "", Business: "fund_anomaly",
		Status: "published", Revision: 1, PublishedAt: now, UpdatedAt: now, UpdatedBy: operatorID,
		ReadKey: adtyNotificationVersion("anomaly:" + strconv.FormatInt(row.Id, 10) + ":" + status),
	}
	return tx.Create(&notice).Error
}

// TransferAdtyEarningToMargin moves earning cash into margin and opens a new draw cycle.
func TransferAdtyEarningToMargin(siteID, operator int, amount, tier decimal.Decimal, key string) error {
	if !amount.IsPositive() || !tier.IsPositive() || strings.TrimSpace(key) == "" {
		return ErrAdtySiteWalletInvalidOperation
	}
	if err := EnsureAdtySiteWallets(siteID); err != nil {
		return err
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := applyAdtySiteWalletChangeTx(tx, siteID, model.AdtySiteWalletEarnings, amount.Neg(), AdtySiteWalletChange{
			Type: "earning_to_margin", IdempotencyKey: key + ":earn", OperatorID: operator,
		}); err != nil {
			return err
		}
		_, err := applyAdtySiteWalletChangeTx(tx, siteID, model.AdtySiteWalletMargin, amount, AdtySiteWalletChange{
			Type: "earning_to_margin", IdempotencyKey: key + ":margin", OperatorID: operator,
			OpenMarginCycle: true, CycleTier: tier,
		})
		return err
	})
}

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
