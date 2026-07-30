package runtimev2

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCanonicalAssessmentCoverageRejectsWeakAcceptedEvidence(t *testing.T) {
	aggregate, _ := assessmentGateFixture("behavior is verified", "TestBehavior")
	aggregate.Facts.Evidence.Accepted[0].Quality = common.EvidenceQualityInconclusive
	aggregate.Facts.Evidence.Accepted[0].ZeroWork = true
	if refs, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); satisfied || len(refs) != 0 {
		t.Fatalf("weak owner evidence closed canonical coverage: refs=%v satisfied=%t", refs, satisfied)
	}
}

func TestCanonicalAssessmentCoverageRejectsManagedValidationWithoutPassedChecks(t *testing.T) {
	aggregate, _ := assessmentGateFixture("behavior is verified", "")
	if refs, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); satisfied || len(refs) != 0 {
		t.Fatalf("zero-check owner evidence closed canonical coverage: refs=%v satisfied=%t", refs, satisfied)
	}
}

func TestCanonicalAssessmentCoverageDoesNotUseClaimText(t *testing.T) {
	aggregate, _ := assessmentGateFixture("complete LSP tool implementation", "TestLSPToolExistence")
	if refs, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); !satisfied || len(refs) != 1 || refs[0] != "evidence-claim" {
		t.Fatalf("structured owner evidence did not close coverage: refs=%v satisfied=%t", refs, satisfied)
	}
}

func TestCanonicalAssessmentCoverageDoesNotUseNumericClaimText(t *testing.T) {
	aggregate, _ := assessmentGateFixture("request latency stays under 50ms", "TestRequestLatency")
	if _, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); !satisfied {
		t.Fatal("numeric claim text altered owner-evidence coverage decision")
	}
}

func TestCanonicalAssessmentCoverageAcceptsStructuredExecutedEvidence(t *testing.T) {
	aggregate, _ := assessmentGateFixture("request latency stays under 50ms", "TestRequestLatencyUnder50ms")
	if _, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); !satisfied {
		t.Fatal("structured executable owner evidence did not close coverage")
	}
}

func TestCanonicalAssessmentCoverageAcceptsCrossLanguageStructuredEvidence(t *testing.T) {
	aggregate, _ := assessmentGateFixture("LSP 工具", "TestToolBehavior")
	if _, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); !satisfied {
		t.Fatal("cross-language structured owner evidence did not close coverage")
	}
}

func TestCanonicalAssessmentCoveragePreservesNeutralBehaviorWhenStrictFlagsAreDisabled(t *testing.T) {
	aggregate, _ := assessmentGateFixture("unrelated concrete claim", "")
	aggregate.Facts.Focus.RequireClaimEvidence = false
	aggregate.Facts.Focus.RequireExecutableBehaviorEvidence = false
	aggregate.Facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite}
	dispatch := &aggregate.CompletedDispatches[0]
	dispatch.Candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{
			ID: "write-claim", Kind: runtimecontract.StepKindFileWrite,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-claim"}, TargetRefs: []string{"target.txt"},
		},
	}}}}
	aggregate.Facts.Evidence.Accepted[0].OperationID = "write-claim"
	aggregate.Facts.Evidence.Accepted[0].Validation = nil
	if _, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); !satisfied {
		t.Fatal("non-strict profile behavior changed")
	}
}

func TestCanonicalAssessmentCoverageDoesNotRewriteProducerObservation(t *testing.T) {
	aggregate, assessment := assessmentGateFixture("complete LSP tool implementation", "TestLSPToolExistence")
	aggregate.Facts.Evidence.Accepted[0].Validation.PassedChecks = nil
	assessment.Disposition = runtimecontract.AssessmentDispositionRepairableGap
	assessment.Gap = &runtimecontract.RuntimeAssessmentGap{
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-other"}, TargetRefs: []string{"other.txt"}, Reason: "another gap",
	}
	if _, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); satisfied {
		t.Fatal("incomplete owner validation closed canonical coverage")
	}
	if assessment.Disposition != runtimecontract.AssessmentDispositionRepairableGap || assessment.Gap == nil ||
		len(assessment.Gap.CoverageWorkItemIDs) != 1 || assessment.Gap.TargetRefs[0] != "other.txt" || assessment.Gap.Reason != "another gap" {
		t.Fatalf("canonical quality evaluation rewrote producer observation: %+v", assessment)
	}
}

