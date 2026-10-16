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
