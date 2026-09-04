package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeNonTerminalOuterCountingRepository struct {
	delegate      runtime.RuntimeStateRepository
	successfulCAS int
}

func (r *runtimeNonTerminalOuterCountingRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *runtimeNonTerminalOuterCountingRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if err := r.delegate.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	r.successfulCAS++
	return nil
}

type runtimeNonTerminalOuterTemporaryError struct{}

func (runtimeNonTerminalOuterTemporaryError) Error() string   { return "arbitrary temporary failure" }
func (runtimeNonTerminalOuterTemporaryError) Temporary() bool { return true }

func runtimeNonTerminalOwnerTransientForOuterTest(aggregate runtime.RuntimeAggregate) *runtime.RuntimeOwnerTransientError {
	expectedRevision := aggregate.StateRevision
	return &runtime.RuntimeOwnerTransientError{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ExpectedRevision: expectedRevision, ObservedRevision: expectedRevision,
		Feedback: runtimecontract.RuntimeFeedback{
			Domain: runtimecontract.RuntimeFeedbackDomainPersistence, StableCategory: runtimecontract.RuntimeFeedbackCategoryOwnerTransient,
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: "blocks/run",
				OriginalClass: "owner_transient", OriginalCode: "runtime_store_unavailable",
				OriginalMessage: "runtime store is temporarily unavailable",
				Signature: fmt.Sprintf(
					"runtime-state-update/%s/%s/%d/%d/owner-transient",
					aggregate.Facts.WorkspaceID,
					aggregate.Facts.RunID,
					expectedRevision,
					expectedRevision,
				),
				ExpectedRevision: fmt.Sprintf("%d", expectedRevision), ObservedRevision: fmt.Sprintf("%d", expectedRevision),
			},
		},
	}
}

func newRuntimeNonTerminalOuterHarness(
	t *testing.T,
	repository runtime.RuntimeStateRepository,
	aggregate runtime.RuntimeAggregate,
) (*Biz, *recordingBackgroundRoutine, scheduledRuntimeDriveRequest) {
	t.Helper()
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("nonterminal recovery fixture did not acquire its run gate")
	}
	request := scheduledRuntimeDriveRequestAtCurrentGate(t, module, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "nonterminal_recovery_test")
	return module, background, request
}

func assertRuntimeNonTerminalOuterTimer(
	t *testing.T,
	module *Biz,
	background *recordingBackgroundRoutine,
	request scheduledRuntimeDriveRequest,
	lease runtimecontract.RuntimeDriveLease,
) {
	t.Helper()
	module.driveMu.Lock()
	registration, registered := module.retryLeaseTimers[request.driveKey]
	gate, gateOwned := module.drivingRuns[request.driveKey]
	module.driveMu.Unlock()
	if !registered || registration.Epoch != request.gateEpoch || registration.GateToken != request.gateToken ||
		registration.Generation != lease.RecoveryGeneration || !runtimecontract.SameRuntimeDriveLeaseIdentity(registration.Lease, lease) {
		t.Fatalf("exact nonterminal recovery timer was not mounted: registered=%v registration=%+v lease=%+v", registered, registration, lease)
	}
	if !gateOwned || gate.Epoch != request.gateEpoch || gate.Token != request.gateToken {
		t.Fatalf("nonterminal recovery timer lost exact drive gate: owned=%v gate=%+v request=%+v", gateOwned, gate, request)
	}
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("nonterminal recovery timer submissions=%d callback=%v, want one exact submission", background.asyncFunctionCalls, background.lastAsyncFunction != nil)
	}
}

func TestRuntimeNonTerminalRecoveryOuterCASGenerationZeroCommitsOneAtomicSuccessor(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	base := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	counting := &runtimeNonTerminalOuterCountingRepository{delegate: base}
	module, background, request := newRuntimeNonTerminalOuterHarness(t, counting, aggregate)
	cause := &runtime.RuntimeCASConflictError{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ExpectedRevision: aggregate.StateRevision, ObservedRevision: aggregate.StateRevision + 1,
	}
	retry := module.handleRuntimeNonTerminalRetry(ctx, request, classifyRuntimeDriveFailure(cause), cause)
	if retry == nil || !retry.AwaitingNonTerminalRecovery || retry.NonTerminalRecoveryRequest.Kind != runtimecontract.RuntimeNonTerminalRecoveryCASConflict {
		t.Fatalf("typed CAS did not enter the atomic nonterminal transaction: %+v", retry)
	}
	module.finishScheduledRuntimeDrive(ctx, request, &retry)

	persisted, err := base.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	record := persisted.Facts.Recovery.Normalized().NonTerminalRecovery
	if counting.successfulCAS != 1 || persisted.StateRevision != aggregate.StateRevision+1 || persisted.Facts.Recovery.CASReloadAttempts != 1 ||
		record.Request.RequestID != retry.NonTerminalRecoveryRequest.RequestID ||
		record.Disposition != runtimecontract.RuntimeNonTerminalRecoveryDispositionBudgetConsumed ||
		persisted.DriveLease == nil || persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		persisted.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonCASReload ||
		persisted.DriveLease.RecoveryRequestID != retry.NonTerminalRecoveryRequest.RequestID {
		t.Fatalf("generation-zero CAS recovery was not one atomic budget+lease CAS: saves=%d aggregate=%+v record=%+v", counting.successfulCAS, persisted, record)
	}
	assertRuntimeNonTerminalOuterTimer(t, module, background, request, persisted.DriveLease.Clone())
}

