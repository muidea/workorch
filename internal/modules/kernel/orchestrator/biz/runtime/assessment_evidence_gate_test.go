package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCanonicalAssessmentCoverageRejectsWeakAcceptedEvidence(t *testing.T) {
	aggregate, _ := assessmentGateFixture(t, "behavior is verified", "TestBehavior")
	aggregate.Facts.Evidence.Accepted[0].Quality = common.EvidenceQualityInconclusive
	aggregate.Facts.Evidence.Accepted[0].ZeroWork = true
	if refs, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); satisfied || len(refs) != 0 {
		t.Fatalf("weak owner evidence closed canonical coverage: refs=%v satisfied=%t", refs, satisfied)
	}
}

func TestCanonicalAssessmentCoverageRejectsManagedValidationWithoutPassedChecks(t *testing.T) {
	aggregate, _ := assessmentGateFixture(t, "behavior is verified", "")
	if refs, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); satisfied || len(refs) != 0 {
		t.Fatalf("zero-check owner evidence closed canonical coverage: refs=%v satisfied=%t", refs, satisfied)
	}
}

func TestCanonicalAssessmentCoverageDoesNotUseClaimText(t *testing.T) {
	aggregate, _ := assessmentGateFixture(t, "complete LSP tool implementation", "TestLSPToolExistence")
	if refs, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); !satisfied || len(refs) != 1 || refs[0] != "evidence-claim" {
		t.Fatalf("structured owner evidence did not close coverage: refs=%v satisfied=%t", refs, satisfied)
	}
}

func TestCanonicalAssessmentCoverageDoesNotUseNumericClaimText(t *testing.T) {
	aggregate, _ := assessmentGateFixture(t, "request latency stays under 50ms", "TestRequestLatency")
	if _, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); !satisfied {
		t.Fatal("numeric claim text altered owner-evidence coverage decision")
	}
}

func TestCanonicalAssessmentCoverageAcceptsStructuredExecutedEvidence(t *testing.T) {
	aggregate, _ := assessmentGateFixture(t, "request latency stays under 50ms", "TestRequestLatencyUnder50ms")
	if _, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); !satisfied {
		t.Fatal("structured executable owner evidence did not close coverage")
	}
}

func TestCanonicalAssessmentCoverageAcceptsCrossLanguageStructuredEvidence(t *testing.T) {
	aggregate, _ := assessmentGateFixture(t, "LSP 工具", "TestToolBehavior")
	if _, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, "cwi-claim"); !satisfied {
		t.Fatal("cross-language structured owner evidence did not close coverage")
	}
}

