package runtime

import (
	"encoding/json"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDiagnosticSourceInformationBudgetDoesNotResetWithSourceIdentityOrRestart(t *testing.T) {
	document := runtimecontract.RuntimeSemanticPlanDocument{BatchID: "batch", Proposals: []runtimecontract.RuntimeSemanticProofProposal{{CoverageWorkItemID: "original-cwi", OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession, ValidationRecipeID: "recipe", DeclarationRef: "original", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, ValidationInputReview: &runtimecontract.RuntimeValidationInputReview{SubjectHash: "old", Checks: []runtimecontract.RuntimeValidationInputCheckReview{{RequiredCheckRef: "original-check", Conclusion: runtimecontract.RuntimeValidationInputConsistent}}, DiagnosticSources: []runtimecontract.RuntimeValidationDiagnosticSourceReview{{SourceRef: "old-source", Conclusion: runtimecontract.RuntimeValidationInputInconclusive}}}}}}
	manifest := &runtimecontract.RuntimeSemanticPlanManifest{OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{{OwnerModuleID: runtimecontract.RuntimeModuleShell, ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{ID: "recipe", OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession, ProofRequirementBindings: []runtimecontract.RuntimeValidationProofRequirementBinding{{CoverageWorkItemID: "original-cwi", DeclarationRef: "original", InputReview: &runtimecontract.RuntimeValidationInputReviewSubject{ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV3, DiagnosticSourceContractRef: runtimecontract.RuntimeValidationDiagnosticSourceReviewV1}}}}}}}}
	ledger := runtimecontract.RuntimeSemanticPlanningLedger{Manifest: manifest, History: []runtimecontract.RuntimeSemanticPlanningGeneration{{Reason: runtimecontract.RuntimeSemanticPlanningRetirementInputReviewInformation, Snapshot: runtimecontract.RuntimeSemanticPlanningLedger{Manifest: manifest, ActiveBatchID: "batch", BatchDocuments: []runtimecontract.RuntimeSemanticPlanDocument{document}}}}}
	current := document.Clone()
	current.Proposals[0].ValidationInputReview.SubjectHash = "new-bytes-path-native-revision"
	current.Proposals[0].ValidationInputReview.DiagnosticSources[0].SourceRef = "renamed-source"
	if got := runtimeInputReviewInformationFailureForLedger(ledger, current); got != "validation_input_review_information_exhausted" {
		t.Fatal("source identity reset budget", got)
	}
	bytes, _ := json.Marshal(ledger)
	var reload runtimecontract.RuntimeSemanticPlanningLedger
	if json.Unmarshal(bytes, &reload) != nil || runtimeInputReviewInformationFailureForLedger(reload, current) != "validation_input_review_information_exhausted" {
		t.Fatal("reload reset budget")
	}
	current.Proposals[0].ValidationInputReview.DiagnosticSources[0].Conclusion = runtimecontract.RuntimeValidationInputConsistent
	if got := runtimeInputReviewInformationFailureForLedger(ledger, current); got != "" {
		t.Fatal("consistent source blocked", got)
	}
	current.Proposals[0].ValidationInputReview.Checks[0].Conclusion = runtimecontract.RuntimeValidationInputInconclusive
	if got := runtimeInputReviewInformationFailureForLedger(ledger, current); got != "validation_input_review_information_exhausted" {
		t.Fatal("source budget laundered into original check", got)
	}
}