func TestRuntimeNonTerminalRecoveryAtomicLinearizationRejectsTornPersistence(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	cause := &runtime.RuntimeCASConflictError{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ExpectedRevision: aggregate.StateRevision, ObservedRevision: aggregate.StateRevision + 1,
	}
	request, err := runtime.NewRuntimeNonTerminalRecoveryRequest(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		0,
		aggregate.StateRevision,
		"2026-09-04T12:00:00Z",
		cause,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:  runtime.RuntimeInternalEventNonTerminalRecoveryScheduled,
		Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Delivery: &runtime.RuntimeInternalDeliveryPayload{
			NonTerminalRecoveryRequest: &request,
			NonTerminalRecoveryCause:   cause,
		},
	})
	if err != nil || result.DriveLease == nil {
		t.Fatalf("atomic nonterminal fixture result=%+v err=%v", result, err)
	}
	if err := result.Aggregate.Validate(); err != nil {
		t.Fatalf("valid atomic aggregate: %v", err)
	}
	if err := runtimeStateRecordFromAggregate(result.Aggregate).Validate(); err != nil {
		t.Fatalf("valid atomic state record: %v", err)
	}

	for _, testCase := range []struct {
		name   string
		mutate func(*runtime.RuntimeAggregate)
	}{
		{
			name: "record ahead of current revision",
			mutate: func(value *runtime.RuntimeAggregate) {
				value.StateRevision = value.Facts.Recovery.NonTerminalRecovery.RecordedAtStateRevision - 1
			},
		},
		{
			name: "atomic successor missing",
			mutate: func(value *runtime.RuntimeAggregate) {
				value.DriveLease = nil
			},
		},
		{
			name: "atomic successor replaced",
			mutate: func(value *runtime.RuntimeAggregate) {
				lease := value.DriveLease.Clone()
				lease.Reason = runtimecontract.RuntimeDriveLeaseReasonPersistedPending
				lease.FailureSignature = "unrelated-successor"
				lease.RecoveryRequestID = ""
				value.DriveLease = &lease
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tampered := result.Aggregate
			testCase.mutate(&tampered)
			if err := tampered.Validate(); err == nil {
				t.Fatal("Runtime aggregate accepted torn nonterminal recovery persistence")
			}
			if err := runtimeStateRecordFromAggregate(tampered).Validate(); err == nil {
				t.Fatal("Runtime state record accepted torn nonterminal recovery persistence")
			}
		})
	}
}

func TestRuntimeNonTerminalRecoveryOuterOwnerTransientUsesSharedCASBudgetOnly(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	base := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	counting := &runtimeNonTerminalOuterCountingRepository{delegate: base}
	module, background, request := newRuntimeNonTerminalOuterHarness(t, counting, aggregate)
	cause := runtimeNonTerminalOwnerTransientForOuterTest(aggregate)
	retry := module.handleRuntimeNonTerminalRetry(ctx, request, classifyRuntimeDriveFailure(cause), cause)
	if retry == nil || !retry.AwaitingNonTerminalRecovery || retry.NonTerminalRecoveryRequest.Kind != runtimecontract.RuntimeNonTerminalRecoveryOwnerTransient {
		t.Fatalf("typed owner transient did not enter the atomic nonterminal transaction: %+v", retry)
	}
	module.finishScheduledRuntimeDrive(ctx, request, &retry)

	persisted, err := base.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || counting.successfulCAS != 1 || persisted.DriveLease == nil ||
		persisted.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonOwnerTransient ||
		persisted.DriveLease.RecoveryRequestID != retry.NonTerminalRecoveryRequest.RequestID ||
		persisted.Facts.Recovery.CASReloadAttempts != 1 || persisted.Facts.Recovery.OperationRecoveryAttempts != 0 {
		t.Fatalf("owner-transient recovery used the wrong budget/successor: saves=%d aggregate=%+v err=%v", counting.successfulCAS, persisted, err)
	}
	assertRuntimeNonTerminalOuterTimer(t, module, background, request, persisted.DriveLease.Clone())
}

