package workorchruntimecontract

import (
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeOperationRecoveryAuthorizationFreshLoadBindsCounterRevisionAndExactExecution(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-recovery-authorization")
	runID := common.RunID("run-recovery-authorization")
	execution := RuntimeOperationExecutionRecord{
		OperationID: "operation-1", ExecutionKey: "execution-1", Fence: 7,
	}
	active := &RuntimeDispatchRecord{
		ID: "dispatch-1", WorkspaceID: workspaceID, RunID: runID,
		OperationExecutions: []RuntimeOperationExecutionRecord{execution},
	}
	authorization := RuntimeOperationRecoveryAuthorization{
		WorkspaceID: workspaceID, RunID: runID, DispatchID: active.ID,
		OperationID: execution.OperationID, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence,
		RecoveryAttempt: 1, IssuedAtStateRevision: 9, State: RuntimeOperationRecoveryAuthorizationIssued,
	}
	authorization.AuthorizationID = StableRuntimeOperationRecoveryAuthorizationID(authorization)
	ledger := RuntimeRecoveryLedger{OperationRecoveryAttempts: 1}
	execution.State = OperationExecutionClaimed
	active.OperationExecutions[0] = execution
	if err := ValidateRuntimeOperationRecoveryAuthorizations(workspaceID, runID, active, nil, nil, []RuntimeOperationRecoveryAuthorization{authorization}, ledger, 9); err != nil {
		t.Fatalf("valid fresh-load recovery authorization rejected: %v", err)
	}
	if loaded, found := RuntimeOperationRecoveryAuthorizationForExecution([]RuntimeOperationRecoveryAuthorization{authorization}, active.ID, execution); !found || loaded.AuthorizationID != authorization.AuthorizationID {
		t.Fatalf("exact issued authorization was not recoverable: %+v found=%v", loaded, found)
	}
	if err := ValidateRuntimeOperationRecoveryAuthorizations(workspaceID, runID, active, nil, []RuntimeOperationReceipt{{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence,
	}}, []RuntimeOperationRecoveryAuthorization{authorization}, ledger, 9); err == nil {
		t.Fatal("issued authorization remained actionable after a terminal owner receipt")
	}

	for _, test := range []struct {
		name          string
		active        *RuntimeDispatchRecord
		values        []RuntimeOperationRecoveryAuthorization
		ledger        RuntimeRecoveryLedger
		stateRevision uint64
	}{
		{name: "counter without authorization", active: active, ledger: ledger, stateRevision: 9},
		{name: "authorization without exact execution", active: &RuntimeDispatchRecord{ID: active.ID, WorkspaceID: workspaceID, RunID: runID}, values: []RuntimeOperationRecoveryAuthorization{authorization}, ledger: ledger, stateRevision: 9},
		{name: "issued revision beyond aggregate", active: active, values: []RuntimeOperationRecoveryAuthorization{authorization}, ledger: ledger, stateRevision: 8},
		{name: "attempt sequence gap", active: active, values: func() []RuntimeOperationRecoveryAuthorization {
			value := authorization
			value.RecoveryAttempt = 2
			value.AuthorizationID = StableRuntimeOperationRecoveryAuthorizationID(value)
			return []RuntimeOperationRecoveryAuthorization{value}
		}(), ledger: ledger, stateRevision: 9},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateRuntimeOperationRecoveryAuthorizations(workspaceID, runID, test.active, nil, nil, test.values, test.ledger, test.stateRevision); err == nil {
				t.Fatal("corrupt fresh-load recovery authorization was accepted")
			}
		})
	}

	uncertain := runtimeOperationRecoveryAuthorizationWithClaimForTest(authorization, RuntimeOperationRecoveryAuthorizationUncertain)
	if _, found := RuntimeOperationRecoveryAuthorizationForExecution([]RuntimeOperationRecoveryAuthorization{uncertain}, active.ID, execution); found {
		t.Fatal("uncertain historical authorization remained actionable")
	}
}