func TestCanonicalAssessmentCoveragePreservesNeutralBehaviorWhenStrictFlagsAreDisabled(t *testing.T) {
	aggregate, _ := assessmentGateFixture(t, "unrelated concrete claim", "")
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
	aggregate, assessment := assessmentGateFixture(t, "complete LSP tool implementation", "TestLSPToolExistence")
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
	facts.Focus.ValidationRecipeID = runtimeManagedValidationEvidencePolicy().ID
	facts.Focus.RequiredCheckRefs = []string{"TestLSPToolExistence"}
	for catalogIndex := range facts.Resources.OperationAdmissibilities {
		catalog := &facts.Resources.OperationAdmissibilities[catalogIndex]
		if catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			continue
		}
		for surfaceIndex := range catalog.ValidationSurfaces {
			surface := &catalog.ValidationSurfaces[surfaceIndex]
			if surface.ID == runtimeManagedValidationEvidencePolicy().ID {
				surface.CheckRefs = []string{"TestLSPToolExistence"}
			}
		}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	binding, found := coverageBindingForRuntimeCWI(aggregate.Facts.Focus, "cwi-v2")
	if !found {
		t.Fatalf("sealed assessment fixture has no current CWI binding: %+v", aggregate.Facts.Focus)
	}
	validationProof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
	if !found {
		t.Fatalf("sealed assessment fixture has no exact validation proof: %+v", binding)
	}
	if len(validationProof.ValidationCheckAttestations) == 0 {
		t.Fatalf("sealed assessment fixture has no owner-native check attestations: %+v", validationProof)
	}
	protocolRef := validationProof.ValidationCheckAttestations[0].ObservationProtocolRef
	nativeChecks := make([]string, 0, len(validationProof.ValidationCheckAttestations))
	for _, attestation := range validationProof.ValidationCheckAttestations {
		if attestation.ObservationProtocolRef != protocolRef {
			t.Fatalf("sealed assessment fixture uses multiple observation protocols: %+v", validationProof.ValidationCheckAttestations)
		}
		nativeChecks = append(nativeChecks, attestation.NativeCheckRef)
	}
	candidate := managedValidationCandidate("validate-claim")
	candidate.Validation.Operations[0].Operation.BindingRefs = []string{binding.ID}
	candidate.Validation.Operations[0].Operation.CoverageWorkItemIDs = nil
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	commit := ExecutionEvidence{
		EventID: "evidence-claim", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: runtimeTestActiveOperationID(t, sealed, 0),
		Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusAccepted,
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: "validation completed", Quality: common.EvidenceQualityValid,
			Validation: &runtimecontract.RuntimeValidationObservation{
				RecipeID:               runtimeManagedValidationEvidencePolicy().ID,
				ObservationProtocolRef: protocolRef,
				ExecutedChecks:         append([]string(nil), nativeChecks...),
				PassedChecks:           append([]string(nil), nativeChecks...),
			},
		},
	}
	attachRuntimeTestOwnerReceipt(t, repository.aggregate, &commit)
	evidence, err := controller.commitEvidence(context.Background(), commit)
	if err != nil || evidence.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("validation evidence did not reach assessment: %+v err=%v", evidence.Aggregate.Decision, err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: evidence.Aggregate.Facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
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

func assessmentGateFixture(t *testing.T, claim, passedCheck string) (RuntimeAggregate, runtimecontract.AssessmentCandidate) {
	t.Helper()
	const bindingRef = "binding/cwi-claim"
	operation := runtimecontract.RuntimeOperation{
		ID: "validate-claim", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		BindingRefs: []string{bindingRef}, TargetRefs: []string{"target.txt"},
	}
	requiredChecks := []string(nil)
	if passedCheck != "" {
		requiredChecks = []string{passedCheck}
	} else {
		requiredChecks = []string{"TestRequiredClaim"}
	}
	binding := assessmentTestBinding(
		bindingRef, "cwi-claim", "focus-claim", "definition-claim",
		[]string{runtimecontract.EvidenceKindValidation}, []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		[]string{runtimecontract.RuntimeBlockKindManagedValidationSession}, []string{"target.txt"}, "test-recipe", requiredChecks,
	)
	validationProof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
	if !found {
		t.Fatal("assessment fixture has no managed-validation proof")
	}
	observation := &runtimecontract.RuntimeValidationObservation{RecipeID: "test-recipe"}
	if len(validationProof.ValidationCheckAttestations) != 0 {
		observation.ObservationProtocolRef = validationProof.ValidationCheckAttestations[0].ObservationProtocolRef
	}
	if passedCheck != "" {
		for _, attestation := range validationProof.ValidationCheckAttestations {
			if attestation.RequiredCheckRef != passedCheck {
				continue
			}
			observation.ExecutedChecks = []string{attestation.NativeCheckRef}
			observation.PassedChecks = []string{attestation.NativeCheckRef}
			break
		}
		if !runtimecontract.RuntimeValidationObservationSatisfiesCoverageProof(validationProof, *observation) {
			t.Fatalf("assessment fixture observation does not satisfy managed proof: proof=%+v observation=%+v", validationProof, *observation)
		}
	}
	aggregate := RuntimeAggregate{
		ResultFeedbackVersion: runtimecontract.RuntimeResultFeedbackVersionCurrent,
		Facts: runtimecontract.RuntimeFacts{
			DefinitionRevision: "definition-claim",
			Focus: runtimecontract.CoverageFocus{
				ID: "focus-claim", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-claim"},
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
				AcceptedStepKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				RequireDirectEvidence: true, RequireClaimEvidence: true, RequireExecutableBehaviorEvidence: true,
				CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding},
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
	}
	runtimeTestSynchronizeCanonicalCWILedger(t, &aggregate)
	return aggregate, runtimecontract.AssessmentCandidate{
		FocusID: "focus-claim", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-claim"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, EvidenceRefs: []string{"evidence-claim"},
		Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-claim", Satisfied: true, EvidenceRefs: []string{"evidence-claim"}}},
	}
}

func assessmentTestBinding(id string, coverageID common.CoverageWorkItemID, focusID, definitionRevision string, _ []string, scopes []runtimecontract.RuntimeEffectScope, operationKinds, targetRefs []string, recipeID string, checks []string) runtimecontract.CoverageEvidenceBinding {
	attestations := make([]runtimecontract.RuntimeValidationCheckAttestationBinding, 0, len(checks))
	for index, checkRef := range checks {
		attestations = append(attestations, runtimecontract.RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef:       checkRef,
			ObservationProtocolRef: "test/assessment-validation-observation/v1",
			NativeCheckRef:         fmt.Sprintf("test/native/assessment/%d", index),
		})
	}
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: id, CoverageWorkItemID: coverageID, FocusID: focusID, DefinitionRevision: definitionRevision,
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: scopes, AllowedOperationKinds: operationKinds,
			ValidationRecipeID: recipeID, ValidationProofRequirementHash: "runtime-validation-proof-requirement-test-cwi-claim", RequiredCheckRefs: checks,
			ValidationCheckAttestations: attestations,
		}},
		TargetRefs: targetRefs, DeclarationRef: "test/assessment-proof",
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	return binding
}
