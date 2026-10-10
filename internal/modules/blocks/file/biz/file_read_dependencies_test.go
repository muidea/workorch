package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileReadOwnerProducesActualDependenciesAndReplaysOriginalReceipt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Teardown(ctx); err != nil {
			t.Error(err)
		}
	})
	workspace := t.TempDir()
	raw := []byte("原文\r\nsecond\nthird\n")
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	var persisted *runtimecontract.RuntimeOperationReceipt
	commits := 0
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: persisted != nil, Receipt: persisted}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		commits++
		payload, err := json.Marshal(command.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		var loaded runtimecontract.RuntimeOperationReceipt
		if err := json.Unmarshal(payload, &loaded); err != nil {
			t.Fatal(err)
		}
		persisted = &loaded
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: loaded}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	path := "source.txt"
	maxBytes := int64(5)
	operation := runtimecontract.RuntimeOperation{ID: "read", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{path}, Parameters: []runtimecontract.OperationParameter{{Name: "maxBytes", Integer: &maxBytes}}}
	sealed, err := fileevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	command := fileevents.ExecuteFileReadOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution:   fileBizTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws/run/dispatch/read", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone}),
			WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "read",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority", DefinitionRevision: "definition"},
			TargetRefs:  []string{path}, EvidenceKind: runtimecontract.EvidenceKindArtifact, WorkspaceDeliverableDirectory: workspace,
		}, SealedOperation: sealed,
	}
	send := func(value fileevents.ExecuteFileReadOperationCommand) (fileevents.ExecuteOperationResult, error) {
		data, err := hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileReadOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, value)).Get()
		if err != nil {
			return fileevents.ExecuteOperationResult{}, err
		}
		response, ok := data.(fileevents.ExecuteOperationResult)
		if !ok {
			t.Fatalf("真实 File 返回错误类型 %T", data)
		}
		return response, nil
	}
	for _, definition := range []string{"", " definition", "definition "} {
		missing := command
		missing.Transaction.DefinitionRevision = definition
		if _, err := send(missing); err == nil || commits != 0 {
			t.Fatal("无 exact Definition 读取不得产生 receipt")
		}
	}
	response, err := send(command)
	if err != nil {
		t.Fatal(err)
	}
	dependencies := response.Receipt.Evidence.OwnerRevisionSet
	digest := sha256.Sum256(raw)
	contentHash := hex.EncodeToString(digest[:])
	if dependencies == nil || dependencies.Validate() != nil || dependencies.DefinitionRevision != "definition" || len(dependencies.Entries) != 1 {
		t.Fatalf("缺实际读取依赖: %+v", dependencies)
	}
	entry := dependencies.Entries[0]
	if entry.OwnerKind != "capability" || entry.OwnerInstanceID != ModuleID+"/ws" || entry.ResourceKind != string(runtimecontract.RuntimeResourceWorkspaceTarget) || entry.ResourceID != path || entry.Revision != contentHash || entry.DependencyHash == "" || response.Receipt.Evidence.ContentHash != contentHash || len(response.Receipt.ResourceChanges) != 0 {
		t.Fatalf("依赖不是本次全流 bytes 或混入写入变更: %+v", response.Receipt)
	}
	// 独立前像使用已知输入字节和 sealed selector，不调用 producer helper。
	// 上限截在第二个中文字符中；当前 adapter 只返回完整 UTF-8 前缀“原”。
	preimage, err := json.Marshal(struct {
		WorkspaceID, TargetRef, ContentHash, SelectedContent string
		StartLine, EndLine, MaxBytes                         int64
		Truncated, Binary, ContentComplete                   bool
	}{"ws", path, contentHash, "原", 0, 0, maxBytes, true, false, false})
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(append([]byte("file-read-dependency/v1\x00"), preimage...))
	if entry.DependencyHash != hex.EncodeToString(expected[:]) {
		t.Fatal("实际读取依赖与独立 bytes／selector 前像不符")
	}
	if err := os.WriteFile(filepath.Join(workspace, path), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	owner.BeginShutdown(ctx)
	replay, err := send(command)
	if err != nil || commits != 1 || !reflect.DeepEqual(replay.Receipt, response.Receipt) {
		t.Fatalf("重放用 current 文件替换历史读取: %v commits=%d", err, commits)
	}
	response.Receipt.Evidence.OwnerRevisionSet.Entries[0].Revision = "caller-change"
	if persisted.Evidence.OwnerRevisionSet.Entries[0].Revision != contentHash {
		t.Fatal("typed response 与保存的依赖共享可变 entries")
	}
}

func TestFileReadDependenciesBindScopeAndExactSelection(t *testing.T) {
	command := fileevents.ExecuteFileReadOperationCommand{RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: common.WorkspaceID("ws"), Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: "definition"}}}
	observed := FileReadResult{Exists: true, TargetRef: "source.txt", ContentHash: "actual-stream-hash", Content: "原文\r\n", ContentComplete: true}
	original, err := fileReadOwnerDependencies(command, observed, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"workspace", "target", "content", "selected", "range", "bound", "truncated", "binary", "complete"} {
		t.Run(scenario, func(t *testing.T) {
			value, scope := observed, command
			start, end, bound := int64(0), int64(0), int64(0)
			switch scenario {
			case "workspace":
				scope.WorkspaceID = "other"
			case "target":
				value.TargetRef = "other"
			case "content":
				value.ContentHash = "changed"
			case "selected":
				value.Content = "changed"
			case "range":
				start, end = 1, 2
			case "bound":
				bound = 5
			case "truncated":
				value.Truncated = true
			case "binary":
				value.Binary = true
			case "complete":
				value.ContentComplete = false
			}
			changed, err := fileReadOwnerDependencies(scope, value, start, end, bound)
			if err != nil || changed.Entries[0].DependencyHash == original.Entries[0].DependencyHash || changed.SetHash == original.SetHash {
				t.Fatalf("实际读取 scope／selector 未绑定: %v", err)
			}
		})
	}
}
