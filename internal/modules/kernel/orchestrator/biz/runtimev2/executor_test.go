package runtimev2

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type recordingOperationExecutor struct {
	requests []RuntimeOperationExecutionRequest
	rejectID string
}

func (e *recordingOperationExecutor) ExecuteRuntimeOperation(_ context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	e.requests = append(e.requests, request)
	result := RuntimeOperationExecutionResult{
		EventID: "event-" + request.Operation.ID, OperationID: request.Operation.ID,
		EvidenceKind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
		SourceRef: "runtime://" + request.Operation.ID,
	}
	if request.Operation.ID == e.rejectID {
		result.Status = ExecutionEvidenceStatusRejected
		result.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "failure-" + request.Operation.ID, Message: "operation failed"}
	}
	return result, nil
}

func TestRuntimeControllerExecutesDependencyOrderedOperationsAndCommitsEvidence(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: runtimecontract.StepKindFileEdit}}
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate := multiOperationCandidate()
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, candidate); err != nil {
		t.Fatal(err)
	}
	executor := &recordingOperationExecutor{}
	completed, err := controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatalf("ExecuteActiveDispatch: %v", err)
	}
	if len(executor.requests) != 2 || executor.requests[0].Operation.ID != "write-a" || executor.requests[1].Operation.ID != "write-b" {
		t.Fatalf("execution order = %+v", executor.requests)
	}
	if len(executor.requests[0].EvidencePolicies) != 1 || executor.requests[0].EvidencePolicies[0].ID != "policy-1" {
		t.Fatalf("executor request lost typed evidence policy: %+v", executor.requests[0].EvidencePolicies)
	}
	if completed.ActiveDispatch != nil || len(completed.CompletedDispatches) != 1 || completed.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("completed aggregate = %+v", completed)
	}
	if completed.EvidenceSequence != 2 || len(completed.Facts.Evidence.Accepted) != 2 {
		t.Fatalf("evidence state = sequence %d facts %+v", completed.EvidenceSequence, completed.Facts.Evidence)
	}
}

func TestRuntimeControllerStopsDispatchAfterRejectedOperation(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, multiOperationCandidate()); err != nil {
		t.Fatal(err)
	}
	executor := &recordingOperationExecutor{rejectID: "write-a"}
	failed, err := controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatalf("ExecuteActiveDispatch: %v", err)
	}
	if len(executor.requests) != 1 || failed.ActiveDispatch != nil || len(failed.CompletedDispatches) != 1 {
		t.Fatalf("failed dispatch = requests %+v aggregate %+v", executor.requests, failed)
	}
	if failed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || failed.Facts.Failure == nil {
		t.Fatalf("failed decision = %+v facts=%+v", failed.Decision, failed.Facts)
	}
}

func beginAndAcceptCandidate(t *testing.T, controller *RuntimeController, aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	t.Helper()
	if _, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
		return RuntimeAggregate{}, err
	}
	return controller.AcceptCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
}

func multiOperationCandidate() runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: runtimecontract.RuntimeOperation{ID: "write-b", Kind: "file/write", DependsOn: []string{"write-a"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"b.txt"}}},
		{Operation: runtimecontract.RuntimeOperation{ID: "write-a", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"a.txt"}}},
	}}}
}
