package service

import "testing"

func TestAdtyBepusdtCallbackPaidUSDTRejectsNonUsdtFiatFallback(t *testing.T) {
	got := AdtyBepusdtCallbackPaidUSDT(map[string]interface{}{
		"amount":   "100",
		"currency": "USD",
	})
	if got != -1 {
		t.Fatalf("fiat USD amount must not be treated as paid USDT, got %v", got)
	}

	missing := AdtyBepusdtCallbackPaidUSDT(map[string]interface{}{
		"amount": "10.5",
	})
	if missing != -1 {
		t.Fatalf("amount without a USDT currency must not be used, got %v", missing)
	}

	ok := AdtyBepusdtCallbackPaidUSDT(map[string]interface{}{
		"amount": "10.5",
		"fiat":   "USDT",
	})
	if ok != 10.5 {
		t.Fatalf("explicit USDT fiat may fall back to amount, got %v", ok)
	}

	actual := AdtyBepusdtCallbackPaidUSDT(map[string]interface{}{
		"actual_amount": "9.99",
		"amount":        "100",
		"currency":      "USD",
	})
	if actual != 9.99 {
		t.Fatalf("actual_amount wins over fiat amount, got %v", actual)
	}
}
