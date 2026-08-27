package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func semanticCorrectionObservationForTest(rejection RuntimeOriginalDiagnostic) RuntimeSemanticCorrectionObservation {
	return RuntimeSemanticCorrectionObservation{
		SchemaVersion: RuntimeSemanticCorrectionObservationSchemaCurrent,
		OwnerModuleID: rejection.OwnerModuleID, Class: rejection.OriginalClass, Code: rejection.OriginalCode,
		Signature: rejection.Signature, OperationID: rejection.OperationID,
		TargetRefs: append([]string(nil), rejection.TargetRefs...), ExpectedRevision: rejection.ExpectedRevision,
		ObservedRevision: rejection.ObservedRevision, FieldViolations: CloneRuntimeFieldViolations(rejection.FieldViolations),
		SemanticViolations: cloneRuntimeSemanticContractViolations(rejection.SemanticViolations), TargetObservation: rejection.TargetObservation,
	}
}

func TestRuntimeSemanticCorrectionDirectiveIsManifestBoundAndDetached(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems[0].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-000"}
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-000", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-000", TargetText: "prerequisite"})
	manifest = sealSemanticTestManifest(t, manifest)
	violation := RuntimeSemanticContractViolation{
		Kind: RuntimeSemanticContractViolationDependencyOrder, Path: "$.coverageOrder",
		RequiredBeforeID: "cwi-000", RequiredAfterID: "cwi-001", ActualBeforePosition: 1, ActualAfterPosition: 0,
	}
	rejection := *semanticContractTestDiagnostic()
	rejection.SemanticViolations = []RuntimeSemanticContractViolation{violation}
	observation := semanticCorrectionObservationForTest(rejection)
	directive := RuntimeSemanticCorrectionDirective{
		SchemaVersion: RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      RuntimeSemanticPlanStepCoverageOrder, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		StablePrefixHash: "prefix-1", SourceObservationHash: StableRuntimeSemanticCorrectionObservationHash(observation), Attempt: 1,
		Violations: []RuntimeSemanticContractViolation{violation},
	}
	directive.Hash = StableRuntimeSemanticCorrectionDirectiveHash(directive)
	if err := directive.ValidateAgainstObservation(manifest, observation); err != nil {
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
	if err := directive.ValidateAgainstObservation(staleManifest, observation); err == nil {
		t.Fatal("directive accepted stale manifest")
	}
	staleRejection := rejection
	staleRejection.Signature = "different"
	staleObservation := semanticCorrectionObservationForTest(staleRejection)
	if err := directive.ValidateAgainstObservation(manifest, staleObservation); err == nil {
		t.Fatal("directive accepted stale source rejection")
	}
}

func TestRuntimeSemanticProofBatchCorrectionIsBoundToExactBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	violation := RuntimeSemanticContractViolation{
		Kind: RuntimeSemanticContractViolationProofBatchDocument, Path: "$.proposals",
		RequiredCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
	}
	rejection := *semanticContractTestDiagnostic()
	rejection.FieldViolations = []RuntimeFieldViolation{{Path: "$.proposals", Actual: "missing_required"}}
	rejection.SemanticViolations = []RuntimeSemanticContractViolation{violation}
	observation := semanticCorrectionObservationForTest(rejection)
	directive := RuntimeSemanticCorrectionDirective{
		SchemaVersion: RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      RuntimeSemanticPlanStepProofBatch, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		StablePrefixHash: "prefix-1", SourceObservationHash: StableRuntimeSemanticCorrectionObservationHash(observation), Attempt: 1,
		Violations: []RuntimeSemanticContractViolation{violation},
	}
	directive.Hash = StableRuntimeSemanticCorrectionDirectiveHash(directive)
	if err := directive.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}
	clone := directive.Clone()
	clone.Violations[0].RequiredCoverageWorkItemIDs[0] = "changed"
	if directive.Violations[0].RequiredCoverageWorkItemIDs[0] != "cwi-001" {
		t.Fatal("proof-batch correction clone retained batch coverage storage")
	}
	wrongStep := directive.Clone()
	wrongStep.PlanStep = RuntimeSemanticPlanStepCoverageOrder
	wrongStep.Hash = StableRuntimeSemanticCorrectionDirectiveHash(wrongStep)
	if err := wrongStep.ValidateAgainstObservation(manifest, observation); err == nil {
		t.Fatal("proof-batch correction was accepted outside proof-batch step")
	}
}

