package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRejectedConfirmedRepairClosesAtomicallyAcrossCASAndRestart(t *testing.T) {
	ctx := context.Background()
	for _, partial := range []bool{false, true} {
		name := "first_member"
		if partial {
			name = "after_partial_effect"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newConfirmedRepairMultiOperationFixture(t)
			coordinator, err := NewDispatchCoordinator(fixture.repository)
			if err != nil {
				t.Fatal(err)
			}
			if partial {
				first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
				attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
				if _, err := coordinator.commitEvidence(ctx, first); err != nil {
					t.Fatal(err)
				}
			}
			dispatch := *fixture.repository.aggregate.ActiveDispatch
			operations, err := orderedCandidateOperations(dispatch.Candidate)
			if err != nil {
				t.Fatal(err)
			}
			operation := operations[0]
			if partial {
				operation = operations[1]
			}
			rejected := ExecutionEvidence{
				EventID:     "runtime-operation|" + dispatch.ID + "|" + operation.ID,
				WorkspaceID: fixture.repository.aggregate.Facts.WorkspaceID, RunID: fixture.repository.aggregate.Facts.RunID,
				DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
				OperationID: operation.ID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusRejected,
				SourceRef: "repair/rejected",
				Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "owner_rejected_effect",
					Signature: "repair-rejected", Message: "owner rejected the sealed edit", TargetRefs: operation.TargetRefs},
			}
			attachRuntimeTestOwnerReceipt(t, &fixture.repository.aggregate, &rejected)
			before := cloneRuntimeAggregateForMCPProvenanceTest(t, fixture.repository.aggregate)
			for name, mode := range map[string]failureAuthorityCASMode{
				"normal": failureAuthorityCASNormal, "before_write": failureAuthorityCASFailBeforeWrite, "lost_response": failureAuthorityCASLoseResponse,
			} {
				t.Run(name, func(t *testing.T) {
					repository := &failureAuthorityCASRepository{aggregate: cloneRuntimeAggregateForMCPProvenanceTest(t, before), mode: mode}
					coordinator, err := NewDispatchCoordinator(repository)
					if err != nil {
						t.Fatal(err)
					}
					_, applyErr := coordinator.commitEvidence(ctx, rejected)
					if mode == failureAuthorityCASFailBeforeWrite {
						if !errors.Is(applyErr, errFailureAuthorityCASCut) || !reflect.DeepEqual(before, repository.aggregate) {
							t.Fatalf("pre-CAS cut tore repair state: err=%v", applyErr)
						}
						repository.mode = failureAuthorityCASNormal
						if _, err := coordinator.commitEvidence(ctx, rejected); err != nil {
							t.Fatal(err)
						}
					} else if mode == failureAuthorityCASLoseResponse {
						if !errors.Is(applyErr, errFailureAuthorityCASCut) {
							t.Fatalf("lost response=%v", applyErr)
						}
					} else if applyErr != nil {
						t.Fatal(applyErr)
					}
					got := repository.aggregate
					if got.StateRevision != before.StateRevision+1 || got.ActiveDispatch != nil || got.Decision.Terminal == nil ||
						got.Decision.Terminal.Code != "repair_surface_unavailable" || got.Facts.RepairDirective != nil || got.Facts.Resources.FailureTarget != nil {
						t.Fatalf("rejected repair did not close in one CAS: decision=%+v", got.Decision)
					}
					if got.Facts.Failure == nil || got.Facts.Failure.OriginalDiagnostic == nil || got.Facts.Failure.OriginalDiagnostic.OriginalCode != "owner_rejected_effect" {
						t.Fatalf("closure lost owner diagnostic: %+v", got.Facts.Failure)
					}
					if !reflect.DeepEqual(got.OperationReceipts, before.OperationReceipts) ||
						!reflect.DeepEqual(got.ExecutionBudget, before.ExecutionBudget) || !reflect.DeepEqual(got.Facts.Coverage, before.Facts.Coverage) {
						t.Fatal("repair rejection replayed effects, changed credit or consumed an execution budget")
					}
					last := got.CompletedDispatches[len(got.CompletedDispatches)-1]
					if last.RepairCommitment == nil || last.RepairCommitment.Closure.ID != fixture.closureID {
						t.Fatal("closure lost immutable repair causation")
					}
					if err := got.Validate(); err != nil {
						t.Fatalf("fresh-load invariant: %v", err)
					}
					repository.mode = failureAuthorityCASNormal
					restarted, err := NewDispatchCoordinator(repository)
					if err != nil {
						t.Fatal(err)
					}
					saves := repository.saveAttempts
					replay, err := restarted.commitEvidence(ctx, rejected)
					if err != nil || replay.Disposition != CommitDispositionDuplicate || repository.saveAttempts != saves || replay.Aggregate.StateRevision != got.StateRevision {
						t.Fatalf("restart duplicate changed terminal: %+v err=%v", replay, err)
					}
				})
			}
		})
	}
}

func TestRejectedSemanticRepairRetiresIntermediateBindingInReceiptCAS(t *testing.T) {
	ctx := context.Background()
	prepared, _ := prepareSemanticValidationRepairOwnerInfeasibility(t)
	repository := &durableRuntimeStateRepository{aggregate: prepared}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	attempt := prepared.CandidateAttempts[len(prepared.CandidateAttempts)-1]
	candidate := *attempt.Proposal
	if err := RecordFixtureCandidateFeasibilities(ctx, controller, prepared.Facts.WorkspaceID, prepared.Facts.RunID, 0, candidate); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(ctx, prepared.Facts.WorkspaceID, prepared.Facts.RunID, 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	dispatch := *sealed.ActiveDispatch
	operation := candidateRuntimeOperations(dispatch.Candidate)[0]
	rejected := ExecutionEvidence{
		EventID:     "runtime-operation|" + dispatch.ID + "|" + operation.ID,
		WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
		OperationID: operation.ID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusRejected, SourceRef: "semantic-repair/rejected",
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "owner_rejected_effect",
			Signature: "semantic-repair-rejected", Message: "owner rejected repair", TargetRefs: operation.TargetRefs},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &rejected)
	before := repository.aggregate
	result, err := controller.commitEvidence(ctx, rejected)
	if err != nil {
		t.Fatal(err)
	}
	got := result.Aggregate
	if got.StateRevision != before.StateRevision+1 || got.Decision.Terminal == nil || got.Decision.Terminal.Code != "repair_surface_unavailable" ||
		got.Facts.RepairDirective != nil || got.Facts.Resources.FailureTarget != nil || got.ActiveDispatch != nil ||
		got.Facts.Coverage[0].CurrentBindingRef != nil || len(got.Facts.Coverage[0].BindingRetirementLineage) != len(before.Facts.Coverage[0].BindingRetirementLineage)+1 {
		t.Fatalf("receipt left intermediate repair authority: decision=%+v coverage=%+v", got.Decision, got.Facts.Coverage)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("retired semantic repair reload: %v", err)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := restarted.commitEvidence(ctx, rejected)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != got.StateRevision {
		t.Fatalf("semantic repair replay: %+v err=%v", duplicate, err)
	}
}
