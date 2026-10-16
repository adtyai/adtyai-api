package model

import "testing"

func TestComputeBonusPlanFirstTakesMax(t *testing.T) {
	// ensure defaults
	_ = UpdateTopupBonusTiersByJSONString(DefaultTopupBonusTiersJSON())
	_ = UpdateFirstTopupBonusRateString("0.10")

	plan, err := ComputeBonusPlan(100, true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ImmediateBonusUSD != 10 {
		t.Fatalf("want immediate 10 got %v", plan.ImmediateBonusUSD)
	}
	plan2, err := ComputeBonusPlan(100, false, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.ImmediateBonusUSD != 2.5 {
		t.Fatalf("want second 2.5 got %v", plan2.ImmediateBonusUSD)
	}
	plan3, err := ComputeBonusPlan(100, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan3.ImmediateBonusUSD != 2.5 || plan3.PendingExtraUSD != 7.5 {
		t.Fatalf("missing fp want 2.5+7.5 got %v+%v", plan3.ImmediateBonusUSD, plan3.PendingExtraUSD)
	}
}

func TestIsFixedTopupAmount(t *testing.T) {
	if IsFixedTopupAmount(20) || !IsFixedTopupAmount(100) {
		t.Fatal("tier check failed")
	}
}
