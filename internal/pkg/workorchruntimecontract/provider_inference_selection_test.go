package workorchruntimecontract

import "testing"

func TestProviderInferenceSelectionPreservesDefaultAndExplicitNone(t *testing.T) {
	providerDefault := ProviderInferenceSelection{
		Target: ProviderModelTarget{ProviderID: " ai-proxy ", ModelID: " model-a "},
	}
	explicitNone := ProviderInferenceSelection{
		Target:          ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"},
		ReasoningEffort: " none ",
	}
	if err := providerDefault.Validate(); err != nil {
		t.Fatalf("validate provider default: %v", err)
	}
	if err := explicitNone.Validate(); err != nil {
		t.Fatalf("validate explicit none: %v", err)
	}
	if providerDefault.Same(explicitNone) {
		t.Fatal("provider default and explicit none must have different identity")
	}
	if got := explicitNone.Normalized(); got.Target.ModelID != "model-a" || got.ReasoningEffort != "none" {
		t.Fatalf("normalized selection = %+v", got)
	}
}

func TestProviderInferenceSelectionCloneAndValidation(t *testing.T) {
	selection := &ProviderInferenceSelection{
		Target:          ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"},
		ReasoningEffort: "high",
	}
	cloned := CloneProviderInferenceSelection(selection)
	if cloned == nil || cloned == selection || !cloned.Same(*selection) {
		t.Fatalf("cloned selection = %+v", cloned)
	}
	if CloneProviderInferenceSelection(nil) != nil {
		t.Fatal("nil selection must remain nil")
	}
	invalid := selection.Clone()
	invalid.ReasoningEffort = "not valid"
	if err := invalid.Validate(); err == nil {
		t.Fatal("reasoning effort containing whitespace must fail")
	}
}

func TestProviderCeilingEqualityIncludesReasoningEffort(t *testing.T) {
	left := &ProviderCeiling{
		Selection:                ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"}, ReasoningEffort: "high"},
		ContextWindowTokens:      8192,
		ProviderMaxOutputTokens:  2048,
		EffectiveMaxOutputTokens: 1024,
		MaxInputTokens:           4096,
		MaxBytes:                 16384,
		CapabilityRevision:       "cap-1",
		ConfigurationRevision:    "config-1",
	}
	right := *left
	if !SameProviderCeiling(left, &right) {
		t.Fatal("equal provider ceilings were not equal")
	}
	right.Selection.ReasoningEffort = "low"
	if SameProviderCeiling(left, &right) {
		t.Fatal("provider ceiling reasoning effort drift was not detected")
	}
}
