package proof

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMatchCoverageReceiptRequiresOneExactBinding(t *testing.T) {
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1",
		DeclarationRef: "receipt-test", TargetRefs: []string{"main.go"},
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "recipe-1", RequiredCheckRefs: []string{"check-1"},
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
			RecipeID: "recipe-1", ExecutedChecks: []string{"check-1"}, PassedChecks: []string{"check-1"},
		}},
	}
	finding := MatchCoverageReceipt(facts.Focus.CoverageEvidenceBindings, operation, runtimecontract.EvidenceKindValidation, receipt)
	if finding.Diagnostic != "" || len(finding.CoverageWorkItemIDs) != 1 || finding.CoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("exact receipt finding = %+v", finding)
	}
	conflicting := *receipt
	conflictingObservation := receipt.Evidence.Validation.Clone()
	conflictingObservation.FailedChecks = []string{"check-1"}
	conflicting.Evidence.Validation = &conflictingObservation
	finding = MatchCoverageReceipt(facts.Focus.CoverageEvidenceBindings, operation, runtimecontract.EvidenceKindValidation, &conflicting)
	if len(finding.CoverageWorkItemIDs) != 0 || !strings.Contains(finding.Diagnostic, "passed check") {
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
