package runtime

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInputReviewInformationBudgetAppliesOnlyToUnresolvedExactSubjects(t *testing.T) {
	document := runtimecontract.RuntimeSemanticPlanDocument{Proposals: []runtimecontract.RuntimeSemanticProofProposal{
		{ValidationInputReview: &runtimecontract.RuntimeValidationInputReview{SubjectHash: "unresolved-input", Checks: []runtimecontract.RuntimeValidationInputCheckReview{{Conclusion: runtimecontract.RuntimeValidationInputInconclusive}}}},
		{ValidationInputReview: &runtimecontract.RuntimeValidationInputReview{SubjectHash: "consistent-input", Checks: []runtimecontract.RuntimeValidationInputCheckReview{{Conclusion: runtimecontract.RuntimeValidationInputConsistent}}}},
	}}
	for _, tc := range []struct {
		name, subject, want string
		count               int
	}{
		{name: "first question"},
		{name: "last available retirement", count: runtimecontract.RuntimeSemanticPlanningHistoryLimit - 1},
		{name: "history exhausted", count: runtimecontract.RuntimeSemanticPlanningHistoryLimit, want: "validation_input_review_history_exhausted"},
		{name: "exact unresolved subject already answered", subject: "unresolved-input", want: "validation_input_review_information_exhausted"},
		{name: "different content has a new subject", subject: "older-input"},
		{name: "another now-consistent input does not block a new question", subject: "consistent-input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var info []runtimecontract.RuntimeValidationInputReviewInformation
			if tc.subject != "" {
				info = []runtimecontract.RuntimeValidationInputReviewInformation{{SubjectHash: tc.subject}}
			}
			if got := runtimeInputReviewInformationFailure(tc.count, info, document); got != tc.want {
				t.Fatalf("failure=%q want %q", got, tc.want)
			}
		})
	}
	for _, order := range [][]string{{runtimecontract.RuntimeValidationInputConflict, runtimecontract.RuntimeValidationInputInconclusive}, {runtimecontract.RuntimeValidationInputInconclusive, runtimecontract.RuntimeValidationInputConflict}} {
		document.Proposals[0].ValidationInputReview.Checks[0].Conclusion = order[0]
		document.Proposals[1].ValidationInputReview.Checks[0].Conclusion = order[1]
		if runtimeInputReviewConclusion(document) != runtimecontract.RuntimeValidationInputConflict {
			t.Fatal("uncertainty overrode a known conflict")
		}
	}
}

func TestInputReviewV2InformationBudgetCannotResetWithInputSubject(t *testing.T) {
	document := runtimecontract.RuntimeSemanticPlanDocument{BatchID: "batch", Proposals: []runtimecontract.RuntimeSemanticProofProposal{{
		CoverageWorkItemID: "cwi-original", OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		ValidationRecipeID: "recipe", DeclarationRef: "original-declaration", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		ValidationInputReview: &runtimecontract.RuntimeValidationInputReview{SubjectHash: "old-input", Checks: []runtimecontract.RuntimeValidationInputCheckReview{{RequiredCheckRef: "original-check", Conclusion: runtimecontract.RuntimeValidationInputInconclusive}}},
	}}}
	manifest := &runtimecontract.RuntimeSemanticPlanManifest{OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{{
		PrerequisiteID: "review-catalog", OwnerModuleID: runtimecontract.RuntimeModuleShell, ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: "recipe", OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			ProofRequirementBindings: []runtimecontract.RuntimeValidationProofRequirementBinding{{RequirementHash: "original-requirement", CoverageWorkItemID: "cwi-original", DeclarationRef: "original-declaration", InputReview: &runtimecontract.RuntimeValidationInputReviewSubject{ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2}}},
		}},
	}}}
	prior := runtimecontract.RuntimeSemanticPlanningGeneration{Reason: runtimecontract.RuntimeSemanticPlanningRetirementInputReviewInformation,
		Snapshot: runtimecontract.RuntimeSemanticPlanningLedger{Manifest: manifest, ActiveBatchID: "batch", BatchDocuments: []runtimecontract.RuntimeSemanticPlanDocument{document}}}
	for _, tc := range []struct {
		name, protocol, subject, check, want string
		coverage                             common.CoverageWorkItemID
	}{
		{name: "new content", protocol: runtimecontract.RuntimeValidationInputReviewProtocolV2, subject: "new-content", check: "original-check", coverage: "cwi-original", want: "validation_input_review_information_exhausted"},
		{name: "new path and native", protocol: runtimecontract.RuntimeValidationInputReviewProtocolV2, subject: "new-path-native-and-catalog", check: "original-check", coverage: "cwi-original", want: "validation_input_review_information_exhausted"},
		{name: "v3 changed subject retains original budget", protocol: runtimecontract.RuntimeValidationInputReviewProtocolV3, subject: "new-composite-subject", check: "original-check", coverage: "cwi-original", want: "validation_input_review_information_exhausted"},
		{name: "v3 different CWI", protocol: runtimecontract.RuntimeValidationInputReviewProtocolV3, subject: "other", check: "original-check", coverage: "cwi-other"},
		{name: "v3 different original check", protocol: runtimecontract.RuntimeValidationInputReviewProtocolV3, subject: "other", check: "other-check", coverage: "cwi-original"},
		{name: "different CWI", protocol: runtimecontract.RuntimeValidationInputReviewProtocolV2, subject: "other", check: "original-check", coverage: "cwi-other"},
		{name: "different original check", protocol: runtimecontract.RuntimeValidationInputReviewProtocolV2, subject: "other", check: "other-check", coverage: "cwi-original"},
		{name: "legacy subject budget unchanged", protocol: runtimecontract.RuntimeValidationInputReviewProtocol, subject: "new-content", check: "original-check", coverage: "cwi-original"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := document.Clone()
			current.Proposals[0].CoverageWorkItemID = tc.coverage
			current.Proposals[0].ValidationInputReview.SubjectHash = tc.subject
			current.Proposals[0].ValidationInputReview.Checks[0].RequiredCheckRef = tc.check
			m := manifest.Clone()
			binding := &m.OwnerCatalogs[0].ValidationSurfaces[0].ProofRequirementBindings[0]
			binding.CoverageWorkItemID, binding.InputReview.ProtocolRef = tc.coverage, tc.protocol
			ledger := runtimecontract.RuntimeSemanticPlanningLedger{Manifest: &m, History: []runtimecontract.RuntimeSemanticPlanningGeneration{prior}}
			if got := runtimeInputReviewInformationFailureForLedger(ledger, current); got != tc.want {
				t.Fatalf("information budget: got %q want %q", got, tc.want)
			}
			if got := runtimeInputReviewInformationFailureForLedger(ledger.Clone(), current.Clone()); got != tc.want {
				t.Fatal("reload clone reset information budget")
			}
		})
	}
}
