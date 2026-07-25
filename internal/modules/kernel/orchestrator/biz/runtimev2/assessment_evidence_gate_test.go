package runtimev2

import (
	"context"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAssessmentEvidenceGateRejectsWeakAcceptedEvidence(t *testing.T) {
	aggregate, assessment := assessmentGateFixture("behavior is verified", "TestBehavior")
	aggregate.Facts.Evidence.Accepted[0].Quality = common.EvidenceQualityInconclusive
	aggregate.Facts.Evidence.Accepted[0].ZeroWork = true
	got := applyAssessmentEvidenceGate(aggregate, assessment)
	assertAssessmentEvidenceGap(t, got, "zero-work")
}

func TestAssessmentEvidenceGateRejectsManagedValidationWithoutPassedChecks(t *testing.T) {
	aggregate, assessment := assessmentGateFixture("behavior is verified", "")
	got := applyAssessmentEvidenceGate(aggregate, assessment)
	assertAssessmentEvidenceGap(t, got, "does not show executed behavior")
}

func TestAssessmentEvidenceGateRejectsGenericExistenceCheckForCompleteImplementationClaim(t *testing.T) {
	aggregate, assessment := assessmentGateFixture("complete LSP tool implementation", "TestLSPToolExistence")
	got := applyAssessmentEvidenceGate(aggregate, assessment)
	assertAssessmentEvidenceGap(t, got, "not aligned")
}

func TestAssessmentEvidenceGateRequiresNumericClaimSignal(t *testing.T) {
	aggregate, assessment := assessmentGateFixture("request latency stays under 50ms", "TestRequestLatency")
	got := applyAssessmentEvidenceGate(aggregate, assessment)
	assertAssessmentEvidenceGap(t, got, "not aligned")
}

func TestAssessmentEvidenceGateAcceptsAlignedExecutedNumericEvidence(t *testing.T) {
	aggregate, assessment := assessmentGateFixture("request latency stays under 50ms", "TestRequestLatencyUnder50ms")
	got := applyAssessmentEvidenceGate(aggregate, assessment)
	if got.Disposition != runtimecontract.AssessmentDispositionSatisfied || !got.Items[0].Satisfied || got.Gap != nil {
		t.Fatalf("aligned executable evidence was rejected: %+v", got)
	}
}

func TestAssessmentEvidenceGateSkipsAlignmentForShortLowInformationTitle(t *testing.T) {
	aggregate, assessment := assessmentGateFixture("LSP 工具", "TestToolBehavior")
	got := applyAssessmentEvidenceGate(aggregate, assessment)
	if got.Disposition != runtimecontract.AssessmentDispositionSatisfied || !got.Items[0].Satisfied {
		t.Fatalf("short title should retain the basic evidence gate only: %+v", got)
	}
}

func TestAssessmentEvidenceGatePreservesNeutralBehaviorWhenStrictFlagsAreDisabled(t *testing.T) {
	aggregate, assessment := assessmentGateFixture("unrelated concrete claim", "")
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
	got := applyAssessmentEvidenceGate(aggregate, assessment)
	if got.Disposition != runtimecontract.AssessmentDispositionSatisfied || !got.Items[0].Satisfied {
		t.Fatalf("non-strict profile behavior changed: %+v", got)
	}
}

func TestAssessmentEvidenceGateMergesQualityFailureIntoExistingRepairableGap(t *testing.T) {
	aggregate, assessment := assessmentGateFixture("complete LSP tool implementation", "TestLSPToolExistence")
	assessment.Disposition = runtimecontract.AssessmentDispositionRepairableGap
	assessment.Gap = &runtimecontract.RuntimeAssessmentGap{
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-other"}, TargetRefs: []string{"other.txt"}, Reason: "another gap",
	}
	got := applyAssessmentEvidenceGate(aggregate, assessment)
	if got.Gap == nil || len(got.Gap.CoverageWorkItemIDs) != 2 || got.Gap.TargetRefs[0] != "other.txt" ||
		!strings.Contains(got.Gap.Reason, "another gap") || !strings.Contains(got.Gap.Reason, "not aligned") {
		t.Fatalf("quality gap did not preserve existing repair context: %+v", got)
	}
}

func TestCommitAssessmentPersistsClaimEvidenceGapAndKeepsRunRepairable(t *testing.T) {
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
	if err != nil || evidence.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceAssessment {
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
	progressed, err := controller.CommitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Decision.Action != runtimecontract.RuntimeActionProduceValidation || progressed.Decision.Terminal != nil ||
		len(progressed.Facts.Assessments) != 1 || progressed.Facts.Assessments[0].Disposition != runtimecontract.AssessmentDispositionRepairableGap ||
		progressed.Facts.Assessments[0].Gap == nil || progressed.Facts.Assessments[0].Gap.FailureCode != "assessment_evidence_quality_gap" ||
		progressed.Facts.Coverage[0].State == runtimecontract.RuntimeCoverageItemStateSatisfied {
		t.Fatalf("claim evidence gap did not remain repairable: %+v", progressed)
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

func assertAssessmentEvidenceGap(t *testing.T, assessment runtimecontract.AssessmentCandidate, reason string) {
	t.Helper()
	if assessment.Disposition != runtimecontract.AssessmentDispositionRepairableGap || assessment.Items[0].Satisfied || assessment.Gap == nil ||
		assessment.Gap.FailureClass != runtimecontract.FailureClassCoverageAssessmentIncomplete || assessment.Gap.FailureCode != "assessment_evidence_quality_gap" ||
		!strings.Contains(assessment.Gap.Reason, reason) {
		t.Fatalf("assessment evidence gate result = %+v", assessment)
	}
}
