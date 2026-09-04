package runtime

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type arbitraryTemporaryRuntimeError struct{}

func (arbitraryTemporaryRuntimeError) Error() string   { return "temporary" }
func (arbitraryTemporaryRuntimeError) Temporary() bool { return true }

func runtimeOwnerTransientErrorForTest(
	workspaceID common.WorkspaceID,
	runID common.RunID,
	expectedRevision uint64,
	observedRevision uint64,
) *RuntimeOwnerTransientError {
	return &RuntimeOwnerTransientError{
		WorkspaceID: workspaceID, RunID: runID,
		ExpectedRevision: expectedRevision, ObservedRevision: observedRevision,
		Feedback: runtimecontract.RuntimeFeedback{
			Domain: runtimecontract.RuntimeFeedbackDomainPersistence, StableCategory: runtimecontract.RuntimeFeedbackCategoryOwnerTransient,
			WorkspaceID: workspaceID, RunID: runID,
			OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: "blocks/run",
				OriginalClass: "owner_transient", OriginalCode: "runtime_store_unavailable",
				OriginalMessage:  "runtime store is temporarily unavailable",
				Signature:        fmt.Sprintf("runtime-state-update/%s/%s/%d/%d/owner-transient", workspaceID, runID, expectedRevision, observedRevision),
				ExpectedRevision: fmt.Sprintf("%d", expectedRevision), ObservedRevision: fmt.Sprintf("%d", observedRevision),
			},
		},
	}
}

type nonterminalRecoveryResponseLossRepository struct {
	controllerRepository
	loseNextResponse bool
}

func (r *nonterminalRecoveryResponseLossRepository) Save(ctx context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	if err := r.controllerRepository.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if r.loseNextResponse {
		r.loseNextResponse = false
		return runtimeOwnerTransientErrorForTest(
			aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, expectedRevision, aggregate.StateRevision,
		)
	}
	return nil
}

func runtimeNonTerminalRecoveryEvent(request runtimecontract.RuntimeNonTerminalRecoveryRequest, cause error) RuntimeInternalEvent {
	return RuntimeInternalEvent{
		Kind:  RuntimeInternalEventNonTerminalRecoveryScheduled,
		Scope: RuntimeInternalEventScope{WorkspaceID: request.WorkspaceID, RunID: request.RunID},
		Delivery: &RuntimeInternalDeliveryPayload{
			NonTerminalRecoveryRequest: &request,
			NonTerminalRecoveryCause:   cause,
		},
	}
}

func TestRuntimeNonTerminalRecoveryTypedOutcomeGateRejectsGenericErrors(t *testing.T) {
	observedAt := "2026-09-04T12:00:00Z"
	for name, cause := range map[string]error{
		"plain conflict":      common.ErrConflict,
		"arbitrary temporary": arbitraryTemporaryRuntimeError{},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRuntimeNonTerminalRecoveryRequest("ws-v2", "run-v2", 0, 1, observedAt, cause); err == nil {
				t.Fatalf("generic error %T entered typed recovery", cause)
			}
		})
	}
	cas := &RuntimeCASConflictError{WorkspaceID: "ws-v2", RunID: "run-v2", ExpectedRevision: 1, ObservedRevision: 2}
	request, err := NewRuntimeNonTerminalRecoveryRequest("ws-v2", "run-v2", 0, 2, observedAt, fmt.Errorf("wrapped: %w", cas))
	if err != nil || request.Kind != runtimecontract.RuntimeNonTerminalRecoveryCASConflict {
		t.Fatalf("typed wrapped CAS request=%+v err=%v", request, err)
	}
	controller, err := NewRuntimeController(&controllerRepository{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ApplyInternalEvent(context.Background(), runtimeNonTerminalRecoveryEvent(request, common.ErrConflict)); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("plain conflict crossed the pre-I/O event gate: %v", err)
	}
	owner := runtimeOwnerTransientErrorForTest("ws-v2", "run-v2", 2, 2)
	request, err = NewRuntimeNonTerminalRecoveryRequest("ws-v2", "run-v2", 0, 2, observedAt, owner)
	if err != nil || request.Kind != runtimecontract.RuntimeNonTerminalRecoveryOwnerTransient {
		t.Fatalf("typed owner-transient request=%+v err=%v", request, err)
	}
	if _, err := NewRuntimeNonTerminalRecoveryRequest("ws-v2", "run-v2", 0, 2, observedAt, errors.Join(cas, owner)); err == nil {
		t.Fatal("ambiguous typed persistence outcomes entered recovery")
	}
}

