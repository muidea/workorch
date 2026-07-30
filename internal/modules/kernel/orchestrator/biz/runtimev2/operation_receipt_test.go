package runtimev2

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func attachOperationReceiptFeedbackForTest(t *testing.T, aggregate RuntimeAggregate, receipt runtimecontract.RuntimeOperationReceipt) runtimecontract.RuntimeOperationReceipt {
	t.Helper()
	if aggregate.ActiveDispatch == nil || receipt.Failure == nil {
		t.Fatalf("feedback fixture requires active dispatch and failure: aggregate=%+v receipt=%+v", aggregate.ActiveDispatch, receipt)
	}
	operation, found := candidateOperationByID(aggregate.ActiveDispatch.Candidate, receipt.OperationID)
	if !found {
		t.Fatalf("feedback fixture operation %q is not sealed", receipt.OperationID)
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found {
		t.Fatalf("feedback fixture operation kind %q is unknown", operation.Kind)
	}
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		AuthoritySnapshotID: aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: aggregate.ActiveDispatch.ID,
	}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(receipt.Outcome), descriptor.OwnerModuleID, *receipt.Failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = receipt.OperationID
	receipt.Feedback = &feedback
	return receipt
}

func TestRuntimeControllerOperationReceiptFenceAndTerminalUniqueness(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("receipt")},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", "write-1")
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, "write-1")
	if !ok {
		t.Fatal("claimed execution is missing")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "written"},
	}
	if _, _, err := controller.CommitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence+1, receipt); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale fence err = %v, want conflict", err)
	}
	committed, first, err := controller.CommitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, receipt)
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	replayed, duplicate, err := controller.CommitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, receipt)
	if err != nil || !runtimecontract.SameRuntimeOperationReceipt(first, duplicate) || replayed.StateRevision != committed.StateRevision || repository.saves != saves {
		t.Fatalf("duplicate receipt was not idempotent: aggregate=%+v receipt=%+v saves=%d err=%v", replayed, duplicate, repository.saves, err)
	}
	conflict := receipt
	conflict.Evidence.Summary = "different terminal outcome"
	if _, _, err := controller.CommitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, conflict); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different receipt for same execution key err = %v, want conflict", err)
	}
}
