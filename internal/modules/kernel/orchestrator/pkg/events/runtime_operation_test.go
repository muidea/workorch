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

func TestFileEditOperationUsesConcreteParameterContract(t *testing.T) {
	path := "target.txt"
	oldText := "hello"
	newText := "world"
	data, err := FileEditOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{path},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "path", String: &path}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.Path != path || data.OldText != oldText || data.NewText != newText {
		t.Fatalf("file/edit data = %+v", data)
	}
}

func TestGenericRuntimeOperationBridgeRejectsConcreteFileKinds(t *testing.T) {
	adapter := &runtimeOperationAdapterStub{}
	for _, kind := range []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply} {
		_, err := ExecuteRuntimeOperation(context.Background(), ExecuteRuntimeOperationCommand{
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			Operation:   runtimecontract.RuntimeOperation{ID: "operation-1", Kind: kind}, EvidenceKind: runtimecontract.EvidenceKindStateChange,
		}, adapter)
		if err == nil {
			t.Fatalf("generic bridge accepted concrete file kind %q", kind)
		}
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