func TestRuntimeNonTerminalRecoveryAtomicallyPersistsBudgetRecordAndUniqueLease(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cause := runtimeOwnerTransientErrorForTest(
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, initialized.StateRevision, initialized.StateRevision,
	)
	request, err := NewRuntimeNonTerminalRecoveryRequest(
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, 0, initialized.StateRevision,
		"2026-09-04T12:00:00Z", cause,
	)
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	if _, _, changed, err := controller.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonOwnerTransient, FailureSignature: request.FailureSignature,
		RecoveryRequestID: request.RequestID, DueAt: "2026-09-04T12:00:01Z",
	}); !errors.Is(err, common.ErrConflict) || changed || repository.saves != saves {
		t.Fatalf("standalone scheduler created request-bound lease: changed=%v saves=%d/%d err=%v", changed, repository.saves, saves, err)
	}
	result, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(request, cause))
	if err != nil {
		t.Fatal(err)
	}
	if !result.LeaseChanged || !result.BudgetConsumed || result.NonTerminalRecoveryRecord == nil || result.DriveLease == nil ||
		result.Aggregate.StateRevision != initialized.StateRevision+1 || repository.saves != saves+1 {
		t.Fatalf("atomic result=%+v saves=%d/%d", result, repository.saves, saves+1)
	}
	recovery := result.Aggregate.Facts.Recovery
	if recovery.CASReloadAttempts != 1 || recovery.OperationRecoveryAttempts != 0 ||
		recovery.NonTerminalRecovery.Request.RequestID != request.RequestID ||
		result.DriveLease.RecoveryRequestID != request.RequestID ||
		result.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonOwnerTransient ||
		len(result.Aggregate.OperationRecoveryAuthorizations) != 0 {
		t.Fatalf("owner-transient recovery crossed ledgers: recovery=%+v lease=%+v", recovery, result.DriveLease)
	}
	if err := result.Aggregate.Validate(); err != nil {
		t.Fatalf("atomic recovery aggregate invalid: %v", err)
	}

	// The same typed outcome can be redelivered with a later wall-clock value.
	// Its stable ID and persisted first due-at win, and no second CAS occurs.
	duplicateRequest := request
	duplicateRequest.ObservedAt = "2026-09-04T12:05:00Z"
	saves = repository.saves
	duplicate, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(duplicateRequest, cause))
	if err != nil || duplicate.LeaseChanged || duplicate.BudgetConsumed || duplicate.NonTerminalRecoveryRecord == nil ||
		duplicate.DriveLease == nil || duplicate.DriveLease.DueAt != result.DriveLease.DueAt || repository.saves != saves {
		t.Fatalf("duplicate result=%+v saves=%d/%d err=%v", duplicate, repository.saves, saves, err)
	}
	_, running, claimed, err := controller.claimDriveLease(
		ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, *duplicate.DriveLease, duplicate.DriveLease.DueAt,
	)
	if err != nil || !claimed {
		t.Fatalf("claim persisted generic lease=%+v claimed=%v err=%v", running, claimed, err)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	recovered, restartedLease, changed, err := restarted.recoverDriveLease(
		ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, "2026-09-04T12:10:00Z",
	)
	if err != nil || !changed || restartedLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		restartedLease.RecoveryGeneration != running.RecoveryGeneration+1 || restartedLease.RecoveryRequestID != request.RequestID ||
		recovered.Facts.Recovery.CASReloadAttempts != 1 ||
		recovered.Facts.Recovery.NonTerminalRecovery.Request.RequestID != request.RequestID {
		t.Fatalf("restart recovery lost atomic lineage: aggregate=%+v lease=%+v changed=%v err=%v", recovered, restartedLease, changed, err)
	}
}

