package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileModuleIdentityConfigureStartStopContract(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(Dependencies{EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestFileModuleStartRequiresEventHub(t *testing.T) {
	module := New()
	if err := module.Configure(Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}

func TestFileModuleExecutesConcreteFileEditEventAndWritesState(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "required-output-1"}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteFileEditOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ExecuteFileEditOperationCommand{
			Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/dispatch-1/edit-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable},
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1", Transaction: transaction,
			OperationID: "edit-1", TargetRefs: []string{"main.txt"}, EvidenceKind: runtimecontract.EvidenceKindStateChange,
			WorkspaceDeliverableDirectory: workspace,
			Data:                          orchestratorevents.FileEditOperationData{Path: "main.txt", OldText: "old", NewText: "new"},
		},
	))
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	response := data.(orchestratorevents.ExecuteFileEditOperationResult)
	if response.Receipt.Evidence.ContentHash == "" || len(response.Receipt.Evidence.EvidenceRefs) != 1 {
		t.Fatalf("file/edit response = %+v", response)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "new\n" {
		t.Fatalf("file/edit content = %q err=%v", string(content), err)
	}
}

func TestFileModuleExecutesConcreteFileWriteEventAndWritesState(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "required-output-1"}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteFileWriteOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ExecuteFileWriteOperationCommand{
			Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/dispatch-1/write-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable},
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1", Transaction: transaction,
			OperationID: "write-1", TargetRefs: []string{"README.md"}, EvidenceKind: runtimecontract.EvidenceKindStateChange,
			WorkspaceDeliverableDirectory: workspace,
			Data:                          orchestratorevents.FileWriteOperationData{TargetRef: "README.md", Content: "durable\n", ContentPresent: true},
		},
	))
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	response := data.(orchestratorevents.ExecuteFileWriteOperationResult)
	if response.Receipt.Evidence.ContentHash == "" {
		t.Fatalf("file/write response = %+v", response)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "durable\n" {
		t.Fatalf("file/write content = %q err=%v", string(content), err)
	}
}

func TestFileOwnerStateChangingReceiptReplaysWithoutSecondWrite(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Receipt: &receipt, Found: found}, nil)
	})
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		if receipt, found := receipts[command.Receipt.ExecutionKey]; found {
			result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt}, nil)
			return
		}
		receipts[command.Receipt.ExecutionKey] = command.Receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
	module := New()
	if err := module.Configure(Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	command := fileWriteReceiptCommand(workspace, "first\n")
	first := sendFileWriteReceiptCommand(t, ctx, hub, command)
	if first.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !first.Receipt.Changed || first.Receipt.AfterHash == "" {
		t.Fatalf("first receipt = %+v", first.Receipt)
	}
	target := filepath.Join(workspace, "README.md")
	if content, err := os.ReadFile(target); err != nil || string(content) != "first\n" {
		t.Fatalf("first file write content=%q err=%v", content, err)
	}
	if err := os.WriteFile(target, []byte("external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command.Data.Content = "replayed\n"
	replayed := sendFileWriteReceiptCommand(t, ctx, hub, command)
	if replayed.Receipt.ExecutionKey != first.Receipt.ExecutionKey || replayed.Receipt.AfterHash != first.Receipt.AfterHash {
		t.Fatalf("replayed receipt=%+v, want persisted=%+v", replayed.Receipt, first.Receipt)
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "external\n" {
		t.Fatalf("duplicate receipt replay wrote target content=%q err=%v", content, err)
	}
}

func fileWriteReceiptCommand(workspace, content string) orchestratorevents.ExecuteFileWriteOperationCommand {
	return orchestratorevents.ExecuteFileWriteOperationCommand{
		Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-file/run-file/dispatch-file/write-readme", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable},
		WorkspaceID: "ws-file", RunID: "run-file", DispatchID: "dispatch-file",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-file", RequiredOutputHash: "output-file"},
		OperationID: "write-readme", TargetRefs: []string{"README.md"}, EvidenceKind: runtimecontract.EvidenceKindStateChange,
		WorkspaceDeliverableDirectory: workspace,
		Data:                          orchestratorevents.FileWriteOperationData{TargetRef: "README.md", Content: content, ContentPresent: true},
	}
}

func sendFileWriteReceiptCommand(t *testing.T, ctx context.Context, hub magicevent.Hub, command orchestratorevents.ExecuteFileWriteOperationCommand) orchestratorevents.ExecuteFileWriteOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteFileWriteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.ExecuteFileWriteOperationResult)
	if !ok {
		t.Fatalf("response type=%T", data)
	}
	return response
}

func TestFileModuleExecutesConcreteInspectionAndPreviewEvents(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "required-output-1"}

	listResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteFileListOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ExecuteFileListOperationCommand{
			Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/dispatch-1/list-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone},
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1", Transaction: transaction,
			OperationID: "list-1", EvidenceKind: runtimecontract.EvidenceKindValidation,
			WorkspaceDeliverableDirectory: workspace, Data: orchestratorevents.FileListOperationData{Path: ".", MaxEntries: 10},
		},
	))
	listData, magicErr := listResult.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	listResponse := listData.(orchestratorevents.ExecuteFileListOperationResult)
	if listResponse.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("file/list response = %+v", listResponse)
	}

	readResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteFileReadOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ExecuteFileReadOperationCommand{
			Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/dispatch-1/read-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone},
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1", Transaction: transaction,
			OperationID: "read-1", TargetRefs: []string{"main.txt"}, EvidenceKind: runtimecontract.EvidenceKindValidation,
			WorkspaceDeliverableDirectory: workspace, Data: orchestratorevents.FileReadOperationData{Path: "main.txt", MaxBytes: 4096},
		},
	))
	readData, magicErr := readResult.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	readResponse := readData.(orchestratorevents.ExecuteFileReadOperationResult)
	if readResponse.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || readResponse.Receipt.Evidence.ContentHash == "" {
		t.Fatalf("file/read response = %+v", readResponse)
	}

	previewResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecutePatchPreviewOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ExecutePatchPreviewOperationCommand{
			Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/dispatch-1/preview-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone},
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1", Transaction: transaction,
			OperationID: "preview-1", TargetRefs: []string{"preview.txt"}, EvidenceKind: runtimecontract.EvidenceKindStateChange,
			WorkspaceDeliverableDirectory: workspace,
			Data:                          orchestratorevents.PatchApplyOperationData{TargetRef: "preview.txt", PatchType: "file_write", Content: "preview\n", ContentPresent: true},
		},
	))
	previewData, magicErr := previewResult.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	previewResponse := previewData.(orchestratorevents.ExecutePatchPreviewOperationResult)
	if previewResponse.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("patch/preview response = %+v", previewResponse)
	}
	if _, err := os.Stat(filepath.Join(workspace, "preview.txt")); !os.IsNotExist(err) {
		t.Fatalf("patch/preview wrote target or stat failed: %v", err)
	}
}