func TestRuntimeOperationRecoveryAuthorizationRevisionHistoryMustBeStrictlyIncreasing(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-recovery-history")
	runID := common.RunID("run-recovery-history")
	execution := RuntimeOperationExecutionRecord{OperationID: "operation-1", ExecutionKey: "execution-1", Fence: 1}
	active := &RuntimeDispatchRecord{ID: "dispatch-1", WorkspaceID: workspaceID, RunID: runID, OperationExecutions: []RuntimeOperationExecutionRecord{execution}}
	values := make([]RuntimeOperationRecoveryAuthorization, 2)
	for idx := range values {
		value := RuntimeOperationRecoveryAuthorization{
			WorkspaceID: workspaceID, RunID: runID, DispatchID: active.ID,
			OperationID: execution.OperationID, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence,
			RecoveryAttempt: idx + 1, IssuedAtStateRevision: 4, State: RuntimeOperationRecoveryAuthorizationIssued,
		}
		value.AuthorizationID = StableRuntimeOperationRecoveryAuthorizationID(value)
		values[idx] = runtimeOperationRecoveryAuthorizationWithClaimForTest(value, RuntimeOperationRecoveryAuthorizationUncertain)
	}
	execution.State = OperationExecutionClaimed
	active.OperationExecutions[0] = execution
	if err := ValidateRuntimeOperationRecoveryAuthorizations(workspaceID, runID, active, nil, nil, values, RuntimeRecoveryLedger{OperationRecoveryAttempts: 2}, 5); err == nil {
		t.Fatal("two recovery authorizations issued at one aggregate revision were accepted")
	}
}

func TestRuntimeOperationRecoveryAuthorizationOnlyLatestGrantMayRemainIssued(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-recovery-current")
	runID := common.RunID("run-recovery-current")
	execution := RuntimeOperationExecutionRecord{OperationID: "operation-1", ExecutionKey: "execution-1", Fence: 1, State: OperationExecutionClaimed}
	active := &RuntimeDispatchRecord{ID: "dispatch-1", WorkspaceID: workspaceID, RunID: runID, OperationExecutions: []RuntimeOperationExecutionRecord{execution}}
	values := make([]RuntimeOperationRecoveryAuthorization, 2)
	for idx := range values {
		value := RuntimeOperationRecoveryAuthorization{
			WorkspaceID: workspaceID, RunID: runID, DispatchID: active.ID,
			OperationID: execution.OperationID, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence,
			RecoveryAttempt: idx + 1, IssuedAtStateRevision: uint64(idx + 4), State: RuntimeOperationRecoveryAuthorizationIssued,
		}
		value.AuthorizationID = StableRuntimeOperationRecoveryAuthorizationID(value)
		values[idx] = value
	}
	if err := ValidateRuntimeOperationRecoveryAuthorizations(workspaceID, runID, active, nil, nil, values, RuntimeRecoveryLedger{OperationRecoveryAttempts: 2}, 5); err == nil {
		t.Fatal("two concurrently actionable replay grants were accepted")
	}
	values[0] = runtimeOperationRecoveryAuthorizationWithClaimForTest(values[0], RuntimeOperationRecoveryAuthorizationUncertain)
	if err := ValidateRuntimeOperationRecoveryAuthorizations(workspaceID, runID, active, nil, nil, values, RuntimeRecoveryLedger{OperationRecoveryAttempts: 2}, 5); err != nil {
		t.Fatalf("historical uncertain grant plus current issued grant rejected: %v", err)
	}
}

func runtimeOperationRecoveryAuthorizationWithClaimForTest(value RuntimeOperationRecoveryAuthorization, state RuntimeOperationRecoveryAuthorizationState) RuntimeOperationRecoveryAuthorization {
	value = value.Normalized()
	value.State = state
	value.ClaimGeneration = 1
	claimedAt := time.Date(2026, time.August, 30, 0, value.RecoveryAttempt, 0, 0, time.UTC)
	value.ClaimedAt = claimedAt.Format(time.RFC3339Nano)
	value.LeaseDeadline = claimedAt.Add(RuntimeOperationRecoveryClaimLeaseDuration).Format(time.RFC3339Nano)
	value.ClaimID = StableRuntimeOperationRecoveryClaimID(value)
	return value
}