func TestRuntimeNonTerminalRecoveryResponseLossReloadsPersistedRecordAndLease(t *testing.T) {
	ctx := context.Background()
	repository := &nonterminalRecoveryResponseLossRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cause := &RuntimeCASConflictError{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		ExpectedRevision: initialized.StateRevision, ObservedRevision: initialized.StateRevision + 1,
	}
	request, err := NewRuntimeNonTerminalRecoveryRequest(
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, 0, initialized.StateRevision,
		"2026-09-04T12:00:00Z", cause,
	)
	if err != nil {
		t.Fatal(err)
	}
	repository.loseNextResponse = true
	saves := repository.saves
	if _, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(request, cause)); err == nil {
		t.Fatal("simulated response loss was not returned")
	}
	if repository.saves != saves+1 || repository.aggregate == nil ||
		repository.aggregate.Facts.Recovery.NonTerminalRecovery.Request.RequestID != request.RequestID {
		t.Fatalf("commit was not durable before response loss: aggregate=%+v saves=%d", repository.aggregate, repository.saves)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	saves = repository.saves
	result, err := restarted.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(request, cause))
	if err != nil || result.LeaseChanged || result.BudgetConsumed || result.NonTerminalRecoveryRecord == nil || result.DriveLease == nil ||
		result.NonTerminalRecoveryRecord.Request.RequestID != request.RequestID || result.DriveLease.RecoveryRequestID != request.RequestID ||
		repository.saves != saves {
		t.Fatalf("response-loss reload result=%+v saves=%d/%d err=%v", result, repository.saves, saves, err)
	}
}