func TestRuntimeSemanticCoverageOrderCorrectionIsBoundToExactManifest(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	violation := RuntimeSemanticContractViolation{
		Kind: RuntimeSemanticContractViolationCoverageOrderDocument, Path: "$.coverageOrder", Actual: "incomplete_order",
		RequiredCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
	}
	rejection := *semanticContractTestDiagnostic()
	rejection.SemanticViolations = []RuntimeSemanticContractViolation{violation}
	observation := semanticCorrectionObservationForTest(rejection)
	directive := RuntimeSemanticCorrectionDirective{
		SchemaVersion: RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      RuntimeSemanticPlanStepCoverageOrder, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		StablePrefixHash: "prefix-1", SourceObservationHash: StableRuntimeSemanticCorrectionObservationHash(observation), Attempt: 1,
		Violations: []RuntimeSemanticContractViolation{violation},
	}
	directive.Hash = StableRuntimeSemanticCorrectionDirectiveHash(directive)
	if err := directive.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}

	incomplete := violation
	incomplete.RequiredCoverageWorkItemIDs = nil
	if err := incomplete.ValidateAgainstManifest(manifest); err == nil {
		t.Fatal("coverage-order correction accepted empty manifest coverage")
	}
	wrongStep := directive.Clone()
	wrongStep.PlanStep = RuntimeSemanticPlanStepProofBatch
	wrongStep.Hash = StableRuntimeSemanticCorrectionDirectiveHash(wrongStep)
	if err := wrongStep.ValidateAgainstObservation(manifest, observation); err == nil {
		t.Fatal("coverage-order correction was accepted outside coverage-order step")
	}
}

func TestRuntimeSemanticProposalCorrectionPreservesExactFieldFact(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	violation := RuntimeSemanticContractViolation{
		Kind: RuntimeSemanticContractViolationProposalConstraint, Path: "$.proposals[0].requiredEffectScopes",
		CoverageWorkItemID: "cwi-001", ProposalIndex: 0, Rule: RuntimeSemanticConstraintNonEmpty,
		Actual: "empty_array", Required: []string{"at least one effect scope"},
	}
	rejection := *semanticContractTestDiagnostic()
	rejection.SemanticViolations = []RuntimeSemanticContractViolation{violation}
	observation := semanticCorrectionObservationForTest(rejection)
	directive := RuntimeSemanticCorrectionDirective{
		SchemaVersion: RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      RuntimeSemanticPlanStepProofBatch, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		StablePrefixHash: "prefix-1", SourceObservationHash: StableRuntimeSemanticCorrectionObservationHash(observation), Attempt: 1,
		Violations: []RuntimeSemanticContractViolation{violation},
	}
	directive.Hash = StableRuntimeSemanticCorrectionDirectiveHash(directive)
	if err := directive.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}
	clone := directive.Clone()
	clone.Violations[0].Required[0] = "changed"
	if directive.Violations[0].Required[0] != "at least one effect scope" {
		t.Fatal("proposal correction clone retained required-value backing storage")
	}
	changed := directive.Clone()
	changed.Violations[0].Actual = "different"
	if StableRuntimeSemanticCorrectionDirectiveHash(changed) == directive.Hash {
		t.Fatal("proposal correction hash ignored actual value")
	}
}

func TestRuntimeSemanticProposalCorrectionAllowsEmptyCatalogValues(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	violation := RuntimeSemanticContractViolation{
		Kind: RuntimeSemanticContractViolationProposalConstraint, Path: "$.proposals[0].validationRecipeId",
		CoverageWorkItemID: "cwi-001", ProposalIndex: 0, Rule: RuntimeSemanticConstraintAllowedValue,
		Actual: "recipe/not-allowed",
	}
	rejection := *semanticContractTestDiagnostic()
	rejection.SemanticViolations = []RuntimeSemanticContractViolation{violation}
	observation := semanticCorrectionObservationForTest(rejection)
	directive := RuntimeSemanticCorrectionDirective{
		SchemaVersion: RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      RuntimeSemanticPlanStepProofBatch, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		StablePrefixHash: "prefix-1", SourceObservationHash: StableRuntimeSemanticCorrectionObservationHash(observation), Attempt: 1,
		Violations: []RuntimeSemanticContractViolation{violation},
	}
	directive.Hash = StableRuntimeSemanticCorrectionDirectiveHash(directive)
	if err := directive.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatalf("empty sealed catalog should remain a typed correction fact: %v", err)
	}
}
