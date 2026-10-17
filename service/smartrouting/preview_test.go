package smartrouting

import "testing"

func TestPreviewTokenRequiredAndMatch(t *testing.T) {
	intent := PreviewIntent{Kind: "activate_scheme", SchemeName: "balanced_default", Weights: DefaultWeights(), OperatorID: 1}
	if err := ConsumePreviewToken("", intent); err != ErrPreviewRequired {
		t.Fatalf("want preview_required, got %v", err)
	}
	tok, _ := IssuePreviewToken(intent)
	bad := intent
	bad.SchemeName = "other"
	if err := ConsumePreviewToken(tok, bad); err != ErrPreviewMismatch {
		t.Fatalf("want mismatch, got %v", err)
	}
	// token still unused after mismatch? current impl does not consume on mismatch — re-issue
	tok2, _ := IssuePreviewToken(intent)
	if err := ConsumePreviewToken(tok2, intent); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if err := ConsumePreviewToken(tok2, intent); err != ErrPreviewInvalid {
		t.Fatalf("single-use: %v", err)
	}
}

func TestRequirePreviewForConfigChange(t *testing.T) {
	old := DefaultConfig()
	neu := old
	neu.ActiveWeightScheme = "custom_a"
	neu.Weights = WeightParts{Conn: 40, Price: 20, Speed: 20, Stab: 10, Auth: 10}
	if err := RequirePreviewForConfigChange(old, neu, 1, ""); err != ErrPreviewRequired {
		t.Fatalf("got %v", err)
	}
	intent := PreviewIntent{Kind: "put_config", SchemeName: neu.ActiveWeightScheme, Weights: neu.Weights, OperatorID: 1}
	tok, _ := IssuePreviewToken(intent)
	if err := RequirePreviewForConfigChange(old, neu, 1, tok); err != nil {
		t.Fatal(err)
	}
	// no change → no token needed
	if err := RequirePreviewForConfigChange(old, old, 1, ""); err != nil {
		t.Fatal(err)
	}
}

// 审查重审 P2-3：preview_token 绑定 operator_id，PUT 侧必须一致。
func TestPreviewTokenOperatorBinding(t *testing.T) {
	intent := PreviewIntent{Kind: "put_config", SchemeName: "balanced_default", Weights: DefaultWeights(), OperatorID: 1}
	tok, _ := IssuePreviewToken(intent)
	other := intent
	other.OperatorID = 2
	if err := ConsumePreviewToken(tok, other); err != ErrPreviewMismatch {
		t.Fatalf("want operator mismatch, got %v", err)
	}
	tok2, _ := IssuePreviewToken(intent)
	zero := intent
	zero.OperatorID = 0
	if err := ConsumePreviewToken(tok2, zero); err != ErrPreviewMismatch {
		t.Fatalf("want zero operator rejected, got %v", err)
	}
	tok3, _ := IssuePreviewToken(intent)
	if err := ConsumePreviewToken(tok3, intent); err != nil {
		t.Fatalf("same operator should pass: %v", err)
	}
}

func TestIssuePreviewTokenRejectsZeroOperator(t *testing.T) {
	intent := PreviewIntent{Kind: "put_config", SchemeName: "balanced_default", Weights: DefaultWeights(), OperatorID: 0}
	tok, exp := IssuePreviewToken(intent)
	if tok != "" || !exp.IsZero() {
		t.Fatalf("zero operator must not issue token, got tok=%q exp=%v", tok, exp)
	}
}