func TestRuntimeNonTerminalRecoveryOuterPostCommitResponseLossReloadsWithoutSecondCAS(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	base := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	counting := &runtimeNonTerminalOuterCountingRepository{delegate: base}
	responseLoss := &runtimePostCommitResponseLossRepository{
		delegate: counting,
		match: func(candidate runtime.RuntimeAggregate) bool {
			return runtimecontract.RuntimeNonTerminalRecoveryRecordPresent(candidate.Facts.Recovery.Normalized().NonTerminalRecovery)
		},
		err: errors.New("injected ordinary post-commit response loss"),
	}
	module, background, request := newRuntimeNonTerminalOuterHarness(t, responseLoss, aggregate)
	cause := &runtime.RuntimeCASConflictError{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ExpectedRevision: aggregate.StateRevision, ObservedRevision: aggregate.StateRevision + 1,
	}
	retry := module.handleRuntimeNonTerminalRetry(ctx, request, classifyRuntimeDriveFailure(cause), cause)
	if retry == nil || !retry.AwaitingNonTerminalRecovery || !responseLoss.fired {
		t.Fatalf("post-commit response loss was not retained for fresh settlement: retry=%+v fired=%v", retry, responseLoss.fired)
	}
	module.finishScheduledRuntimeDrive(ctx, request, &retry)

	persisted, err := base.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	record := persisted.Facts.Recovery.Normalized().NonTerminalRecovery
	if err != nil || counting.successfulCAS != 1 || persisted.Facts.Recovery.CASReloadAttempts != 1 ||
		!runtimecontract.SameRuntimeNonTerminalRecoveryRequestIdentity(record.Request, retry.NonTerminalRecoveryRequest) ||
		persisted.DriveLease == nil || persisted.DriveLease.RecoveryRequestID != record.Request.RequestID {
		t.Fatalf("fresh reload duplicated a committed nonterminal recovery: saves=%d aggregate=%+v record=%+v err=%v", counting.successfulCAS, persisted, record, err)
	}
	assertRuntimeNonTerminalOuterTimer(t, module, background, request, persisted.DriveLease.Clone())
}

func TestRuntimeNonTerminalRecoveryOuterExhaustionKeepsCappedDurableSuccessor(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	aggregate.Facts.RecoveryPolicy.MaxCASReloadAttempts = 1
	aggregate.Facts.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(
		aggregate.Facts.ConvergencePolicy,
		aggregate.Facts.RecoveryPolicy,
	)
	executionBudget, err := runtimecontract.NewRuntimeExecutionBudgetLedger(aggregate.Facts.ExecutionBudgetPolicy)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.ExecutionBudget = executionBudget
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	base := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	controller, err := runtime.NewRuntimeController(base)
	if err != nil {
		t.Fatal(err)
	}
	firstCause := &runtime.RuntimeCASConflictError{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ExpectedRevision: aggregate.StateRevision, ObservedRevision: aggregate.StateRevision + 1,
	}
	firstRequest, err := runtime.NewRuntimeNonTerminalRecoveryRequest(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		0,
		aggregate.StateRevision,
		"2026-09-04T12:00:00Z",
		firstCause,
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:  runtime.RuntimeInternalEventNonTerminalRecoveryScheduled,
		Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Delivery: &runtime.RuntimeInternalDeliveryPayload{
			NonTerminalRecoveryRequest: &firstRequest,
			NonTerminalRecoveryCause:   firstCause,
		},
	})
	if err != nil || first.DriveLease == nil {
		t.Fatalf("seed atomic nonterminal recovery result=%+v err=%v", first, err)
	}
	claimed, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:  runtime.RuntimeInternalEventDriveLeaseClaimed,
		Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Delivery: &runtime.RuntimeInternalDeliveryPayload{
			DriveLease: first.DriveLease,
			LeaseNow:   first.DriveLease.DueAt,
		},
	})
	if err != nil || !claimed.LeaseClaimed || claimed.DriveLease == nil || claimed.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning {
		t.Fatalf("claim atomic nonterminal recovery lease result=%+v err=%v", claimed, err)
	}
	aggregate = claimed.Aggregate
	counting := &runtimeNonTerminalOuterCountingRepository{delegate: base}
	module, background, request := newRuntimeNonTerminalOuterHarness(t, counting, aggregate)
	request.leaseGeneration = claimed.DriveLease.RecoveryGeneration
	cause := &runtime.RuntimeCASConflictError{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ExpectedRevision: aggregate.StateRevision, ObservedRevision: aggregate.StateRevision + 1,
	}
	retry := module.handleRuntimeNonTerminalRetry(ctx, request, classifyRuntimeDriveFailure(cause), cause)
	module.finishScheduledRuntimeDrive(ctx, request, &retry)

	persisted, err := base.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || retry == nil || persisted.DriveLease == nil {
		t.Fatalf("exhausted nonterminal recovery lost its successor: retry=%+v aggregate=%+v err=%v", retry, persisted, err)
	}
	record := persisted.Facts.Recovery.Normalized().NonTerminalRecovery
	observedAt, observedErr := time.Parse(time.RFC3339Nano, record.Request.ObservedAt)
	dueAt, dueErr := time.Parse(time.RFC3339Nano, persisted.DriveLease.DueAt)
	if counting.successfulCAS != 1 || record.Disposition != runtimecontract.RuntimeNonTerminalRecoveryDispositionBudgetExhausted ||
		record.Attempt != aggregate.Facts.RecoveryPolicy.MaxCASReloadAttempts ||
		persisted.Facts.Recovery.CASReloadAttempts != aggregate.Facts.RecoveryPolicy.MaxCASReloadAttempts ||
		persisted.Facts.Failure != nil || persisted.Decision.Terminal != nil ||
		persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		persisted.DriveLease.RecoveryRequestID != record.Request.RequestID || observedErr != nil || dueErr != nil ||
		dueAt.Sub(observedAt) != runtimecontract.RuntimeNonTerminalRecoveryBackoffCap {
		t.Fatalf("exhausted nonterminal recovery was not a capped nonterminal successor: saves=%d aggregate=%+v record=%+v observedErr=%v dueErr=%v", counting.successfulCAS, persisted, record, observedErr, dueErr)
	}
	assertRuntimeNonTerminalOuterTimer(t, module, background, request, persisted.DriveLease.Clone())
}

