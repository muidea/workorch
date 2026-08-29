package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateRuntimeOperationReceiptExecutionLineageAcceptsExactBidirectionalProjection(t *testing.T) {
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch-source/op-1", 3, "op-1")
	execution := runtimeOperationReceiptLineageExecution(receipt, OperationExecutionOwnerCommitted)
	active := RuntimeDispatchRecord{ID: "dispatch-resume", OperationExecutions: []RuntimeOperationExecutionRecord{execution}}

	completedClone := execution.Clone()
	completedClone.State = OperationExecutionEvidenceCommitted
	completedClone.EvidenceFactID = "evidence-op-1"
	completedClone.EvidenceFactHash = "evidence-hash-1"
	completed := []RuntimeDispatchRecord{{ID: "dispatch-source", OperationExecutions: []RuntimeOperationExecutionRecord{completedClone}}}
	if err := ValidateRuntimeOperationReceiptExecutionLineage(&active, completed, []RuntimeOperationReceipt{receipt}); err != nil {
		t.Fatalf("validate exact receipt lineage: %v", err)
	}
}

func TestValidateRuntimeOperationReceiptExecutionLineageRejectsMissingAndDuplicateReceipt(t *testing.T) {
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch-1/op-1", 1, "op-1")
	execution := runtimeOperationReceiptLineageExecution(receipt, OperationExecutionOwnerCommitted)
	active := RuntimeDispatchRecord{ID: "dispatch-1", OperationExecutions: []RuntimeOperationExecutionRecord{execution}}

	if err := ValidateRuntimeOperationReceiptExecutionLineage(&active, nil, nil); err == nil || !strings.Contains(err.Error(), "has no owner receipt") {
		t.Fatalf("expected missing receipt rejection, got %v", err)
	}
	if err := ValidateRuntimeOperationReceiptExecutionLineage(&active, nil, []RuntimeOperationReceipt{receipt, receipt.Clone()}); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("expected duplicate receipt rejection, got %v", err)
	}
}

func TestValidateRuntimeOperationReceiptExecutionLineageRejectsMismatchedBindings(t *testing.T) {
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch-1/op-1", 2, "op-1")

	tests := []struct {
		name       string
		mutate     func(*RuntimeOperationExecutionRecord)
		wantReason string
	}{
		{
			name: "operation",
			mutate: func(execution *RuntimeOperationExecutionRecord) {
				execution.OperationID = "op-other"
				execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(*execution)
			},
			wantReason: "operation mismatch",
		},
		{
			name: "receipt ref",
			mutate: func(execution *RuntimeOperationExecutionRecord) {
				execution.OwnerReceiptRef = "runtime-receipt/wrong"
			},
			wantReason: "receipt ref mismatch",
		},
		{
			name: "receipt hash",
			mutate: func(execution *RuntimeOperationExecutionRecord) {
				execution.OwnerReceiptHash = "wrong-hash"
			},
			wantReason: "receipt hash mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execution := runtimeOperationReceiptLineageExecution(receipt, OperationExecutionOwnerCommitted)
			tt.mutate(&execution)
			active := RuntimeDispatchRecord{ID: "dispatch-1", OperationExecutions: []RuntimeOperationExecutionRecord{execution}}
			err := ValidateRuntimeOperationReceiptExecutionLineage(&active, nil, []RuntimeOperationReceipt{receipt})
			if err == nil || !strings.Contains(err.Error(), tt.wantReason) {
				t.Fatalf("expected %s rejection, got %v", tt.wantReason, err)
			}
		})
	}
}

func TestValidateRuntimeOperationReceiptExecutionLineageRejectsOrphanReceipt(t *testing.T) {
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch-1/op-1", 1, "op-1")
	if err := ValidateRuntimeOperationReceiptExecutionLineage(nil, nil, []RuntimeOperationReceipt{receipt}); err == nil || !strings.Contains(err.Error(), "no committed dispatch execution projection") {
		t.Fatalf("expected orphan receipt rejection, got %v", err)
	}
}

func TestValidateRuntimeOperationReceiptExecutionLineageRejectsReceiptOnNonCommittedExecution(t *testing.T) {
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch-1/op-1", 1, "op-1")
	execution := runtimeOperationReceiptLineageExecution(receipt, OperationExecutionClaimed)
	active := RuntimeDispatchRecord{ID: "dispatch-1", OperationExecutions: []RuntimeOperationExecutionRecord{execution}}
	if err := ValidateRuntimeOperationReceiptExecutionLineage(&active, nil, []RuntimeOperationReceipt{receipt}); err == nil || !strings.Contains(err.Error(), "claimed operation execution") {
		t.Fatalf("expected non-committed receipt rejection, got %v", err)
	}
}

func runtimeOperationReceiptLineageFixture(executionKey string, fence uint64, operationID string) RuntimeOperationReceipt {
	return RuntimeOperationReceipt{
		ExecutionKey:        executionKey,
		Fence:               fence,
		OperationID:         operationID,
		Outcome:             RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: RuntimeEffectWorkspaceDurable,
		Changed:             true,
		TargetRefs:          []string{"artifact.txt"},
		BeforeHash:          "before",
		AfterHash:           "after",
		Evidence: RuntimeOperationEvidence{
			Summary:     "updated artifact",
			ContentHash: "evidence-hash",
		},
	}
}

func runtimeOperationReceiptLineageExecution(receipt RuntimeOperationReceipt, state RuntimeOperationExecutionState) RuntimeOperationExecutionRecord {
	execution := RuntimeOperationExecutionRecord{
		OperationID:         receipt.OperationID,
		OperationHash:       "operation-hash",
		ExecutionKey:        receipt.ExecutionKey,
		ExpectedEffectScope: RuntimeEffectWorkspaceDurable,
		IdempotencyClass:    RuntimeOperationIdempotencyAtMostOnce,
		State:               state,
		Fence:               receipt.Fence,
		OwnerReceiptRef:     "runtime-receipt/" + receipt.ExecutionKey,
		OwnerReceiptHash:    StableRuntimeOperationReceiptHash(receipt),
		ClaimedAt:           RuntimeOperationClaimMarkerInitial,
	}
	if state == OperationExecutionEvidenceCommitted {
		execution.EvidenceFactID = common.EvidenceRecordID("evidence-" + receipt.OperationID)
		execution.EvidenceFactHash = "evidence-hash-" + receipt.OperationID
	}
	execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(execution)
	return execution
}