func TestFileModulePatchFailureKeepsStableSignatureAcrossDispatches(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("current\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "required-output-1"}
	execute := func(dispatchID string) orchestratorevents.ExecutePatchApplyOperationResult {
		result := hub.Send(magicevent.NewEventWithContext(
			orchestratorevents.TopicOrchestratorExecutePatchApplyOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
			orchestratorevents.ExecutePatchApplyOperationCommand{
				Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/" + dispatchID + "/patch-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable},
				WorkspaceID: "ws-1", RunID: "run-1", DispatchID: dispatchID, Transaction: transaction,
				OperationID: "patch-1", TargetRefs: []string{"main.txt"}, EvidenceKind: runtimecontract.EvidenceKindStateChange,
				WorkspaceDeliverableDirectory: workspace,
				Data: orchestratorevents.PatchApplyOperationData{
					TargetRef: "main.txt", PatchType: "text_replace", OldText: "missing", NewText: "next", NewTextPresent: true,
				},
			},
		))
		data, magicErr := result.Get()
		if magicErr != nil {
			t.Fatal(magicErr)
		}
		return data.(orchestratorevents.ExecutePatchApplyOperationResult)
	}
	first := execute("dispatch-1")
	second := execute("dispatch-2")
	if first.Receipt.Failure == nil || second.Receipt.Failure == nil ||
		first.Receipt.Failure.Code != "patch_text_replace_conflict" || first.Receipt.Failure.Signature == "" || first.Receipt.Failure.Signature != second.Receipt.Failure.Signature || first.Receipt.Failure.Message == "" {
		t.Fatalf("stable failures = first:%+v second:%+v", first.Receipt.Failure, second.Receipt.Failure)
	}
}

func TestFileModuleExecutesConcretePatchApplyEvent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecutePatchApplyOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ExecutePatchApplyOperationCommand{
			Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/dispatch-1/patch-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable},
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "required-output-1"},
			OperationID: "patch-1", TargetRefs: []string{"README.md"}, EvidenceKind: runtimecontract.EvidenceKindStateChange,
			WorkspaceDeliverableDirectory: workspace,
			Data: orchestratorevents.PatchApplyOperationData{
				TargetRef: "README.md", PatchType: "file_write", Content: "hello\n", ContentPresent: true,
			},
		},
	))
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	response := data.(orchestratorevents.ExecutePatchApplyOperationResult)
	if response.Receipt.Evidence.ContentHash == "" {
		t.Fatalf("patch/apply response = %+v", response)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "hello\n" {
		t.Fatalf("patch/apply content = %q err=%v", string(content), err)
	}
}

func registerFileModuleReceiptOwner(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Receipt: &receipt, Found: found}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		receipts[command.Receipt.ExecutionKey] = command.Receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
}
