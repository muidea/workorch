package readmodelprojection

import (
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCommittedRuntimeOperationEvidenceRefsExcludeGovernancePendingAndUncommittedExecutions(t *testing.T) {
	operations := []projectioncontract.RuntimeOperationStatus{
		{DispatchID: "dispatch-committed", OperationID: "operation-committed", ExecutionState: string(runtimecontract.OperationExecutionEvidenceCommitted)},
		{DispatchID: " dispatch-committed ", OperationID: " operation-committed ", ExecutionState: string(runtimecontract.OperationExecutionEvidenceCommitted)},
		{DispatchID: "dispatch-governance", OperationID: "operation-governance", ExecutionState: string(runtimecontract.OperationExecutionOwnerCommitted), Outcome: string(runtimecontract.RuntimeOperationOutcomeGovernanceRequired)},
		{DispatchID: "dispatch-unknown", OperationID: "operation-unknown", ExecutionState: string(runtimecontract.OperationExecutionOutcomeUnknown)},
		{DispatchID: "dispatch-claimed", OperationID: "operation-claimed", ExecutionState: string(runtimecontract.OperationExecutionClaimed)},
		{DispatchID: "", OperationID: "operation-missing-dispatch", ExecutionState: string(runtimecontract.OperationExecutionEvidenceCommitted)},
		{DispatchID: "dispatch-missing-operation", OperationID: "", ExecutionState: string(runtimecontract.OperationExecutionEvidenceCommitted)},
	}

	got := committedRuntimeOperationEvidenceRefs(operations)
	want := "workorch://runtime-operations/dispatch-committed/operation-committed"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("operation evidence refs = %+v, want only %q", got, want)
	}
}
