package service

import (
	"errors"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

const (
	// WalletLotProjectionSourceBatches is a remaining-by-kind sum of stored lots.
	WalletLotProjectionSourceBatches = "fund_batches"
	// WalletLotProjectionSourceLegacy is a display-only stand-in when a user has
	// no stored lots yet. ConsumeAdtyFundBatchesTx would record the raw balance
	// as one principal lot; this projection does not write that lot.
	WalletLotProjectionSourceLegacy = "legacy_principal"
)

// WalletLotProjection is the official remaining 本金 and 永久赠金 view of an
// account's fund lots. It is a projection, not a second wallet. TotalUSD is
// PrincipalUSD plus BonusUSD from WalletUSDPartsFromQuota (DEC-001).
type WalletLotProjection struct {
	PrincipalQuota int64           `json:"principal_quota"`
	BonusQuota     int64           `json:"bonus_quota"`
	TotalQuota     int64           `json:"total_quota"`
	PrincipalUSD   decimal.Decimal `json:"principal_usd"`
	BonusUSD       decimal.Decimal `json:"bonus_usd"`
	TotalUSD       decimal.Decimal `json:"total_usd"`
	Source         string          `json:"source"`
}

// ProjectWalletLotsForUser returns the lot remaining projection for one account.
// ok is false when the fund-batch table is missing so callers can omit the field
// instead of inventing numbers or failing login. 待履约 (not-yet-granted) bonus
// is not counted because it is not stored as a spendable bonus lot.
func ProjectWalletLotsForUser(user *model.User) (WalletLotProjection, bool, error) {
	if user == nil || user.Id <= 0 {
		return WalletLotProjection{}, false, nil
	}
	projections, err := ProjectWalletLotsForUsers([]model.User{*user})
	if err != nil {
		if errors.Is(err, model.ErrAdtyFundBatchTableMissing) {
			return WalletLotProjection{}, false, nil
		}
		return WalletLotProjection{}, false, err
	}
	proj, ok := projections[user.Id]
	return proj, ok, nil
}

// ProjectWalletLotsForUsers returns remaining-by-kind projections for many accounts.
func ProjectWalletLotsForUsers(users []model.User) (map[int]WalletLotProjection, error) {
	out := map[int]WalletLotProjection{}
	if len(users) == 0 {
		return out, nil
	}
	ids := make([]int, 0, len(users))
	for _, user := range users {
		if user.Id > 0 {
			ids = append(ids, user.Id)
		}
	}
	sums, err := model.SumAdtyFundBatchRemainingByKindForUsers(ids)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		if user.Id <= 0 {
			continue
		}
		sum := sums[user.Id]
		principal, bonus := sum.Principal, sum.Bonus
		source := WalletLotProjectionSourceBatches
		if !sum.HasLots {
			// Display-only: a user with no lots is still treated as legacy principal.
			principal, bonus = int64(user.Quota), 0
			source = WalletLotProjectionSourceLegacy
		}
		out[user.Id] = walletLotProjectionFromQuota(principal, bonus, source)
	}
	return out, nil
}

func walletLotProjectionFromQuota(principalQuota, bonusQuota int64, source string) WalletLotProjection {
	parts := WalletUSDPartsFromQuota(principalQuota, bonusQuota)
	return WalletLotProjection{
		PrincipalQuota: principalQuota,
		BonusQuota:     bonusQuota,
		TotalQuota:     principalQuota + bonusQuota,
		PrincipalUSD:   parts.Principal,
		BonusUSD:       parts.Bonus,
		TotalUSD:       parts.Total,
		Source:         source,
	}
}
