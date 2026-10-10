package proof

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMatchCoverageReceiptSharedCWIRequiresBothSourceObligations(t *testing.T) {
	item := runtimecontract.RuntimeCoverageItem{ID: "shared", SliceID: "slice", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	for _, id := range []string{"A", "B"} {
		check := runtimecontract.RuntimeSourceObligationCheck{InventoryHash: "inventory", ObligationID: id, RequirementID: "requirement", VerificationID: "verify-" + id, Criterion: "判据 " + id, Declaration: "声明 " + id}
		check.CheckRef = runtimecontract.StableRuntimeSourceObligationCheckRef("definition", item.ID, check)
		item.SourceObligationChecks = append(item.SourceObligationChecks, check)
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements("definition", []runtimecontract.RuntimeCoverageItem{item}, []runtimecontract.RuntimeDefinitionFocus{{
		ID: "focus", SliceID: "slice", AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, EvidenceDeclarationRef: "definition/dodItem/dod-001",
	}})
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile: %+v %v", requirements, err)
	}
	requirement := requirements[0]
	var attestations []runtimecontract.RuntimeValidationCheckAttestationBinding
	var allNative, onlyA []string
	for _, ref := range requirement.RequiredCheckRefs {
		native := "native/" + ref
		attestations = append(attestations, runtimecontract.RuntimeValidationCheckAttestationBinding{RequiredCheckRef: ref, ObservationProtocolRef: "owner/protocol", NativeCheckRef: native})
		allNative = append(allNative, native)
		if ref != item.SourceObligationChecks[1].CheckRef {
			onlyA = append(onlyA, native)
		}
	}
	binding := runtimecontract.CoverageEvidenceBinding{ID: "binding", CoverageWorkItemID: item.ID, FocusID: "focus", DefinitionRevision: "definition", DeclarationRef: requirement.DeclarationRef, TargetRefs: []string{"target"},
		Proofs: []runtimecontract.CoverageEvidenceProof{{EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			ValidationRecipeID: "recipe", ValidationProofRequirementHash: requirement.Hash, RequiredCheckRefs: requirement.RequiredCheckRefs, ValidationCheckAttestations: attestations}}}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	operation := runtimecontract.RuntimeOperation{ID: "op", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, BindingRefs: []string{binding.ID}, TargetRefs: []string{"target"}}
	receipt := runtimecontract.RuntimeOperationReceipt{ExecutionKey: "key", Fence: 1, OperationID: operation.ID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone, TargetRefs: operation.TargetRefs,
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{RecipeID: "recipe", ObservationProtocolRef: "owner/protocol", ExecutedChecks: allNative, PassedChecks: onlyA}}}
	finding := MatchCoverageReceipt([]runtimecontract.CoverageEvidenceBinding{binding}, operation, runtimecontract.EvidenceKindValidation, &receipt)
	if len(finding.CoverageWorkItemIDs) != 0 || finding.Diagnostic == "" {
		t.Fatalf("A-only receipt received shared CWI credit: %+v", finding)
	}
	receipt.Evidence.Validation.PassedChecks = allNative
	finding = MatchCoverageReceipt([]runtimecontract.CoverageEvidenceBinding{binding}, operation, runtimecontract.EvidenceKindValidation, &receipt)
	if len(finding.CoverageWorkItemIDs) != 1 || finding.CoverageWorkItemIDs[0] != item.ID || finding.Diagnostic != "" {
		t.Fatalf("all_of receipt rejected: %+v", finding)
	}
}
