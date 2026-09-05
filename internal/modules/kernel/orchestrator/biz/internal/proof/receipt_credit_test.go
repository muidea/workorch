package proof

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMatchCoverageReceiptRequiresOneExactBinding(t *testing.T) {
	coverageItem := runtimecontract.RuntimeCoverageItem{
		ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "validate the exact behavior",
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(
		"definition-1",
		[]runtimecontract.RuntimeCoverageItem{coverageItem},
		[]runtimecontract.RuntimeDefinitionFocus{{
			ID: "focus-1", SliceID: coverageItem.SliceID,
			AssessmentScope: coverageItem.SourceScope, AssessmentID: coverageItem.SourceID, AssessmentText: coverageItem.TargetText,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageItem.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			AcceptedStepKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(coverageItem.SourceScope, coverageItem.SourceID),
		}},
	)
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile exact validation requirement: requirements=%+v err=%v", requirements, err)
	}
	requirement := requirements[0]
	const observationProtocolRef = "test/shell-native-validation/v1"
	attestations := make([]runtimecontract.RuntimeValidationCheckAttestationBinding, 0, len(requirement.RequiredCheckRefs))
	nativeCheckRefs := make([]string, 0, len(requirement.RequiredCheckRefs))
	for _, checkRef := range requirement.RequiredCheckRefs {
		nativeCheckRef := "native/" + checkRef
		attestations = append(attestations, runtimecontract.RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef: checkRef, ObservationProtocolRef: observationProtocolRef, NativeCheckRef: nativeCheckRef,
		})
		nativeCheckRefs = append(nativeCheckRefs, nativeCheckRef)
	}
	proofBinding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, "recipe-1", attestations...)
	if err != nil {
		t.Fatal(err)
	}
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1",
		DeclarationRef: requirement.DeclarationRef, TargetRefs: []string{"main.go"},
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: proofBinding.ValidationRecipeID,
			ValidationProofRequirementHash: proofBinding.RequirementHash, RequiredCheckRefs: proofBinding.RequiredCheckRefs,
			ValidationCheckAttestations: proofBinding.CheckAttestations,
		}},
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	facts := runtimecontract.RuntimeFacts{Focus: runtimecontract.CoverageFocus{
		ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding},
	}}
	operation := runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, BindingRefs: []string{binding.ID}, TargetRefs: []string{"main.go"},
	}
	receipt := &runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "execution-1", Fence: 1, OperationID: operation.ID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectNone, TargetRefs: []string{"main.go"},
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{
			RecipeID: "recipe-1", ObservationProtocolRef: observationProtocolRef,
			ExecutedChecks: append([]string(nil), nativeCheckRefs...), PassedChecks: append([]string(nil), nativeCheckRefs...),
		}},
	}
	finding := MatchCoverageReceipt(facts.Focus.CoverageEvidenceBindings, operation, runtimecontract.EvidenceKindValidation, receipt)
	if finding.Diagnostic != "" || len(finding.CoverageWorkItemIDs) != 1 || finding.CoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("exact receipt finding = %+v", finding)
	}
	conflicting := *receipt
	conflictingObservation := receipt.Evidence.Validation.Clone()
	conflictingObservation.FailedChecks = append([]string(nil), nativeCheckRefs...)
	conflicting.Evidence.Validation = &conflictingObservation
	finding = MatchCoverageReceipt(facts.Focus.CoverageEvidenceBindings, operation, runtimecontract.EvidenceKindValidation, &conflicting)
	if len(finding.CoverageWorkItemIDs) != 0 || !strings.Contains(finding.Diagnostic, "does not satisfy sealed proof") {
		t.Fatalf("conflicting check received coverage credit: %+v", finding)
	}
	operation.BindingRefs = []string{binding.ID, "binding-2"}
	finding = MatchCoverageReceipt(facts.Focus.CoverageEvidenceBindings, operation, runtimecontract.EvidenceKindValidation, receipt)
	if len(finding.CoverageWorkItemIDs) != 0 || !strings.Contains(finding.Diagnostic, "exactly one") {
		t.Fatalf("multi-binding receipt finding = %+v", finding)
	}
	operation.BindingRefs = []string{binding.ID}
	finding = MatchCoverageReceipt(facts.Focus.CoverageEvidenceBindings, operation, runtimecontract.EvidenceKindValidation, nil)
	if !strings.Contains(finding.Diagnostic, "missing persisted owner receipt") {
		t.Fatalf("nil receipt finding = %+v", finding)
	}
}
