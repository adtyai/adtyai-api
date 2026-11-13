package service

import "github.com/shopspring/decimal"

// AdtyMarginDrawInput is one margin cycle. TierUSD is the frozen tier T.
// Used, Frozen and Reserved occupy the 90% cap. Balance is the real margin cash.
type AdtyMarginDrawInput struct {
	TierUSD     decimal.Decimal
	Balance     decimal.Decimal
	Used        decimal.Decimal
	Frozen      decimal.Decimal
	Reserved    decimal.Decimal
	CycleOpened bool
}

// AdtyMarginCycleLimit is 90% of the frozen tier. The other 10% stays retained.
func AdtyMarginCycleLimit(tier decimal.Decimal) decimal.Decimal {
	if !tier.IsPositive() {
		return decimal.Zero
	}
	return tier.Mul(decimal.RequireFromString("0.9"))
}

// AdtyMarginNewDraw is the additional amount this cycle may still use.
// A closed cycle that has already reached the cap returns zero until a real
// top-up opens a new cycle (CycleOpened with Used reset by the caller).
func AdtyMarginNewDraw(in AdtyMarginDrawInput) decimal.Decimal {
	limit := AdtyMarginCycleLimit(in.TierUSD)
	if !limit.IsPositive() {
		return decimal.Zero
	}
	occupied := in.Used.Add(in.Frozen).Add(in.Reserved)
	if occupied.GreaterThanOrEqual(limit) && !in.CycleOpened {
		return decimal.Zero
	}
	if in.CycleOpened {
		occupied = in.Used.Add(in.Frozen).Add(in.Reserved)
	}
	capRoom := limit.Sub(occupied)
	retained := in.TierUSD.Sub(limit)
	cashRoom := in.Balance.Sub(in.Frozen).Sub(in.Reserved).Sub(retained)
	room := decimal.Min(capRoom, cashRoom)
	if room.IsNegative() {
		return decimal.Zero
	}
	return room
}