func TestRuntimeNonTerminalRecoveryExhaustionKeepsDurableCappedSuccessor(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.RecoveryPolicy.MaxCASReloadAttempts = 1
	initialized, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstCause := &RuntimeCASConflictError{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		ExpectedRevision: initialized.StateRevision, ObservedRevision: initialized.StateRevision + 1,
	}
	firstRequest, err := NewRuntimeNonTerminalRecoveryRequest(
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, 0, initialized.StateRevision,
		"2026-09-04T12:00:00Z", firstCause,
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(firstRequest, firstCause))
	if err != nil || first.DriveLease == nil {
		t.Fatalf("first recovery result=%+v err=%v", first, err)
	}
	claimedAggregate, running, claimed, err := controller.claimDriveLease(
		ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, *first.DriveLease, first.DriveLease.DueAt,
	)
	if err != nil || !claimed {
		t.Fatalf("claim first successor lease=%+v claimed=%v err=%v", running, claimed, err)
	}
	secondCause := runtimeOwnerTransientErrorForTest(
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, claimedAggregate.StateRevision, claimedAggregate.StateRevision,
	)
	secondObserved := "2026-09-04T12:01:00Z"
	secondRequest, err := NewRuntimeNonTerminalRecoveryRequest(
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, running.RecoveryGeneration, claimedAggregate.StateRevision,
		secondObserved, secondCause,
	)
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	exhausted, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(secondRequest, secondCause))
	if err != nil {
		t.Fatal(err)
	}
	if exhausted.BudgetConsumed || !exhausted.LeaseChanged || exhausted.NonTerminalRecoveryRecord == nil || exhausted.DriveLease == nil ||
		exhausted.NonTerminalRecoveryRecord.Disposition != runtimecontract.RuntimeNonTerminalRecoveryDispositionBudgetExhausted ||
		exhausted.Aggregate.Facts.Recovery.CASReloadAttempts != 1 || exhausted.Aggregate.Facts.Recovery.OperationRecoveryAttempts != 0 ||
		exhausted.Aggregate.Termination != nil || exhausted.Aggregate.Decision.Terminal != nil ||
		exhausted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		exhausted.DriveLease.RecoveryGeneration != running.RecoveryGeneration+1 || repository.saves != saves+1 {
		t.Fatalf("exhausted recovery lost durable lane: result=%+v saves=%d/%d", exhausted, repository.saves, saves+1)
	}
	due, err := time.Parse(time.RFC3339Nano, exhausted.DriveLease.DueAt)
	if err != nil {
		t.Fatal(err)
	}
	observed, _ := time.Parse(time.RFC3339Nano, secondObserved)
	if delay := due.Sub(observed); delay != runtimecontract.RuntimeNonTerminalRecoveryBackoffCap {
		t.Fatalf("exhausted successor delay = %s, want capped backoff %s", delay, runtimecontract.RuntimeNonTerminalRecoveryBackoffCap)
	}
	claimedAgain, runningAgain, claimed, err := controller.claimDriveLease(
		ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, *exhausted.DriveLease, exhausted.DriveLease.DueAt,
	)
	if err != nil || !claimed {
		t.Fatalf("claim exhausted successor lease=%+v claimed=%v err=%v", runningAgain, claimed, err)
	}
	thirdCause := &RuntimeCASConflictError{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		ExpectedRevision: claimedAgain.StateRevision, ObservedRevision: claimedAgain.StateRevision + 1,
	}
	thirdRequest, err := NewRuntimeNonTerminalRecoveryRequest(
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, runningAgain.RecoveryGeneration, claimedAgain.StateRevision,
		"2026-09-04T12:03:00Z", thirdCause,
	)
	if err != nil {
		t.Fatal(err)
	}
	saves = repository.saves
	third, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(thirdRequest, thirdCause))
	if err != nil || third.BudgetConsumed || !third.LeaseChanged || third.DriveLease == nil ||
		third.DriveLease.RecoveryGeneration != runningAgain.RecoveryGeneration+1 ||
		third.Aggregate.Facts.Recovery.CASReloadAttempts != 1 || repository.saves != saves+1 {
		t.Fatalf("repeated exhaustion did not retain one durable successor: result=%+v saves=%d/%d err=%v", third, repository.saves, saves+1, err)
	}

	staleRequest := secondRequest
	staleRequest.IssuedAtStateRevision = claimedAggregate.StateRevision - 1
	staleRequest.RequestID = runtimecontract.StableRuntimeNonTerminalRecoveryRequestID(staleRequest)
	saves = repository.saves
	if _, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(staleRequest, secondCause)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale admission fence err=%v, want conflict", err)
	}
	if repository.saves != saves {
		t.Fatalf("stale admission fence wrote aggregate: saves=%d/%d", repository.saves, saves)
	}
}

func TestRuntimeAdministrativeTerminationRetiresRunningNonterminalRecoveryAuthority(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cause := &RuntimeCASConflictError{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		ExpectedRevision: initialized.StateRevision, ObservedRevision: initialized.StateRevision + 1,
	}
	request, err := NewRuntimeNonTerminalRecoveryRequest(
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, 0, initialized.StateRevision,
		"2026-09-04T12:00:00Z", cause,
	)
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(request, cause))
	if err != nil || scheduled.DriveLease == nil {
		t.Fatalf("schedule generic recovery result=%+v err=%v", scheduled, err)
	}
	_, running, claimed, err := controller.claimDriveLease(
		ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, *scheduled.DriveLease, scheduled.DriveLease.DueAt,
	)
	if err != nil || !claimed {
		t.Fatalf("claim generic recovery lease=%+v claimed=%v err=%v", running, claimed, err)
	}
	terminated, err := controller.requestTermination(
		ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID,
		runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancelled the active run",
	)
	if err != nil {
		t.Fatal(err)
	}
	retired := terminated.Facts.Recovery.RetiredTerminalDeliveryLease
	if terminated.DriveLease != nil || retired.State != runtimecontract.RuntimeDriveLeaseRunning ||
		retired.RecoveryRequestID != request.RequestID ||
		terminated.Facts.Recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != "" {
		t.Fatalf("termination did not preserve exact generic running lineage: %+v", terminated)
	}
	if err := terminated.Validate(); err != nil {
		t.Fatalf("terminated aggregate with generic retired lineage invalid: %v", err)
	}
}
