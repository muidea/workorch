package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeOperationFeedbackForTest(t *testing.T, aggregate RuntimeAggregate, operationID string, failure runtimecontract.FailureFact) *runtimecontract.RuntimeFeedback {
	t.Helper()
	if failure.Code == "" {
		failure.Code = string(failure.Class)
	}
	if failure.Signature == "" {
		failure.Signature = "test-operation-rejection"
	}
	if aggregate.ActiveDispatch == nil {
		t.Fatalf("feedback fixture requires active dispatch: aggregate=%+v", aggregate.ActiveDispatch)
	}
	operation, found := candidateOperationByID(aggregate.ActiveDispatch.Candidate, operationID)
	if !found {
		t.Fatalf("feedback fixture operation %q is not sealed", operationID)
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found {
		t.Fatalf("feedback fixture operation kind %q is unknown", operation.Kind)
	}
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		AuthoritySnapshotID: aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: aggregate.ActiveDispatch.ID,
	}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected), descriptor.OwnerModuleID, failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = operationID
	return &feedback
}

func TestRuntimeControllerOperationReceiptFenceAndTerminalUniqueness(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("receipt")},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	sealedExecution, ok := operationExecutionRecord(sealed.ActiveDispatch.OperationExecutions, operationID)
	if !ok {
		t.Fatal("sealed execution is missing")
	}
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, operationID, sealedExecution.ExecutionKey, 1, nil); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unclaimed receipt query err = %v, want conflict", err)
	}
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", operationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !ok {
		t.Fatal("claimed execution is missing")
	}
	claimedBudget, err := runtimecontract.BuildRuntimeExecutionBudgetView(claimed.Facts.WorkspaceID, claimed.Facts.RunID, claimed.StateRevision, claimed.ExecutionBudget, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeExecutionDimension(t, claimedBudget, runtimecontract.RuntimeBudgetDimensionCapabilityOperation, 0, 1, 0)
	if _, found, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", claimed.ActiveDispatch.ID, operationID, execution.ExecutionKey, execution.Fence, nil); err != nil || found {
		t.Fatalf("claimed receipt query found=%v err=%v, want authorized found=false", found, err)
	}
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", "different-dispatch", operationID, execution.ExecutionKey, execution.Fence, nil); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong dispatch receipt query err = %v, want conflict", err)
	}
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", claimed.ActiveDispatch.ID, operationID, execution.ExecutionKey+"-wrong", execution.Fence, nil); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong execution key receipt query err = %v, want conflict", err)
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "written"},
	}
	if _, _, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence+1, receipt); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale fence err = %v, want conflict", err)
	}
	committed, first, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, receipt)
	if err != nil {
		t.Fatal(err)
	}
	committedBudget, err := runtimecontract.BuildRuntimeExecutionBudgetView(committed.Facts.WorkspaceID, committed.Facts.RunID, committed.StateRevision, committed.ExecutionBudget, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeExecutionDimension(t, committedBudget, runtimecontract.RuntimeBudgetDimensionCapabilityOperation, 1, 0, 0)
	queried, found, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", claimed.ActiveDispatch.ID, operationID, execution.ExecutionKey, execution.Fence, nil)
	if err != nil || !found || !runtimecontract.SameRuntimeOperationReceipt(first, queried) {
		t.Fatalf("committed receipt query found=%v receipt=%+v err=%v", found, queried, err)
	}
	saves := repository.saves
	replayed, duplicate, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, receipt)
	if err != nil || !runtimecontract.SameRuntimeOperationReceipt(first, duplicate) || replayed.StateRevision != committed.StateRevision || repository.saves != saves {
		t.Fatalf("duplicate receipt was not idempotent: aggregate=%+v receipt=%+v saves=%d err=%v", replayed, duplicate, repository.saves, err)
	}
	conflict := receipt
	conflict.Evidence.Summary = "different terminal outcome"
	if _, _, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, conflict); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different receipt for same execution key err = %v, want conflict", err)
	}
}
