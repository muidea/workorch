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
	aggregate.Facts.Evidence.Accepted[0].BindingRefs = nil
	aggregate.Facts.Evidence.Accepted[0].Validation = nil
	if _, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); satisfied {
		t.Fatal("unbound state-change evidence closed coverage when strict assessment flags were disabled")
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
	facts := validationQualityFacts(t)
	facts.Focus.RequireAssessment = true
	facts.Focus.RequireDirectEvidence = true
	facts.Focus.RequireClaimEvidence = true
	facts.Focus.RequireExecutableBehaviorEvidence = true
	facts.Coverage[0].TargetText = "complete LSP tool implementation"
	facts.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus = 2
	facts.Focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{assessmentTestBinding(
		"binding/cwi-v2", "cwi-v2", facts.Focus.ID, facts.DefinitionRevision,
		[]string{runtimecontract.EvidenceKindValidation}, []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		[]string{runtimecontract.RuntimeBlockKindManagedValidationSession}, []string{"target.txt"},
		runtimeV2ManagedValidationEvidencePolicy().ID, []string{"TestLSPToolExistence"},
	)}
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate := managedValidationCandidate("validate-claim")
	candidate.Validation.Operations[0].Operation.BindingRefs = []string{"binding/cwi-v2"}
	candidate.Validation.Operations[0].Operation.CoverageWorkItemIDs = nil
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
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
				RecipeID: runtimeV2ManagedValidationEvidencePolicy().ID, ExecutedChecks: []string{"TestLSPToolExistence"}, PassedChecks: []string{"TestLSPToolExistence"},
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
	const bindingRef = "binding/cwi-claim"
	operation := runtimecontract.RuntimeOperation{
		ID: "validate-claim", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		BindingRefs: []string{bindingRef}, TargetRefs: []string{"target.txt"},
	}
	observation := &runtimecontract.RuntimeValidationObservation{RecipeID: "test-recipe", ExecutedChecks: []string{passedCheck}, PassedChecks: []string{passedCheck}}
	requiredChecks := []string(nil)
	if passedCheck != "" {
		requiredChecks = []string{passedCheck}
	}
	return RuntimeAggregate{
			Facts: runtimecontract.RuntimeFacts{
				DefinitionRevision: "definition-claim",
				Focus: runtimecontract.CoverageFocus{
					ID: "focus-claim", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-claim"},
					AcceptedStepKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
					RequireDirectEvidence: true, RequireClaimEvidence: true, RequireExecutableBehaviorEvidence: true,
					CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{assessmentTestBinding(
						bindingRef, "cwi-claim", "focus-claim", "definition-claim",
						[]string{runtimecontract.EvidenceKindValidation}, []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
						[]string{runtimecontract.RuntimeBlockKindManagedValidationSession}, []string{"target.txt"}, "test-recipe", requiredChecks,
					)},
				},
				Coverage: []runtimecontract.RuntimeCoverageItemFact{{
					ID: "cwi-claim", State: runtimecontract.RuntimeCoverageItemStateOpen, TargetText: claim,
				}},
				Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{
					ID: "evidence-claim", Kind: runtimecontract.EvidenceKindValidation, DispatchID: "dispatch-validation", OperationID: "validate-claim",
					SourceRef: "runtime://validate-claim", BindingRefs: []string{bindingRef}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-claim"},
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

func assessmentTestBinding(id string, coverageID common.CoverageWorkItemID, focusID, definitionRevision string, _ []string, scopes []runtimecontract.RuntimeEffectScope, operationKinds, targetRefs []string, recipeID string, checks []string) runtimecontract.CoverageEvidenceBinding {
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: id, CoverageWorkItemID: coverageID, FocusID: focusID, DefinitionRevision: definitionRevision,
		Proofs:     []runtimecontract.CoverageEvidenceProof{{EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: scopes, AllowedOperationKinds: operationKinds, ValidationRecipeID: recipeID, RequiredCheckRefs: checks}},
		TargetRefs: targetRefs, DeclarationRef: "test/assessment-proof",
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	return binding
}