func TestCommitAssessmentAcceptsStructuredClaimEvidenceWithoutTextSimilarity(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := validationQualityFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.RequireDirectEvidence = true
	facts.Focus.RequireClaimEvidence = true
	facts.Focus.RequireExecutableBehaviorEvidence = true
	facts.Coverage[0].TargetText = "complete LSP tool implementation"
	facts.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus = 2
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, managedValidationCandidate("validate-claim"))
	if err != nil {
		t.Fatal(err)
	}
	commit := ExecutionEvidence{
		EventID: "evidence-claim", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "validate-claim",
		Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusAccepted,
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: "validation completed", Quality: common.EvidenceQualityValid,
			Validation: &runtimecontract.RuntimeValidationObservation{
				ExecutedChecks: []string{"TestLSPToolExistence"}, PassedChecks: []string{"TestLSPToolExistence"},
			},
		},
	}
	attachRuntimeV2TestOwnerReceipt(t, repository.aggregate, &commit)
	evidence, err := controller.CommitEvidence(context.Background(), commit)
	if err != nil || evidence.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("validation evidence did not reach assessment: %+v err=%v", evidence.Aggregate.Decision, err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "claim is satisfied", EvidenceRefs: []string{"evidence-claim"},
		Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: []string{"evidence-claim"}}},
	}}); err != nil {
		t.Fatal(err)
	}
	progressed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Decision.State != runtimecontract.ConvergenceStateComplete || progressed.Decision.Terminal == nil ||
		len(progressed.Facts.Assessments) != 1 || progressed.Facts.Assessments[0].Disposition != runtimecontract.AssessmentDispositionSatisfied ||
		progressed.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateSatisfied {
		t.Fatalf("structured claim evidence did not close coverage: %+v", progressed)
	}
}

func assessmentGateFixture(claim, passedCheck string) (RuntimeAggregate, runtimecontract.AssessmentCandidate) {
	operation := runtimecontract.RuntimeOperation{
		ID: "validate-claim", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-claim"}, TargetRefs: []string{"target.txt"},
	}
	observation := &runtimecontract.RuntimeValidationObservation{ExecutedChecks: []string{passedCheck}, PassedChecks: []string{passedCheck}}
	return RuntimeAggregate{
			Facts: runtimecontract.RuntimeFacts{
				Focus: runtimecontract.CoverageFocus{
					ID: "focus-claim", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-claim"},
					AcceptedStepKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
					RequireDirectEvidence: true, RequireClaimEvidence: true, RequireExecutableBehaviorEvidence: true,
				},
				Coverage: []runtimecontract.RuntimeCoverageItemFact{{
					ID: "cwi-claim", State: runtimecontract.RuntimeCoverageItemStateOpen, TargetText: claim,
				}},
				Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{
					ID: "evidence-claim", Kind: runtimecontract.EvidenceKindValidation, DispatchID: "dispatch-validation", OperationID: "validate-claim",
					SourceRef: "runtime://validate-claim", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-claim"},
					Quality: common.EvidenceQualityValid, Summary: "validation completed", Validation: observation,
				}}},
			},
			CompletedDispatches: []SealedDispatch{{
				ID:        "dispatch-validation",
				Candidate: runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: operation}}}},
			}},
		}, runtimecontract.AssessmentCandidate{
			FocusID: "focus-claim", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-claim"},
			Disposition: runtimecontract.AssessmentDispositionSatisfied, EvidenceRefs: []string{"evidence-claim"},
			Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-claim", Satisfied: true, EvidenceRefs: []string{"evidence-claim"}}},
		}
}
