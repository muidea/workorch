package file

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileMutationApplicabilityQueryUsesOriginalReceiptAndActualCurrentBytes(t *testing.T) {
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
	content := "完整原文\r\n"
	operation := runtimecontract.RuntimeOperation{ID: "create", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"target.txt"}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}}}
	sealed, err := fileevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	scope := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "create", TargetRefs: operation.TargetRefs,
		WorkspaceDeliverableDirectory: directory, EvidenceKind: runtimecontract.EvidenceKindStateChange,
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority", DefinitionRevision: "definition"},
		Execution:   fileBizTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws/run/dispatch/create", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable}),
	}
	var persisted *runtimecontract.RuntimeOperationReceipt
	commits, queries := 0, 0
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		if command.ReadSource != nil {
			queries++
			if !reflect.DeepEqual(*command.ReadSource, scope) {
				t.Error("query did not carry exact original execution context")
			}
		}
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: persisted != nil, Receipt: persisted}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		commits++
		receipt := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand).Receipt.Clone()
		persisted = &receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	data, executeErr := hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileWriteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, fileevents.ExecuteFileWriteOperationCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed})).Get()
	if executeErr != nil {
		t.Fatal(executeErr)
	}
	receipt := data.(fileevents.ExecuteOperationResult).Receipt
	original := receipt.Clone()
	if receipt.Evidence.ApplicabilityDependencies == nil || receipt.Evidence.OwnerRevisionSet.Entries[0].ResourceKind != "workspace_target_existence" {
		t.Fatal("real creation receipt lacks independent absence and content observations")
	}
	command := fileevents.QueryRuntimeFileReadDependenciesCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed, RequestID: "observe", IssuedAtStateRevision: 7,
		SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt), OriginalDependencies: *runtimecontract.CloneRuntimeOwnerRevisionSet(receipt.Evidence.OwnerRevisionSet),
		ApplicabilitySource: runtimecontract.CloneRuntimeEvidenceApplicabilityDependencies(receipt.Evidence.ApplicabilityDependencies)}
	send := func(value fileevents.QueryRuntimeFileReadDependenciesCommand) (fileevents.QueryRuntimeFileReadDependenciesResult, error) {
		data, err := hub.Send(magicevent.NewEventWithContext(fileevents.TopicQueryRuntimeFileReadDependencies, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, value)).Get()
		if err != nil {
			return fileevents.QueryRuntimeFileReadDependenciesResult{}, err
		}
		return data.(fileevents.QueryRuntimeFileReadDependenciesResult), nil
	}
	assertObserved := func(body []byte, match bool) {
		t.Helper()
		response, err := send(command)
		if err != nil {
			t.Fatal(err)
		}
		set, found, err := fileevents.ValidateRuntimeFileReadDependenciesResult(command, response)
		if err != nil || !found || set.Entries[0].Revision != sha256Hex(body) {
			t.Fatalf("actual observation: %v %+v", err, set)
		}
		matches, err := runtimecontract.RuntimeOwnerRevisionSetMatchesCurrent(command.ApplicabilitySource.OwnerRevisionSet, *set)
		if err != nil || matches != match {
			t.Fatalf("applicability match=%v err=%v", matches, err)
		}
		if commits != 1 || !reflect.DeepEqual(*persisted, original) {
			t.Fatal("read-only observation changed receipt/effect")
		}
		set.Entries[0].Revision = "caller-change"
		if response.CurrentDependencies.Entries[0].Revision != sha256Hex(body) {
			t.Fatal("current response not detached")
		}
	}
	assertObserved([]byte(content), true)
	if err := os.WriteFile(filepath.Join(directory, "unrelated.txt"), []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	assertObserved([]byte(content), true)
	changed := []byte{0, 0xff, '\r', '\n'}
	if err := os.WriteFile(filepath.Join(directory, "target.txt"), changed, 0600); err != nil {
		t.Fatal(err)
	}
	assertObserved(changed, false)
	for _, scenario := range []string{"receipt", "original_reads", "original_current", "protocol", "resource"} {
		value := command
		value.OriginalDependencies = *runtimecontract.CloneRuntimeOwnerRevisionSet(&command.OriginalDependencies)
		value.ApplicabilitySource = runtimecontract.CloneRuntimeEvidenceApplicabilityDependencies(command.ApplicabilitySource)
		switch scenario {
		case "receipt":
			value.SourceReceiptHash = "wrong"
		case "original_reads":
			value.OriginalDependencies.Entries[0].Revision = "forged"
			value.OriginalDependencies = value.OriginalDependencies.WithHash()
		case "original_current":
			value.ApplicabilitySource.OwnerRevisionSet.Entries[0].Revision = sha256Hex(changed)
			value.ApplicabilitySource.OwnerRevisionSet = value.ApplicabilitySource.OwnerRevisionSet.WithHash()
		case "protocol":
			value.ApplicabilitySource.ProtocolRef += " "
		case "resource":
			value.ApplicabilitySource.OwnerRevisionSet.Entries[0].ResourceID = "unrelated.txt"
			value.ApplicabilitySource.OwnerRevisionSet = value.ApplicabilitySource.OwnerRevisionSet.WithHash()
		}
		if _, err := send(value); err == nil {
			t.Fatalf("forged original source accepted: %s", scenario)
		}
	}
	if queries == 0 {
		t.Fatal("owner did not reverse-prove original receipt")
	}
	if err := os.Remove(filepath.Join(directory, "target.txt")); err != nil {
		t.Fatal(err)
	}
	missing, err := send(command)
	if err != nil {
		t.Fatal(err)
	}
	if set, found, err := fileevents.ValidateRuntimeFileReadDependenciesResult(command, missing); err != nil || found || set != nil {
		t.Fatalf("missing target supplied current proof: %v", err)
	}
	if err := os.Mkdir(filepath.Join(directory, "target.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := send(command); err == nil {
		t.Fatal("read error was disguised as missing resource")
	}
	if !reflect.DeepEqual(*persisted, original) {
		t.Fatal("failure changed immutable receipt")
	}
	owner.BeginShutdown(ctx)
	if _, err := send(command); err == nil {
		t.Fatal("query crossed owner shutdown admission")
	}
}
