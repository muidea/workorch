package events

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeOperationAdapterStub struct {
	request runtimecontract.StepExecutionRequest
	result  runtimecontract.StepExecutionResult
	err     error
}

func (s *runtimeOperationAdapterStub) ExecuteRuntimeStep(_ context.Context, request runtimecontract.StepExecutionRequest) (runtimecontract.StepExecutionResult, error) {
	s.request = request
	return s.result, s.err
}

func TestExecuteRuntimeOperationUsesTypedCommandAndConcreteResult(t *testing.T) {
	content := "hello"
	adapter := &runtimeOperationAdapterStub{result: runtimecontract.StepExecutionResult{Output: map[string]any{
		"summary": "written", "contentHash": "hash-1", "evidenceRefs": []string{"artifact-1"},
	}}}
	result, err := ExecuteRuntimeOperation(context.Background(), ExecuteRuntimeOperationCommand{
		WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1"},
		Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"target.txt"},
			Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &content}},
		},
		EvidenceKind:     runtimecontract.EvidenceKindStateChange,
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: runtimecontract.StepKindFileEdit}},
	}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || result.OperationID != "write-1" || result.Evidence.ContentHash != "hash-1" {
		t.Fatalf("result = %+v", result)
	}
	if adapter.request.Kind != runtimecontract.StepKindFileEdit || adapter.request.Input["path"] != "hello" {
		t.Fatalf("adapter request = %+v", adapter.request)
	}
	if len(adapter.request.EvidencePolicies) != 1 || adapter.request.EvidencePolicies[0].ID != "policy-1" {
		t.Fatalf("adapter request lost typed evidence policies: %+v", adapter.request.EvidencePolicies)
	}
}

func TestExecuteRuntimeOperationConvertsAdapterFailureToRejectedEvidence(t *testing.T) {
	adapter := &runtimeOperationAdapterStub{err: errors.New("command failed")}
	result, err := ExecuteRuntimeOperation(context.Background(), ExecuteRuntimeOperationCommand{
		WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1",
		Transaction:  runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1"},
		Operation:    runtimecontract.RuntimeOperation{ID: "shell-1", Kind: runtimecontract.StepKindShellCommand, TargetRefs: []string{"workspace"}},
		EvidenceKind: runtimecontract.EvidenceKindValidation,
	}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != runtimecontract.RuntimeOperationExecutionStatusRejected || result.Failure == nil || result.Failure.Class != runtimecontract.FailureClassExecutionFailure {
		t.Fatalf("result = %+v", result)
	}
}
