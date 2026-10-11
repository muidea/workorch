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
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// File producer 是实际 owner；receipt store 是显式 typed 替身，不替代 Orchestrator 来源验收。
func TestFileReadDependencyQueryObservesCurrentBytesWithoutReplacingReceipt(t *testing.T) {
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
	directory := t.TempDir()
	path := "source.txt"
	raw := []byte("first\r\n原文\r\nlast\n")
	if err := os.WriteFile(filepath.Join(directory, path), raw, 0600); err != nil {
		t.Fatal(err)
	}
	start, end, bound := int64(2), int64(2), int64(128)
	operation := runtimecontract.RuntimeOperation{ID: "read", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{path}, Parameters: []runtimecontract.OperationParameter{
		{Name: "startLine", Integer: &start}, {Name: "endLine", Integer: &end}, {Name: "maxBytes", Integer: &bound},
	}}
	sealed, err := fileevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	scope := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "read", TargetRefs: []string{path}, WorkspaceDeliverableDirectory: directory, EvidenceKind: runtimecontract.EvidenceKindArtifact,
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority", DefinitionRevision: "definition"},
		Execution:   fileBizTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws/run/dispatch/read", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone}),
	}
	var persisted *runtimecontract.RuntimeOperationReceipt
	commits, sourceQueries := 0, 0
	queryMode := "valid"
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		if command.ReadSource != nil {
			sourceQueries++
			if event.Source() != ModuleID || !reflect.DeepEqual(*command.ReadSource, scope) {
				t.Error("未携带原 sealed source")
			}
			if queryMode == "missing" {
				result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{}, nil)
				return
			}
		}
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: persisted != nil, Receipt: persisted}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		commits++
		value := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand).Receipt.Clone()
		persisted = &value
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: value}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	data, executeErr := hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileReadOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ExecuteFileReadOperationCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed})).Get()
	if executeErr != nil {
		t.Fatal(executeErr)
	}
	receipt := data.(fileevents.ExecuteOperationResult).Receipt
	original := receipt.Clone()
	command := fileevents.QueryRuntimeFileReadDependenciesCommand{
		RuntimeOperationExecutionContext: scope, SealedOperation: sealed, RequestID: "observation", IssuedAtStateRevision: 7,
		SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt), OriginalDependencies: *runtimecontract.CloneRuntimeOwnerRevisionSet(receipt.Evidence.OwnerRevisionSet),
	}
	send := func(source string, value fileevents.QueryRuntimeFileReadDependenciesCommand) (fileevents.QueryRuntimeFileReadDependenciesResult, error) {
		event := magicevent.NewEventWithContext(fileevents.TopicQueryRuntimeFileReadDependencies, source, ModuleID, magicevent.NewHeader(), ctx, value)
		data, err := hub.Send(event).Get()
		if err != nil {
			return fileevents.QueryRuntimeFileReadDependenciesResult{}, err
		}
		response, ok := data.(fileevents.QueryRuntimeFileReadDependenciesResult)
		if !ok {
			t.Fatalf("错误 response %T", data)
		}
		return response, nil
	}
	assertCurrent := func(raw []byte, selected string, expectedMatch bool) {
		t.Helper()
		response, err := send(runtimecontract.RuntimeModuleOrchestrator, command)
		if err != nil {
			t.Fatal(err)
		}
		set, observed, err := fileevents.ValidateRuntimeFileReadDependenciesResult(command, response)
		if err != nil || !observed {
			t.Fatalf("观察未完成: %v", err)
		}
		digest := sha256.Sum256(raw)
		hash := hex.EncodeToString(digest[:])
		if set.Entries[0].Revision != hash {
			t.Fatal("不是实际 current 全流 bytes hash")
		}
		body, err := json.Marshal(struct {
			WorkspaceID, TargetRef, ContentHash, SelectedContent string
			StartLine, EndLine, MaxBytes                         int64
			Truncated, Binary, ContentComplete                   bool
		}{"ws", path, hash, selected, start, end, bound, false, false, false})
		if err != nil {
			t.Fatal(err)
		}
		selector := sha256.Sum256(append([]byte("file-read-dependency/v1\x00"), body...))
		if set.Entries[0].DependencyHash != hex.EncodeToString(selector[:]) {
			t.Fatal("未按原 range／bound 重算 current selector")
		}
		match, err := runtimecontract.RuntimeOwnerRevisionSetMatchesCurrent(command.OriginalDependencies, *set)
		if err != nil || match != expectedMatch {
			t.Fatalf("current matcher=%v err=%v", match, err)
		}
		if commits != 1 || !reflect.DeepEqual(*persisted, original) {
			t.Fatal("只读 query 改写原 receipt")
		}
		set.Entries[0].Revision = "caller"
		if response.CurrentDependencies.Entries[0].Revision != hash {
			t.Fatal("validator 未隔离副本")
		}
	}
	assertCurrent(raw, "原文\r\n", true)
	if err := os.WriteFile(filepath.Join(directory, "unrelated.txt"), []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	assertCurrent(raw, "原文\r\n", true)
	changed := []byte("first\r\n新文\r\nlast\n")
	if err := os.WriteFile(filepath.Join(directory, path), changed, 0600); err != nil {
		t.Fatal(err)
	}
	assertCurrent(changed, "新文\r\n", false)
	for _, scenario := range []string{"source", "request", "revision", "receipt", "raw", "hash", "resource"} {
		t.Run(scenario, func(t *testing.T) {
			value := command
			value.OriginalDependencies = *runtimecontract.CloneRuntimeOwnerRevisionSet(&command.OriginalDependencies)
			source := runtimecontract.RuntimeModuleOrchestrator
			switch scenario {
			case "source":
				source = " " + source
			case "request":
				value.RequestID = " observation"
			case "revision":
				value.IssuedAtStateRevision = 0
			case "receipt":
				value.SourceReceiptHash = "wrong"
			case "raw":
				value.OriginalDependencies.Entries[0].Revision = " " + value.OriginalDependencies.Entries[0].Revision
			case "hash":
				value.OriginalDependencies.SetHash = "wrong"
			case "resource":
				value.OriginalDependencies.Entries[0].ResourceID = "other"
				value.OriginalDependencies = value.OriginalDependencies.WithHash()
			}
			if _, err := send(source, value); err == nil {
				t.Fatal("非法 query 被接受")
			}
		})
	}
	queryMode = "missing"
	if _, err := send(runtimecontract.RuntimeModuleOrchestrator, command); err == nil {
		t.Fatal("receipt miss 不得授 query 执行")
	}
	queryMode = "valid"
	if err := os.Remove(filepath.Join(directory, path)); err != nil {
		t.Fatal(err)
	}
	missing, err := send(runtimecontract.RuntimeModuleOrchestrator, command)
	if err != nil {
		t.Fatal(err)
	}
	if set, observed, err := fileevents.ValidateRuntimeFileReadDependenciesResult(command, missing); err != nil || observed || set != nil || missing.Status != fileevents.RuntimeFileReadDependencyMissing {
		t.Fatalf("missing target 被伪造成完整 current set: %+v err=%v", missing, err)
	}
	if err := os.Mkdir(filepath.Join(directory, path), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := send(runtimecontract.RuntimeModuleOrchestrator, command); err == nil {
		t.Fatal("directory 不能冒充 missing 或 actual read")
	}
	if err := os.Remove(filepath.Join(directory, path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, path), changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err = New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	assertCurrent(changed, "新文\r\n", false)
	owner.BeginShutdown(ctx)
	if _, err := send(runtimecontract.RuntimeModuleOrchestrator, command); err == nil {
		t.Fatal("关闭后不得新读取依赖")
	}
	if commits != 1 || sourceQueries == 0 {
		t.Fatal("未走 source query 或产生额外 receipt")
	}
}
