package service

import "sort"

// FundBatch is one principal or permanent bonus lot. Principal lots are spent first.
type FundBatch struct {
	ID        int64
	Kind      string
	Remaining int64
	CreatedAt int64
}

// ConsumeFundFIFO spends principal batches in creation order, then bonus batches.
// It returns the updated lots and any amount the lots could not cover.
func ConsumeFundFIFO(batches []FundBatch, amount int64) ([]FundBatch, int64) {
	if amount <= 0 {
		return batches, 0
	}
	order := append([]FundBatch(nil), batches...)
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].Kind != order[j].Kind {
			return order[i].Kind == "principal"
		}
		if order[i].CreatedAt != order[j].CreatedAt {
			return order[i].CreatedAt < order[j].CreatedAt
		}
		return order[i].ID < order[j].ID
	})
	for i := range order {
		if amount == 0 {
			break
		}
		if order[i].Remaining <= 0 {
			continue
		}
		take := order[i].Remaining
		if take > amount {
			take = amount
		}
		order[i].Remaining -= take
		amount -= take
	}
	return order, amount
}
