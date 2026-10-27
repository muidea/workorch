package workorchmanagementcontract

import "testing"

func TestPreparationDefinitionCompilationViewKeepsExactOriginalIdentity(t *testing.T) {
	a := PreparationAttemptView{
		State: "blocked", EvaluationVersion: "workorch-preparation-check-evaluation/v1", SealEvidenceRef: "seal",
		Assessment: &PreparationAssessmentView{}, Binding: PreparationBindingView{SnapshotHash: "snapshot", ManifestHash: "manifest"},
		Checks: []PreparationCheckView{{ID: "provider_cache", Owner: "llm", Required: true, State: "blocked", Reason: "provider_cache_unavailable"}},
	}
	c := PreparationDefinitionCompilationView{CompilerVersion: "workorch-preparation-definition-compiler/v1", SnapshotHash: "snapshot", ManifestHash: "manifest", SourceInventoryHash: "inventory", ProfileContractsHash: "profile", SourceCount: 2, RequirementCount: 1, ObligationCount: 1, VerificationCount: 1}
	if c.ValidateFor(a) != nil {
		t.Fatal("metadata cannot coexist with Provider blocked")
	}
	for _, mutate := range []func(*PreparationDefinitionCompilationView){
		func(c *PreparationDefinitionCompilationView) { c.CompilerVersion = "other" },
		func(c *PreparationDefinitionCompilationView) { c.SnapshotHash = "other" },
		func(c *PreparationDefinitionCompilationView) { c.ManifestHash = "other" },
		func(c *PreparationDefinitionCompilationView) { c.SourceInventoryHash = "" },
		func(c *PreparationDefinitionCompilationView) { c.ProfileContractsHash = "" },
		func(c *PreparationDefinitionCompilationView) { c.SourceCount = 1 },
		func(c *PreparationDefinitionCompilationView) { c.ObligationCount = 0 },
		func(c *PreparationDefinitionCompilationView) { c.RequirementCount = 4097 },
		func(c *PreparationDefinitionCompilationView) { c.VerificationCount = -1 },
	} {
		bad := c
		mutate(&bad)
		if bad.ValidateFor(a) == nil {
			t.Fatal("invalid source compilation metadata accepted", bad)
		}
	}
	for _, mutate := range []func(*PreparationAttemptView){
		func(a *PreparationAttemptView) { a.State = "accepted" },
		func(a *PreparationAttemptView) { a.Assessment = nil },
		func(a *PreparationAttemptView) { a.SealEvidenceRef = "" },
		func(a *PreparationAttemptView) { a.EvaluationVersion = "" },
		func(a *PreparationAttemptView) { a.Checks = []PreparationCheckView{{Reason: "binding_changed"}} },
		func(a *PreparationAttemptView) {
			a.Checks = []PreparationCheckView{{Reason: "original_check_interrupted"}}
		},
	} {
		bad := a
		mutate(&bad)
		if c.ValidateFor(bad) == nil {
			t.Fatal("unbound or interrupted compilation accepted")
		}
	}
}
