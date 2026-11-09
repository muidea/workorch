package runtime

import (
	"context"
	"reflect"
	"strings"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only the uncommitted owner result is changed. The evidence CAS below admits
// it with the same dispatch, execution key, fence and original input reads.
func runtimeUnavailableDiagnosticFixture(t *testing.T, cause rc.RuntimeValidationDiagnosticUnavailability) (*durableRuntimeStateRepository, *DispatchCoordinator, ExecutionEvidence, RuntimeAggregate) {
	t.Helper()
	repo, coordinator, e, before := runtimeFailureInputReviewFixture(t, false, true)
	e.Failure.ValidationDiagnostic = &rc.ValidationFailureDiagnostic{SchemaVersion: rc.ValidationFailureDiagnosticSchemaVersion, Class: rc.ValidationDiagnosticOutputInconclusive, Unavailability: cause, OwnerCode: "owner_diagnostic_incomplete", Reason: "exact owner diagnostic could not complete", Stage: "ordinary-test", Operation: rc.RuntimeBlockKindManagedValidationSession, CommandKind: "go", ExitCode: 1, RecommendedEvidenceTargetType: rc.ValidationEvidenceTargetValidationOutput}
	e.Receipt.Feedback = runtimeOperationFeedbackForTest(t, repo.aggregate, e.OperationID, *e.Failure)
	for i := range repo.aggregate.OperationReceipts {
		if repo.aggregate.OperationReceipts[i].ExecutionKey == e.Receipt.ExecutionKey {
			repo.aggregate.OperationReceipts[i] = e.Receipt.Clone()
		}
	}
	for i := range repo.aggregate.ActiveDispatch.OperationExecutions {
		repo.aggregate.ActiveDispatch.OperationExecutions[i].OwnerReceiptHash = rc.StableRuntimeOperationReceiptHash(*e.Receipt)
	}
	return repo, coordinator, e, before
}

func TestUnavailableValidationDiagnosticStopsWithoutRepeatingValidation(t *testing.T) {
	for _, cause := range []rc.RuntimeValidationDiagnosticUnavailability{rc.ValidationDiagnosticAssertionsUnavailable, rc.ValidationDiagnosticSourcesUnavailable, rc.ValidationDiagnosticCapacityExceeded, rc.ValidationDiagnosticInputChanged} {
		t.Run(string(cause), func(t *testing.T) {
			repo, coordinator, e, _ := runtimeUnavailableDiagnosticFixture(t, cause)
			receipt, budget := e.Receipt.Clone(), repo.aggregate.ExecutionBudget.Clone()
			got, err := coordinator.commitEvidence(context.Background(), e)
			if err != nil {
				t.Fatal(err)
			}
			a := got.Aggregate
			if a.Decision.Terminal == nil || a.Decision.Terminal.Code != runtimeValidationFailureDiagnosticUnavailable || a.ActiveDispatch != nil || a.SemanticPlanning.FailureInputReview != nil || a.Facts.RepairDirective != nil || a.Facts.Resources.FailureTarget != nil {
				t.Fatalf("missing diagnostics entered blind retry/repair: %+v", a.Decision)
			}
			if !strings.Contains(a.Facts.Failure.Message, "cause="+string(cause)) || !strings.Contains(a.Facts.Failure.Message, "exit=1") || !rc.SameRuntimeOperationReceipt(a.OperationReceipts[len(a.OperationReceipts)-1], receipt) || !reflect.DeepEqual(budget, a.ExecutionBudget) || len(a.Facts.Evidence.Rejected) != 1 || a.Facts.Evidence.Rejected[0].FailureCode == runtimeValidationFailureDiagnosticUnavailable {
				t.Fatal("original owner failure or budget was rewritten")
			}
			a = runtimeTestEvidenceAggregateRoundTrip(t, a)
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
			repo.aggregate = a
			fresh, err := NewDispatchCoordinator(repo)
			if err != nil {
				t.Fatal(err)
			}
			duplicate, err := fresh.commitEvidence(context.Background(), e)
			if err != nil || !reflect.DeepEqual(ownerStateRecord(a), ownerStateRecord(duplicate.Aggregate)) {
				t.Fatal("duplicate changed terminal authority", err)
			}
		})
	}
}

func TestUnavailableBehaviorReviewsInputWithoutGrantingRepair(t *testing.T) {
	for _, cause := range []rc.RuntimeValidationDiagnosticUnavailability{rc.ValidationDiagnosticReproductionUnavailable, rc.ValidationDiagnosticBehaviorUnobserved, rc.ValidationDiagnosticExecutionUnavailable, rc.ValidationDiagnosticDeadlineExceeded} {
		t.Run(string(cause), func(t *testing.T) {
			repo, coordinator, e, before := runtimeUnavailableDiagnosticFixture(t, cause)
			committed, err := coordinator.commitEvidence(context.Background(), e)
			if err != nil {
				t.Fatal(err)
			}
			a := committed.Aggregate
			review := a.SemanticPlanning.FailureInputReview
			if review == nil || a.Decision.Terminal != nil || SemanticOperationAuthorityReady(a) || a.Facts.Resources.FailureTarget != nil {
				t.Fatalf("review depends on missing repair target: %+v", a.Facts.Failure)
			}
			if review.Feedback.Diagnostic.ValidationDiagnostic.Unavailability != cause || review.Feedback.Validation.CheckFailures[0].AssertionText != "expected 15, got 13" || !reflect.DeepEqual(a.Facts.Coverage[0].CurrentBindingRef, before.Facts.Coverage[0].CurrentBindingRef) {
				t.Fatal("review lost exact cause, native assertion or original binding")
			}
			a = runtimeTestEvidenceAggregateRoundTrip(t, a)
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
			for _, mutate := range []func(*RuntimeAggregate){func(v *RuntimeAggregate) { v.Facts.WorkspaceID = "other" }, func(v *RuntimeAggregate) {
				v.OperationReceipts[len(v.OperationReceipts)-1].Feedback.OriginalDiagnostic.ValidationDiagnostic.Unavailability = rc.ValidationDiagnosticInputChanged
			}, func(v *RuntimeAggregate) {
				v.SemanticPlanning.FailureInputReview.Feedback.Diagnostic.ValidationDiagnostic.Unavailability = rc.ValidationDiagnosticInputChanged
			}} {
				v := a.CloneRaw()
				mutate(&v)
				if v.Validate() == nil {
					t.Fatal("tampered review accepted")
				}
			}
			repo.aggregate = a
			duplicate, err := coordinator.commitEvidence(context.Background(), e)
			if err != nil || duplicate.Aggregate.StateRevision != a.StateRevision {
				t.Fatal("duplicate reopened review", err)
			}
		})
	}
}

func TestUnavailableBehaviorReviewConclusionControlsOnlyItsOwnAuthority(t *testing.T) {
	for _, conclusion := range []string{rc.RuntimeValidationInputConsistent, rc.RuntimeValidationInputConflict, rc.RuntimeValidationInputInconclusive} {
		t.Run(conclusion, func(t *testing.T) {
			repo, coordinator, e, _ := runtimeUnavailableDiagnosticFixture(t, rc.ValidationDiagnosticBehaviorUnobserved)
			if _, err := coordinator.commitEvidence(context.Background(), e); err != nil {
				t.Fatal(err)
			}
			controller, a := runtimeSubmitFailureInputReportForTest(t, repo, conclusion)
			attempts, budget := a.Facts.Repair.RepairAttempts, a.ExecutionBudget.Clone()
			updated, err := controller.requireSemanticFeasibility(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Facts.Repair.RepairAttempts != attempts || !reflect.DeepEqual(budget, updated.ExecutionBudget) {
				t.Fatal("review reset or charged execution budget")
			}
			switch conclusion {
			case rc.RuntimeValidationInputConsistent:
				if updated.Decision.Terminal == nil || updated.Decision.Terminal.Code != runtimeValidationFailureDiagnosticUnavailable || updated.SemanticPlanning.FailureInputReview != nil || updated.Facts.Resources.FailureTarget != nil || updated.Facts.RepairDirective != nil {
					t.Fatalf("consistent review manufactured repair: %+v", updated.Decision)
				}
			case rc.RuntimeValidationInputConflict:
				v, err := rc.PendingRuntimeValidationInputRevision(updated.SemanticPlanning, updated.ImplementationWork, updated.Facts.Focus.CoverageWorkItemIDs[0])
				if err != nil || v == nil || !reflect.DeepEqual(v.InputTargetRefs, []string{"input.go"}) || updated.Facts.Failure != nil || updated.Facts.RepairDirective != nil {
					t.Fatal("conflict bypassed protected input maintenance", err)
				}
			case rc.RuntimeValidationInputInconclusive:
				if updated.Decision.State != rc.ConvergenceStateNeedGovernance || updated.Facts.Failure == nil || updated.Facts.Failure.Code != "validation_input_review_inconclusive" {
					t.Fatal("inconclusive bypassed factual decision")
				}
			}
			if err := updated.Validate(); err != nil {
				t.Fatal("reload", err)
			}
		})
	}
}

func TestUnavailableDiagnosticReceiptCASRecovery(t *testing.T) {
	for _, cause := range []rc.RuntimeValidationDiagnosticUnavailability{rc.ValidationDiagnosticAssertionsUnavailable, rc.ValidationDiagnosticBehaviorUnobserved} {
		for _, mode := range []failureAuthorityCASMode{failureAuthorityCASFailBeforeWrite, failureAuthorityCASLoseResponse} {
			t.Run(string(cause)+"/"+map[failureAuthorityCASMode]string{failureAuthorityCASFailBeforeWrite: "before-write", failureAuthorityCASLoseResponse: "response-loss"}[mode], func(t *testing.T) {
				original, _, e, _ := runtimeUnavailableDiagnosticFixture(t, cause)
				repo := &failureAuthorityCASRepository{aggregate: original.aggregate.CloneRaw(), mode: mode}
				c, err := NewDispatchCoordinator(repo)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = c.commitEvidence(context.Background(), e); err == nil {
					t.Fatal("missing injected CAS failure")
				}
				repo.mode = failureAuthorityCASNormal
				resumed, _ := NewDispatchCoordinator(repo)
				got, err := resumed.commitEvidence(context.Background(), e)
				if err != nil {
					t.Fatal(err)
				}
				a := got.Aggregate
				if len(a.Facts.Evidence.Rejected) != 1 || a.Facts.Repair.RepairAttempts != 1 {
					t.Fatal("lost/doubled failed receipt")
				}
				if cause == rc.ValidationDiagnosticAssertionsUnavailable && (a.Decision.Terminal == nil || a.Decision.Terminal.Code != runtimeValidationFailureDiagnosticUnavailable) {
					t.Fatal("lost determined terminal")
				}
				if cause == rc.ValidationDiagnosticBehaviorUnobserved && a.SemanticPlanning.FailureInputReview == nil {
					t.Fatal("lost independent input review")
				}
				dup, err := resumed.commitEvidence(context.Background(), e)
				if err != nil || dup.Aggregate.StateRevision != a.StateRevision {
					t.Fatal("duplicate advanced authority", err)
				}
			})
		}
	}
}

func TestUnavailableBehaviorDoesNotResetExhaustedRepairBudget(t *testing.T) {
	repo, c, e, _ := runtimeUnavailableDiagnosticFixture(t, rc.ValidationDiagnosticBehaviorUnobserved)
	// The dispatch already exists; the receipt does not restore exhausted policy.
	repo.aggregate.Facts.Repair.RepairAttempts = 3
	got, err := c.commitEvidence(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Aggregate
	if a.SemanticPlanning.FailureInputReview != nil || a.Decision.Terminal == nil || a.Facts.Repair.RepairAttempts < 3 || a.Facts.Failure == nil || a.Facts.Failure.Code != "validation_failure_repair_exhausted" {
		t.Fatalf("review bypassed bounded budget: failure=%+v repair=%+v", a.Facts.Failure, a.Facts.Repair)
	}
}
