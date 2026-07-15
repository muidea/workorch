package workorchruntimecontract

import "testing"

func TestProviderModelTargetNormalizeValidateSameClone(t *testing.T) {
	target := ProviderModelTarget{
		ProviderID: "  ai-proxy  ",
		ModelID:    "  gpt-4o  ",
	}
	normalized := target.Normalized()
	if normalized.ProviderID != "ai-proxy" || normalized.ModelID != "gpt-4o" {
		t.Fatalf("normalized = %+v, want trimmed provider/model ids", normalized)
	}
	if err := normalized.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !normalized.Same(ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}) {
		t.Fatal("same targets should match")
	}
	if normalized.Same(ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "other"}) {
		t.Fatal("different model ids must not match")
	}
	if !SameProviderModelTarget(normalized, normalized.Clone()) {
		t.Fatal("clone must preserve identity")
	}

	if err := (ProviderModelTarget{ModelID: "gpt-4o"}).Validate(); err == nil {
		t.Fatal("empty provider id must fail validation")
	}
	if err := (ProviderModelTarget{ProviderID: "ai-proxy"}).Validate(); err == nil {
		t.Fatal("empty model id must fail validation")
	}
}
