package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 实际 File 与调用端；Workspace/receipt 使用 typed fixture，原 aggregate 来源另由 receipt owner 测试反证。
func TestRuntimeFileReadDependencyAdapterUsesOwnerDirectoryAndCurrentRead(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	file, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Teardown(ctx); err != nil {
			t.Error(err)
		}
	})
	directory := t.TempDir()
	raw := []byte("原文\r\n")
	if err := os.WriteFile(filepath.Join(directory, "source.txt"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	workspace := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspace.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(workspaceevents.GetWorkspaceCommand)
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: command.WorkspaceID, DeliverableDirectory: directory}}, nil)
	})
	t.Cleanup(func() { workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	operation := runtimecontract.RuntimeOperation{ID: "read", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{"source.txt"}}
	sealed, err := fileevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	execution := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, ExecutionKey: "exec/ws/run/dispatch/read", Fence: 1, OperationHash: runtimecontract.StableRuntimeOperationHash(operation),
		ExpectedEffectScope: runtimecontract.ResolveExpectedEffectScope(operation, nil), IdempotencyClass: runtimecontract.ResolveOperationIdempotencyClass(operation, nil),
	}
	execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(execution)
	scope := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "read", TargetRefs: operation.TargetRefs,
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority", DefinitionRevision: "definition"},
		Execution:   runtimeOperationExecutionEnvelope(execution, nil), EvidenceKind: runtimecontract.EvidenceKindArtifact, WorkspaceDeliverableDirectory: directory,
	}
	var persisted *runtimecontract.RuntimeOperationReceipt
	commits := 0
	receiptOwner := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		if command.ReadSource != nil && command.ReadSource.WorkspaceDeliverableDirectory != directory {
			t.Error("原 query 没有 owner 目录")
		}
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: persisted != nil, Receipt: persisted}, nil)
	})
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		commits++
		value := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand).Receipt.Clone()
		persisted = &value
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: value}, nil)
	})
	t.Cleanup(func() {
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	_, magicErr := hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileReadOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, fileevents.ExecuteFileReadOperationCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed})).Get()
	if magicErr != nil || persisted == nil {
		t.Fatalf("original actual read err=%v", magicErr)
	}
	original := persisted.Clone()
	owner := newTestBiz(newRuntimeTestOwner(), hub, nil)
	command := fileevents.QueryRuntimeFileReadDependenciesCommand{
		RuntimeOperationExecutionContext: scope, SealedOperation: sealed, RequestID: "observation", IssuedAtStateRevision: 7,
		SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(original), OriginalDependencies: *runtimecontract.CloneRuntimeOwnerRevisionSet(original.Evidence.OwnerRevisionSet),
	}
	command.WorkspaceDeliverableDirectory = "/must-not-use-caller-directory"
	for _, changed := range []bool{false, true} {
		bytes := raw
		if changed {
			bytes = []byte("变化\r\n")
			if err := os.WriteFile(filepath.Join(directory, "source.txt"), bytes, 0600); err != nil {
				t.Fatal(err)
			}
		}
		current, observed, err := owner.queryRuntimeFileReadDependencies(ctx, command)
		if err != nil || !observed {
			t.Fatalf("typed adapter observation err=%v", err)
		}
		digest := sha256.Sum256(bytes)
		if current.Entries[0].Revision != hex.EncodeToString(digest[:]) {
			t.Fatal("adapter 未返回实际 bytes hash")
		}
		match, err := runtimecontract.RuntimeOwnerRevisionSetMatchesCurrent(command.OriginalDependencies, *current)
		if err != nil || match == changed {
			t.Fatalf("actual applicability match=%v err=%v", match, err)
		}
	}
	if commits != 1 || !reflect.DeepEqual(*persisted, original) || command.WorkspaceDeliverableDirectory != "/must-not-use-caller-directory" {
		t.Fatal("query 改写 receipt 或 caller value")
	}
}
