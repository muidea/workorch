package workorchruntimecontract

import "testing"

func TestRuntimeSemanticCorrectionDirectiveIsManifestBoundAndDetached(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems[0].ReopenDependencies = []RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-000", TriggerEvidenceKinds: []string{EvidenceKindStateChange}}}
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-000", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-000", TargetText: "prerequisite"})
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	violation := RuntimeSemanticContractViolation{
		Kind: RuntimeSemanticContractViolationDependencyOrder, Path: "$.coverageOrder",
		RequiredBeforeID: "cwi-000", RequiredAfterID: "cwi-001", ActualBeforePosition: 1, ActualAfterPosition: 0,
	}
	rejection := *semanticContractTestDiagnostic()
	rejection.SemanticViolations = []RuntimeSemanticContractViolation{violation}
	directive := RuntimeSemanticCorrectionDirective{
		SchemaVersion: RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      RuntimeSemanticPlanStepCoverageOrder, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		StablePrefixHash: "prefix-1", SourceRejectionHash: StableRuntimeSemanticPlanCorrectionHash(rejection), Attempt: 1,
		Violations: []RuntimeSemanticContractViolation{violation},
	}
	directive.Hash = StableRuntimeSemanticCorrectionDirectiveHash(directive)
	if err := directive.ValidateAgainst(manifest, rejection); err != nil {
		t.Fatal(err)
	}
	clone := directive.Clone()
	clone.Violations[0].RequiredBeforeID = "changed"
	if directive.Violations[0].RequiredBeforeID != "cwi-000" {
		t.Fatal("directive clone retained violation backing storage")
	}
	for name, mutate := range map[string]func(*RuntimeSemanticCorrectionDirective){
		"edge":     func(value *RuntimeSemanticCorrectionDirective) { value.Violations[0].RequiredBeforeID = "changed" },
		"position": func(value *RuntimeSemanticCorrectionDirective) { value.Violations[0].ActualBeforePosition = 2 },
		"prefix":   func(value *RuntimeSemanticCorrectionDirective) { value.StablePrefixHash = "prefix-2" },
		"manifest": func(value *RuntimeSemanticCorrectionDirective) { value.ManifestHash = "manifest-2" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := directive.Clone()
			mutate(&changed)
			if StableRuntimeSemanticCorrectionDirectiveHash(changed) == directive.Hash {
				t.Fatalf("directive hash ignored %s mutation", name)
			}
		})
	}
	staleManifest := manifest.Clone()
	staleManifest.Hash = "stale-manifest"
	if err := directive.ValidateAgainst(staleManifest, rejection); err == nil {
		t.Fatal("directive accepted stale manifest")
	}
	staleRejection := rejection
	staleRejection.Signature = "different"
	if err := directive.ValidateAgainst(manifest, staleRejection); err == nil {
		t.Fatal("directive accepted stale source rejection")
	}
}