func TestRuntimeNonTerminalRecoveryOuterRejectsUntypedRetryAuthority(t *testing.T) {
	for name, cause := range map[string]error{
		"deadline":  context.DeadlineExceeded,
		"temporary": runtimeNonTerminalOuterTemporaryError{},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			aggregate := runtimeAggregate(t, 1)
			base := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
			module, _, request := newRuntimeNonTerminalOuterHarness(t, base, aggregate)
			retry := module.handleRuntimeNonTerminalRetry(ctx, request, classifyRuntimeDriveFailure(cause), cause)
			persisted, err := base.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if runtimecontract.RuntimeNonTerminalRecoveryRecordPresent(persisted.Facts.Recovery.Normalized().NonTerminalRecovery) ||
				(persisted.DriveLease != nil && runtimecontract.RuntimeDriveLeaseBindsNonTerminalRecovery(*persisted.DriveLease)) {
				t.Fatalf("untyped %T entered nonterminal recovery: retry=%+v aggregate=%+v", cause, retry, persisted)
			}
			if persisted.Facts.Failure == nil || persisted.Facts.Failure.Code != "runtime_nonterminal_recovery_untyped" || persisted.Decision.Terminal == nil {
				t.Fatalf("untyped %T did not fail closed as a Runtime contract defect: retry=%+v aggregate=%+v", cause, retry, persisted)
			}
		})
	}
}

func TestRuntimeBusinessFailureFactTypedCASUsesAtomicNonTerminalRecovery(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	base := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	counting := &runtimeNonTerminalOuterCountingRepository{delegate: base}
	fault := &failOnceCASRuntimeRepository{delegate: counting, remainingFailures: 1}
	module, background, request := newRuntimeNonTerminalOuterHarness(t, fault, aggregate)
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment,
		Code:  "runtime_drive_failed", Signature: "failure-fact-cas", Message: "failure fact must be retried",
	}
	retry := module.handleRuntimeBusinessTerminalFailure(ctx, request, failure)
	if retry == nil || !retry.AwaitingNonTerminalRecovery || retry.Reason == runtimecontract.RuntimeDriveLeaseReasonCASReload ||
		retry.NonTerminalRecoveryRequest.Kind != runtimecontract.RuntimeNonTerminalRecoveryCASConflict {
		t.Fatalf("failure-fact typed CAS created a legacy split retry: %+v", retry)
	}
	module.finishScheduledRuntimeDrive(ctx, request, &retry)

	persisted, err := base.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	record := persisted.Facts.Recovery.Normalized().NonTerminalRecovery
	if err != nil || counting.successfulCAS != 2 || persisted.StateRevision != aggregate.StateRevision+2 ||
		persisted.Facts.Failure != nil || persisted.Decision.Terminal != nil ||
		persisted.Facts.Recovery.CASReloadAttempts != 1 || record.Request.RequestID != retry.NonTerminalRecoveryRequest.RequestID ||
		persisted.DriveLease == nil || persisted.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonCASReload ||
		persisted.DriveLease.RecoveryRequestID != record.Request.RequestID {
		t.Fatalf("failure-fact typed CAS did not converge through atomic recovery: saves=%d aggregate=%+v record=%+v err=%v", counting.successfulCAS, persisted, record, err)
	}
	assertRuntimeNonTerminalOuterTimer(t, module, background, request, persisted.DriveLease.Clone())
}
