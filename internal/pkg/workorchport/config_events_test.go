package workorchport

import "testing"

func TestNormalizeChatModelTargetOptionsKeepsOnlyReadyCompleteUniqueTargets(t *testing.T) {
	got := NormalizeChatModelTargetOptions([]ChatModelTargetOption{
		{ProviderID: " provider-b ", ModelID: " model-2 ", CapabilityReady: true},
		{ProviderID: "provider-a", ModelID: "model-1", CapabilityReady: true},
		{ProviderID: "provider-a", ModelID: "model-1", DisplayName: "replacement", CapabilityReady: true},
		{ProviderID: "provider-a", ModelID: "disabled", CapabilityReady: false},
		{ProviderID: "", ModelID: "missing-provider", CapabilityReady: true},
	})
	if len(got) != 2 {
		t.Fatalf("options = %+v, want two ready unique targets", got)
	}
	if got[0].ProviderID != "provider-a" || got[0].ModelID != "model-1" || got[1].ProviderID != "provider-b" || got[1].ModelID != "model-2" {
		t.Fatalf("sorted options = %+v", got)
	}
}
