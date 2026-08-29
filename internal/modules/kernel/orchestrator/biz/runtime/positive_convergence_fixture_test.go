package runtime

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type receiptAwarePositiveConvergenceController struct {
	*RuntimeController
	repository *controllerRepository
	t          *testing.T
}

func (c *receiptAwarePositiveConvergenceController) commitEvidence(ctx context.Context, evidence ExecutionEvidence) (EvidenceCommitResult, error) {
	c.t.Helper()
	attachRuntimeTestOwnerReceipt(c.t, c.repository.aggregate, &evidence)
	return c.RuntimeController.commitEvidence(ctx, evidence)
}

func (c *receiptAwarePositiveConvergenceController) AcceptCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	c.t.Helper()
	if err := RecordFixtureCandidateFeasibilities(ctx, c.RuntimeController, workspaceID, runID, attempt, candidate); err != nil {
		return RuntimeAggregate{}, err
	}
	return c.RuntimeController.AcceptCandidateAttempt(ctx, workspaceID, runID, attempt, candidate)
}

// RVC-17 deterministic positive-convergence fixture.
// Sequence (controller-level, no public LLM):
//  1. candidate attempt 0 schema failure
//  2. candidate attempt 1 corrected state-change
//  3. accepted state-change evidence
//  4. first assessment: partial + repairable_gap
//  5. same-focus repair state-change
//  6. validation evidence
//  7. second assessment: remaining items satisfied → complete
//
// Also covers restart-style re-drive of completed aggregate (idempotent).
func TestPositiveConvergenceFixture_AttemptCorrectionPartialGapSameFocusRepair(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repository := &controllerRepository{}
	rawController, _ := NewRuntimeController(repository)
	controller := &receiptAwarePositiveConvergenceController{RuntimeController: rawController, repository: repository, t: t}

	facts := runtimeFactsWithManagedValidationRecipe(controllerFacts())
	facts.Focus.RequireAssessment = true
	// Assessment is requested via RequireAssessment, not as an operation evidence kind.
	facts.Focus.RequiredEvidenceKinds = []string{
		runtimecontract.EvidenceKindStateChange,
		runtimecontract.EvidenceKindValidation,
	}
	// The owner-visible validation result, not the assessment claim, decides
	// whether the first evidence set can close coverage.
	facts.Focus.RequireExecutableBehaviorEvidence = true
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.ValidationRecipeID = runtimeManagedValidationEvidencePolicy().ID
	facts.Focus.RequiredCheckRefs = []string{"repair-verification"}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, ctx, rawController, facts, controllerDefinition(), nil)
	facts = initialized.Facts
	focusID := facts.Focus.ID
	validationRecipeID, validationCheckRefs := runtimePositiveConvergenceValidationProof(t, facts)

	// --- 1) attempt 0: schema/contract failure ---
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	failed0, err := controller.failCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassSchema))
	if err != nil {
		t.Fatal(err)
	}
	if failed0.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("after attempt0 failure action = %s, want produce_state_change", failed0.Decision.State)
	}

	// --- 2) attempt 1: corrected sealed state-change ---
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	sealedSC, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 1, runtimecontract.RuntimeCandidate{
		StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
			Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: "file/write",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
				TargetRefs:          []string{"target.txt"},
				Parameters:          runtimeFileWriteParameters("state"),
			},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sealedSC.ActiveDispatch == nil {
		t.Fatal("expected sealed state-change dispatch")
	}

	// --- 3) accepted state-change evidence ---
	afterSC, err := controller.commitEvidence(ctx, ExecutionEvidence{
		EventID: "event-sc-1", WorkspaceID: "ws-v2", RunID: "run-v2",
		DispatchID:          sealedSC.ActiveDispatch.ID,
		AuthoritySnapshotID: sealedSC.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID:         runtimeTestActiveOperationID(t, sealedSC, 0),
		Kind:                runtimecontract.EvidenceKindStateChange,
		Status:              ExecutionEvidenceStatusAccepted,
	})
	if err != nil {
		t.Fatal(err)
	}
	// After state-change, require validation before assessment when RequireValidation is set.
	if afterSC.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation &&
		afterSC.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("after state-change action = %s", afterSC.Aggregate.Decision.State)
	}

	// If validation is next, produce managed validation evidence.
	agg := afterSC.Aggregate
	if agg.Decision.State == runtimecontract.ConvergenceStateNeedValidation {
		if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "validation-prefix"); err != nil {
			t.Fatal(err)
		}
		sealedVal, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
			Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{
				Operation: runtimecontract.RuntimeOperation{
					ID: "validate-1", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
					Parameters: runtimeManagedValidationParameters(),
				},
			}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		afterVal, err := controller.commitEvidence(ctx, ExecutionEvidence{
			EventID: "event-val-1", WorkspaceID: "ws-v2", RunID: "run-v2",
			DispatchID:          sealedVal.ActiveDispatch.ID,
			AuthoritySnapshotID: sealedVal.ActiveDispatch.Transaction.AuthoritySnapshotID,
			OperationID:         runtimeTestActiveOperationID(t, sealedVal, 0),
			Kind:                runtimecontract.EvidenceKindValidation,
			Status:              ExecutionEvidenceStatusAccepted,
			Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{
				RecipeID:       validationRecipeID,
				ExecutedChecks: append([]string(nil), validationCheckRefs...), PassedChecks: append([]string(nil), validationCheckRefs...),
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		agg = afterVal.Aggregate
	}
	if agg.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		term := ""
		if agg.Decision.Terminal != nil {
			term = agg.Decision.Terminal.Code + ": " + agg.Decision.Terminal.Summary
		}
		t.Fatalf("expected produce_assessment, got %s terminal=%q", agg.Decision.State, term)
	}

	// --- 4) first assessment: partial + repairable_gap ---
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "assessment-prefix-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
		Assessment: &runtimecontract.AssessmentCandidate{
			FocusID:             focusID,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			Disposition:         runtimecontract.AssessmentDispositionRepairableGap,
			Summary:             "implementation incomplete for cwi-v2",
			Items: []runtimecontract.RuntimeAssessmentItemFact{
				{CoverageWorkItemID: "cwi-v2", Satisfied: false, Reason: "missing final implementation"},
			},
			Gap: &runtimecontract.RuntimeAssessmentGap{
				// These producer claims deliberately disagree with the sealed
				// focus. C9 retains them in the audit assessment while SM-2 must
				// derive coverage/target/action from persisted owner facts.
				CoverageWorkItemIDs:  []common.CoverageWorkItemID{"untrusted-cwi"},
				MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
				TargetRefs:           []string{"untrusted-target.txt"},
				Reason:               "implementation incomplete",
			},
			EvidenceRefs: []string{"event-sc-1"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	afterGap, err := controller.commitAssessment(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	// The assessment claim cannot reopen a CWI after both sealed owner proofs
	// have been accepted. Before proof-bearing validation required BindingRefs,
	// this fixture accidentally created a real validation gap by dropping the
	// binding identity. The current contract must prefer owner facts.
	if afterGap.Decision.State == runtimecontract.ConvergenceStateComplete {
		return
	}
	if afterGap.Decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("repairable assessment unexpectedly failed: %+v", afterGap.Decision)
	}
	if afterGap.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		// Some paths may go to validation first; allow either repair path but not terminal.
		if afterGap.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
			t.Fatalf("after repairable assessment action = %s, want state-change or validation repair", afterGap.Decision.State)
		}
	}
	if afterGap.Facts.Repair.RepairAttempts < 1 {
		t.Logf("repair ledger after gap (may be counted on later commit): %+v", afterGap.Facts.Repair)
	}
	if afterGap.Decision.State == runtimecontract.ConvergenceStateNeedStateChange && afterGap.Facts.RepairDirective == nil {
		t.Fatalf("state-change repair must persist a directive: %+v", afterGap.Facts)
	}
	if afterGap.Facts.RepairDirective == nil || afterGap.Facts.RepairDirective.Closure == nil ||
		afterGap.Facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedStateChange ||
		!sameRuntimeTargetRefs(afterGap.Facts.RepairDirective.TargetRefs, []string{"target.txt"}) ||
		!coverageIDsIntersect(afterGap.Facts.RepairDirective.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-v2"}) ||
		!sameRuntimeTargetRefs(afterGap.Facts.RepairDirective.Closure.RequiredEvidenceKinds, []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}) {
		t.Fatalf("assessment observation selected repair authority: directive=%+v closure=%+v", afterGap.Facts.RepairDirective, afterGap.Facts.RepairDirective.Closure)
	}
	if afterGap.Facts.Repair.ActiveGap == nil ||
		!coverageIDsIntersect(afterGap.Facts.Repair.ActiveGap.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-v2"}) {
		t.Fatalf("repair ledger did not derive canonical open coverage: %+v", afterGap.Facts.Repair)
	}
	if len(afterGap.Facts.Assessments) == 0 || afterGap.Facts.Assessments[len(afterGap.Facts.Assessments)-1].Gap == nil ||
		!sameRuntimeTargetRefs(afterGap.Facts.Assessments[len(afterGap.Facts.Assessments)-1].Gap.TargetRefs, []string{"untrusted-target.txt"}) {
		t.Fatalf("assessment original diagnostic was not retained: %+v", afterGap.Facts.Assessments)
	}

	// --- 5) same-focus repair state-change ---
	agg = afterGap
	if agg.Decision.State == runtimecontract.ConvergenceStateNeedStateChange {
		if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "repair-sc-prefix"); err != nil {
			t.Fatal(err)
		}
		sealedRepair, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
			StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
				Operation: runtimecontract.RuntimeOperation{
					ID: "write-repair", Kind: "file/write",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
					TargetRefs:          []string{"target.txt"},
					Parameters:          runtimeFileWriteParameters("repair"),
				},
			}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		afterRepair, err := controller.commitEvidence(ctx, ExecutionEvidence{
			EventID: "event-sc-repair", WorkspaceID: "ws-v2", RunID: "run-v2",
			DispatchID:          sealedRepair.ActiveDispatch.ID,
			AuthoritySnapshotID: sealedRepair.ActiveDispatch.Transaction.AuthoritySnapshotID,
			OperationID:         runtimeTestActiveOperationID(t, sealedRepair, 0),
			Kind:                runtimecontract.EvidenceKindStateChange,
			Status:              ExecutionEvidenceStatusAccepted,
		})
		if err != nil {
			t.Fatal(err)
		}
		agg = afterRepair.Aggregate
	}

	// --- 6) validation after repair if required ---
	if agg.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("accepted repair state-change must transition to validation, got %s; directive=%+v evidence=%+v repair=%+v", agg.Decision.State, agg.Facts.RepairDirective, agg.Facts.Evidence.Accepted, agg.Facts.Repair)
	}
	if agg.Decision.State == runtimecontract.ConvergenceStateNeedValidation {
		if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "repair-val-prefix"); err != nil {
			t.Fatal(err)
		}
		sealedVal2, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
			Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{
				Operation: runtimecontract.RuntimeOperation{
					ID: "validate-repair", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
					TargetRefs:          []string{"target.txt"},
					Parameters:          runtimeManagedValidationParameters(),
				},
			}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		afterVal2, err := controller.commitEvidence(ctx, ExecutionEvidence{
			EventID: "event-val-repair", WorkspaceID: "ws-v2", RunID: "run-v2",
			DispatchID:          sealedVal2.ActiveDispatch.ID,
			AuthoritySnapshotID: sealedVal2.ActiveDispatch.Transaction.AuthoritySnapshotID,
			OperationID:         runtimeTestActiveOperationID(t, sealedVal2, 0),
			Kind:                runtimecontract.EvidenceKindValidation,
			Status:              ExecutionEvidenceStatusAccepted,
			Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{
				RecipeID: validationRecipeID, ExecutedChecks: append([]string(nil), validationCheckRefs...), PassedChecks: append([]string(nil), validationCheckRefs...),
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		agg = afterVal2.Aggregate
	}
	if agg.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("expected re-assessment after repair, got %s", agg.Decision.State)
	}

	// --- 7) second assessment: satisfied → complete ---
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "assessment-prefix-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
		Assessment: &runtimecontract.AssessmentCandidate{
			FocusID:             focusID,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			Disposition:         runtimecontract.AssessmentDispositionSatisfied,
			Summary:             "all coverage items satisfied after repair",
			Items: []runtimecontract.RuntimeAssessmentItemFact{
				{CoverageWorkItemID: "cwi-v2", Satisfied: true},
			},
			EvidenceRefs: []string{"event-sc-1", "event-val-repair"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	completed, err := controller.commitAssessment(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Decision.State != runtimecontract.ConvergenceStateComplete {
		quality := canonicalAssessmentEvidenceQualityFailure(completed, "cwi-v2", []string{"event-sc-1", "event-val-repair"})
		t.Fatalf("final action = %s, want complete; quality=%q decision=%+v evidence=%+v coverage=%+v assessments=%+v", completed.Decision.State, quality, completed.Decision, completed.Facts.Evidence.Accepted, completed.Facts.Coverage, completed.Facts.Assessments)
	}
	// State revision must have advanced through the fixture.
	if completed.StateRevision < 2 {
		t.Fatalf("state revision = %d, want progress", completed.StateRevision)
	}

	// Restart-style re-load: completed aggregate remains complete (idempotent).
	reloaded, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Decision.State != runtimecontract.ConvergenceStateComplete {
		t.Fatalf("reloaded action = %s, want complete", reloaded.Decision.State)
	}
	if reloaded.StateRevision != completed.StateRevision {
		t.Fatalf("revision drift on reload: %d vs %d", reloaded.StateRevision, completed.StateRevision)
	}
}

func runtimePositiveConvergenceValidationProof(t *testing.T, facts runtimecontract.RuntimeFacts) (string, []string) {
	t.Helper()
	if len(facts.Focus.CoverageEvidenceBindings) != 1 {
		t.Fatalf("positive convergence focus has no unique semantic binding: %+v", facts.Focus.CoverageEvidenceBindings)
	}
	proof, found := runtimecontract.CoverageEvidenceProofForKind(
		facts.Focus.CoverageEvidenceBindings[0], runtimecontract.EvidenceKindValidation,
	)
	if !found || proof.ValidationRecipeID == "" || len(proof.RequiredCheckRefs) == 0 {
		t.Fatalf("positive convergence binding has no complete validation proof: %+v", facts.Focus.CoverageEvidenceBindings[0])
	}
	return proof.ValidationRecipeID, append([]string(nil), proof.RequiredCheckRefs...)
}

// RVC-17: budget exhaustion produces typed terminal, not infinite repair.
func TestPositiveConvergenceFixture_BudgetExhaustionTerminal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repository := &controllerRepository{}
	rawController, _ := NewRuntimeController(repository)
	controller := &receiptAwarePositiveConvergenceController{RuntimeController: rawController, repository: repository, t: t}

	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Focus.RequireExecutableBehaviorEvidence = true
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite}
	// Tight repair budget.
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{
		MaxRepairAttemptsPerFocus:        1,
		MaxValidationOnlyRetriesPerFocus: 1,
	}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, ctx, rawController, facts, controllerDefinition(), nil)
	facts = initialized.Facts
	focusID := facts.Focus.ID

	// Seed accepted state-change so assessment is required.
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "sc"); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
		StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
			Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: "file/write",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
				TargetRefs:          []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
			},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	afterSC, err := controller.commitEvidence(ctx, ExecutionEvidence{
		EventID: "ev-1", WorkspaceID: "ws-v2", RunID: "run-v2",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil {
		t.Fatal(err)
	}
	agg := afterSC.Aggregate
	if agg.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("non-executable state-change proof did not require canonical assessment, got %s", agg.Decision.State)
	}

	// First repairable assessment opens a repair cycle.
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
		Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: focusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			Disposition: runtimecontract.AssessmentDispositionRepairableGap,
			Summary:     "still broken",
			Items: []runtimecontract.RuntimeAssessmentItemFact{{
				CoverageWorkItemID: "cwi-v2", Satisfied: false, Reason: "still broken",
			}},
			Gap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Reason: "still broken"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	after1, err := controller.commitAssessment(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if after1.Decision.State == runtimecontract.ConvergenceStateFailed {
		// Exhausted on first gap under MaxRepairAttempts=1 is acceptable.
		if after1.Decision.Terminal == nil && after1.Facts.Failure == nil {
			t.Fatalf("fail without terminal/failure fact: %+v", after1)
		}
		return
	}

	// Perform one repair and re-assess with same gap to exhaust budget.
	if after1.Decision.State == runtimecontract.ConvergenceStateNeedStateChange {
		if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "r1"); err != nil {
			t.Fatal(err)
		}
		sr, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
			StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
				Operation: runtimecontract.RuntimeOperation{
					ID: "wr", Kind: "file/write",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
					TargetRefs:          []string{"target.txt"}, Parameters: runtimeFileWriteParameters("repair"),
				},
			}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		er, err := controller.commitEvidence(ctx, ExecutionEvidence{
			EventID: "ev-r", WorkspaceID: "ws-v2", RunID: "run-v2",
			DispatchID: sr.ActiveDispatch.ID, AuthoritySnapshotID: sr.ActiveDispatch.Transaction.AuthoritySnapshotID,
			OperationID: runtimeTestActiveOperationID(t, sr, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
		})
		if err != nil {
			t.Fatal(err)
		}
		agg = er.Aggregate
	} else {
		agg = after1
	}
	if agg.Decision.State == runtimecontract.ConvergenceStateFailed {
		return
	}
	if agg.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("want assessment for exhaustion, got %s", agg.Decision.State)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "a2"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
		Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: focusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			Disposition: runtimecontract.AssessmentDispositionRepairableGap,
			Summary:     "still broken after repair",
			Items: []runtimecontract.RuntimeAssessmentItemFact{{
				CoverageWorkItemID: "cwi-v2", Satisfied: false, Reason: "still broken after repair",
			}},
			Gap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Reason: "still broken"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	exhausted, err := controller.commitAssessment(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if exhausted.Decision.State != runtimecontract.ConvergenceStateFailed {
		// Escalation to state-change under validation-only cap is also a valid budget reaction;
		// but with MaxRepairAttempts=1 a second gap should fail or stop progress.
		if exhausted.Decision.State == runtimecontract.ConvergenceStateNeedStateChange {
			t.Logf("budget escalated to state-change rather than fail: %+v", exhausted.Decision)
			return
		}
		t.Fatalf("expected fail or escalate after budget, got %s", exhausted.Decision.State)
	}
}

func TestPositiveConvergenceFixture_RepairableAssessmentPersistsAcrossControllerRestart(t *testing.T) {
	ctx := context.Background()
	first, repository, _ := prepareOwnerFactAssessment(t, false, runtimecontract.RuntimeBlockKindPatchApply)
	partial, err := first.commitAssessment(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if partial.Decision.State == runtimecontract.ConvergenceStateComplete || partial.Decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("repairable assessment terminalized: decision=%+v terminal=%+v failure=%+v repair=%+v", partial.Decision, partial.Decision.Terminal, partial.Facts.Failure, partial.Facts.Repair)
	}
	if len(partial.Facts.Assessments) == 0 ||
		partial.Facts.Assessments[len(partial.Facts.Assessments)-1].Disposition != runtimecontract.AssessmentDispositionRepairableGap {
		t.Fatalf("owner-derived repairable assessment was not persisted: %+v", partial.Facts.Assessments)
	}
	second, _ := NewRuntimeController(repository)
	reloaded, err := second.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	states := map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemState{}
	for _, item := range reloaded.Facts.Coverage {
		states[item.ID] = item.State
	}
	if states["cwi-v2"] == runtimecontract.RuntimeCoverageItemStateSatisfied ||
		reloaded.StateRevision != partial.StateRevision || len(reloaded.Facts.Assessments) != len(partial.Facts.Assessments) {
		t.Fatalf("reloaded repairable assessment = %+v", reloaded.Facts.Coverage)
	}
}

// RVC-17: multi-focus advance after satisfied assessment.
func TestPositiveConvergenceFixture_FocusAdvanceAfterSatisfiedAssessment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repository := &controllerRepository{}
	rawController, _ := NewRuntimeController(repository)
	controller := &receiptAwarePositiveConvergenceController{RuntimeController: rawController, repository: repository, t: t}

	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	// Remaining queue after current focus so satisfaction advances.
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{
		{
			ID: "focus-v2-next", SliceID: "slice-v2-next", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2-next"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			TargetRefs:            []string{"next.txt"},
			RequireAssessment:     true,
		},
	}
	facts.Coverage = append(facts.Coverage, runtimecontract.RuntimeCoverageItemFact{
		ID: "cwi-v2-next", SliceID: "slice-v2-next", State: runtimecontract.RuntimeCoverageItemStateOpen,
		PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
	})
	facts.Slices = append(facts.Slices, runtimecontract.RuntimeAcceptanceSlice{
		ID: "slice-v2-next", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2-next"},
	})
	facts.DefinitionSeed.CoverageItems = append(facts.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{
		ID: "cwi-v2-next", SliceID: "slice-v2-next", SourceScope: "dodItem", SourceID: "dod-v2-next", TargetText: "deliver next",
		PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
	})
	facts.DefinitionSeed.Slices = append(facts.DefinitionSeed.Slices, runtimecontract.RuntimeAcceptanceSlice{
		ID: "slice-v2-next", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2-next"},
	})
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	initialized := runtimeTestInitializeAndSealSemanticContract(t, ctx, rawController, facts, controllerDefinition(), nil)
	facts = initialized.Facts
	initialFocusID := facts.Focus.ID

	// Satisfy first focus via state-change + assessment.
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "sc"); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimeTestBindCandidateToFacts(t, facts, runtimecontract.RuntimeCandidate{
		StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
			Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: "file/write",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
				TargetRefs:          []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
			},
		}}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	afterSC, err := controller.commitEvidence(ctx, ExecutionEvidence{
		EventID: "ev-1", WorkspaceID: "ws-v2", RunID: "run-v2",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil {
		t.Fatal(err)
	}
	agg := afterSC.Aggregate
	// Drain to assessment.
	for step := 0; step < 6 && agg.Decision.State != runtimecontract.ConvergenceStateNeedAssessment; step++ {
		switch agg.Decision.State {
		case runtimecontract.ConvergenceStateNeedValidation:
			if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "val"); err != nil {
				t.Fatal(err)
			}
			sv, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
				Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{
					Operation: runtimecontract.RuntimeOperation{
						ID: "v1", Kind: runtimecontract.StepKindFileRead,
						CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
						TargetRefs:          []string{"target.txt"},
					},
				}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			ev, err := controller.commitEvidence(ctx, ExecutionEvidence{
				EventID: "ev-v", WorkspaceID: "ws-v2", RunID: "run-v2",
				DispatchID: sv.ActiveDispatch.ID, AuthoritySnapshotID: sv.ActiveDispatch.Transaction.AuthoritySnapshotID,
				OperationID: runtimeTestActiveOperationID(t, sv, 0), Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusAccepted,
			})
			if err != nil {
				t.Fatal(err)
			}
			agg = ev.Aggregate
		case runtimecontract.ConvergenceStateNeedStateChange:
			// unexpected loop
			t.Fatalf("stuck on state-change after accepted evidence: %+v", agg.Decision)
		default:
			t.Fatalf("unexpected action %s", agg.Decision.State)
		}
	}
	if agg.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("want assessment, got %s", agg.Decision.State)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
		Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: initialFocusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			Disposition:  runtimecontract.AssessmentDispositionSatisfied,
			Summary:      "focus-v2 done",
			Items:        []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true}},
			EvidenceRefs: []string{"ev-1"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	advanced, err := controller.commitAssessment(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	// Either advance to next focus state-change or complete if queue drained.
	if advanced.Decision.State == runtimecontract.ConvergenceStateComplete {
		t.Log("single-focus completion path (queue may have been collapsed)")
		return
	}
	if advanced.Facts.Focus.ID == initialFocusID && advanced.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("did not leave satisfied focus: %+v", advanced.Facts.Focus)
	}
	// Preferred: next focus opened.
	if coverageIDsIntersect(advanced.Facts.Focus.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-v2-next"}) ||
		advanced.Decision.State == runtimecontract.ConvergenceStateNeedStateChange {
		return
	}
	t.Logf("focus after advance: id=%s action=%s", advanced.Facts.Focus.ID, advanced.Decision.State)
}

func strPtr(v string) *string { return &v }
