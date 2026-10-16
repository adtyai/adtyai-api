package model

import "testing"

func TestComputeBonusPlanTierOnlyNoFirst(t *testing.T) {
	_ = UpdateTopupBonusTiersByJSONString(DefaultTopupBonusTiersJSON())
	_ = UpdateFirstTopupBonusRateString("0.10") // 即便写入 0.10，结算也不再取高

	// 原「首充候选人 + 合法指纹」路径：现只给档位 2.5%，无 pending
	plan, err := ComputeBonusPlan(100, true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ImmediateBonusUSD != 2.5 || plan.PendingExtraUSD != 0 {
		t.Fatalf("want tier-only 2.5+0 got %v+%v", plan.ImmediateBonusUSD, plan.PendingExtraUSD)
	}
	if plan.IsFirstCandidate || plan.ApplyFirstImmediately || plan.FirstBonusRate != 0 {
		t.Fatalf("first path must be off: %+v", plan)
	}

	// 非首充：仍档位
	plan2, err := ComputeBonusPlan(100, false, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.ImmediateBonusUSD != 2.5 || plan2.PendingExtraUSD != 0 {
		t.Fatalf("want second 2.5 got %v+%v", plan2.ImmediateBonusUSD, plan2.PendingExtraUSD)
	}

	// 缺指纹：也不再进 pending 首充多送
	plan3, err := ComputeBonusPlan(100, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan3.ImmediateBonusUSD != 2.5 || plan3.PendingExtraUSD != 0 {
		t.Fatalf("missing fp want 2.5+0 got %v+%v", plan3.ImmediateBonusUSD, plan3.PendingExtraUSD)
	}
}

func TestGetFirstTopupBonusRateAlwaysZero(t *testing.T) {
	_ = UpdateFirstTopupBonusRateString("0.25")
	if GetFirstTopupBonusRate() != 0 {
		t.Fatalf("GetFirstTopupBonusRate want 0 got %v", GetFirstTopupBonusRate())
	}
}

func TestIsFixedTopupAmount(t *testing.T) {
	if IsFixedTopupAmount(20) || !IsFixedTopupAmount(100) {
		t.Fatal("tier check failed")
	}
}
