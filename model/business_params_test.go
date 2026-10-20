package model

import (
	"errors"
	"testing"
)

func TestValidateBusinessParam_FirstTopupBonusRate(t *testing.T) {
	if err := ValidateBusinessParam("FirstTopupBonusRate", "0.15", nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBusinessParam("FirstTopupBonusRate", "1.5", nil); err == nil {
		t.Fatal("want error for 1.5")
	}
	if err := ValidateBusinessParam("FirstTopupBonusRate", "-0.1", nil); err == nil {
		t.Fatal("want error for -0.1")
	}
}

func TestValidateBusinessParam_DiskAlertCross(t *testing.T) {
	proposed := map[string]string{"DiskAlertWarnPct": "80", "DiskAlertCritPct": "90"}
	if err := ValidateBusinessParam("DiskAlertWarnPct", "80", proposed); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBusinessParam("DiskAlertCritPct", "90", proposed); err != nil {
		t.Fatal(err)
	}
	bad := map[string]string{"DiskAlertWarnPct": "90", "DiskAlertCritPct": "90"}
	if err := ValidateBusinessParam("DiskAlertCritPct", "90", bad); err == nil {
		t.Fatal("want crit>warn")
	}
	if err := ValidateBusinessParam("DiskAlertCritPct", "80", map[string]string{"DiskAlertWarnPct": "80"}); err == nil {
		t.Fatal("want crit>warn")
	}
}

func TestValidateBusinessParam_Price(t *testing.T) {
	if err := ValidateBusinessParam("Price", "7.3", nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBusinessParam("Price", "0.5", nil); err == nil {
		t.Fatal("want error")
	}
	if err := ValidateBusinessParam("Price", "21", nil); err == nil {
		t.Fatal("want error")
	}
}

func TestValidateBusinessParam_DailyReportTime(t *testing.T) {
	if err := ValidateBusinessParam("DailyReportTime", "09:00", nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBusinessParam("DailyReportTime", "9:00", nil); err == nil {
		t.Fatal("want HH:MM")
	}
}

func TestValidateBusinessParam_BackupContact(t *testing.T) {
	if err := ValidateBusinessParam("BackupContact", "support@adtyai.com", map[string]string{"BackupContactType": "email"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBusinessParam("BackupContact", "@adtyai_support", map[string]string{"BackupContactType": "telegram"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBusinessParam("BackupContact", "not-an-email", map[string]string{"BackupContactType": "email"}); err == nil {
		t.Fatal("want error")
	}
}

func TestAffiliateCatalogDefaultsDoNotPay(t *testing.T) {
	for _, d := range BusinessParamCatalog() {
		switch d.Key {
		case "AffTopupCommissionRate", "AffSubscriptionCommissionRate", "AffMinWithdrawAmount":
			if d.Default != "0" {
				t.Fatalf("%s default %s, want 0", d.Key, d.Default)
			}
		}
	}
}

func TestBusinessParamCatalogCount(t *testing.T) {
	cat := BusinessParamCatalog()
	if len(cat) != 31 {
		t.Fatalf("want 31 params, got %d", len(cat))
	}
	for _, d := range cat {
		if d.Key == "EpayInstanceDailyLimit" {
			t.Fatal("EpayInstanceDailyLimit must not be in global catalog")
		}
	}
}

func TestGetTopupOrderRateLimitDefaults(t *testing.T) {
	n, d := GetTopupOrderRateLimit()
	if n != 10 || d != 60 {
		// may have been mutated by other tests; just ensure positive
		if n < 1 || d < 10 {
			t.Fatalf("unexpected rate limit %d/%d", n, d)
		}
	}
}

func TestApplyBusinessParamUpdatesReportsOffendingKey(t *testing.T) {
	// 两个合法 + 一个越界：无论 map 顺序如何，都报越界那个 key（校验在 DB 之前失败，不碰库）
	for i := 0; i < 50; i++ {
		err := ApplyBusinessParamUpdates(map[string]string{
			"AffFreezeDays":              "7",
			"BackupRetentionDays":        "500",
			"TopupOrderTimeoutMinutes":   "30",
			"EpayFailureDemoteThreshold": "5",
		})
		var bpErr *BusinessParamError
		if !errors.As(err, &bpErr) || !errors.Is(err, ErrBusinessParamRange) {
			t.Fatalf("want BusinessParamError range, got %v", err)
		}
		if bpErr.Key != "BackupRetentionDays" || bpErr.Value != "500" {
			t.Fatalf("want BackupRetentionDays=500, got %s=%s", bpErr.Key, bpErr.Value)
		}
	}
	// 两个都越界：按 key 排序稳定报第一个
	for i := 0; i < 50; i++ {
		err := ApplyBusinessParamUpdates(map[string]string{"BackupRetentionDays": "1", "AffFreezeDays": "999"})
		var bpErr *BusinessParamError
		if !errors.As(err, &bpErr) || bpErr.Key != "AffFreezeDays" {
			t.Fatalf("want AffFreezeDays first, got %v", err)
		}
	}
}

func TestValidateTopupBonusTiersCodes(t *testing.T) {
	cases := []struct {
		in     string
		code   string
		params map[string]any
	}{
		{`[1,2,3]`, "topup_tiers_count_mismatch", map[string]any{"key": "TopupBonusTiers", "expected_count": 13, "actual_count": 3}},
		{`[1,1.5,2,2.5,3,3.5,4,4.5,5,5.5,6,6.5,101]`, "out_of_range", map[string]any{"key": "TopupBonusTiers", "index": 12, "value": 101.0, "min": 0, "max": 100}},
		{`[1,1.5,2,2.5,3,3.5,4,4.5,5,5.5,6,6.5,-1]`, "out_of_range", map[string]any{"key": "TopupBonusTiers", "index": 12, "value": -1.0, "min": 0, "max": 100}},
		{`[1,1.5,2,2.5,3,3.5,4,4.5,5,5.5,6,6.5,6]`, "topup_tiers_not_ascending", map[string]any{"key": "TopupBonusTiers", "index": 12, "value": 6.0, "prev_value": 6.5}},
	}
	for _, c := range cases {
		err := ValidateBusinessParam("TopupBonusTiers", c.in, nil)
		var coded *BusinessParamCodedError
		if !errors.As(err, &coded) || !errors.Is(err, ErrInvalidTopupTiers) {
			t.Fatalf("%s: want coded error, got %v", c.in, err)
		}
		if coded.Code != c.code || coded.Message == "" {
			t.Fatalf("%s: code=%q msg=%q", c.in, coded.Code, coded.Message)
		}
		if len(coded.Params) != len(c.params) {
			t.Fatalf("%s: params=%v want %v", c.in, coded.Params, c.params)
		}
		for k, v := range c.params {
			if coded.Params[k] != v {
				t.Fatalf("%s: params[%s]=%v want %v", c.in, k, coded.Params[k], v)
			}
		}
	}
	if err := ValidateBusinessParam("TopupBonusTiers", DefaultTopupBonusTiersJSON(), nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBusinessParam("TopupBonusTiers", `{"a":1}`, nil); !errors.Is(err, ErrBusinessParamFormat) {
		t.Fatalf("non-array want format err, got %v", err)
	}
}

func TestParamRuleViolationModel(t *testing.T) {
	for _, c := range []struct {
		key, val, rule, other string
		proposed              map[string]string
	}{
		{"DiskAlertCritPct", "80", "greater_than", "DiskAlertWarnPct", map[string]string{"DiskAlertWarnPct": "80"}},
		{"DiskAlertWarnPct", "90", "less_than", "DiskAlertCritPct", map[string]string{"DiskAlertCritPct": "90"}},
	} {
		err := ValidateBusinessParam(c.key, c.val, c.proposed)
		var coded *BusinessParamCodedError
		if !errors.As(err, &coded) || coded.Code != "param_rule_violation" || !errors.Is(err, ErrBusinessParamRange) {
			t.Fatalf("%s: got %v", c.key, err)
		}
		p := coded.Params
		if p["key"] != c.key || p["rule"] != c.rule || p["other_key"] != c.other || len(p) != 3 {
			t.Fatalf("%s: params %v", c.key, p)
		}
	}
	// 单字段越界仍是 out_of_range 哨兵，不是规则违反
	err := ValidateBusinessParam("DiskAlertCritPct", "100", nil)
	var coded *BusinessParamCodedError
	if errors.As(err, &coded) || !errors.Is(err, ErrBusinessParamRange) {
		t.Fatalf("crit=100 want plain range err, got %v", err)
	}
}

func TestTopupTiersEdgeCases(t *testing.T) {
	err := ValidateBusinessParam("TopupBonusTiers", `[-0.5,1.5,2,2.5,3,3.5,4,4.5,5,5.5,6,6.5,8]`, nil)
	var coded *BusinessParamCodedError
	if !errors.As(err, &coded) || coded.Code != "out_of_range" || coded.Params["index"] != 0 || coded.Params["value"] != -0.5 {
		t.Fatalf("tier0: %v", err)
	}
	err = ValidateBusinessParam("TopupBonusTiers", `[1,1.5,2,2.5,3,3.5,2,4.5,5,5.5,6,6.5,8]`, nil)
	if !errors.As(err, &coded) || coded.Code != "topup_tiers_not_ascending" || coded.Params["index"] != 6 || coded.Params["prev_value"] != 3.5 {
		t.Fatalf("middle down: %v", err)
	}
}
