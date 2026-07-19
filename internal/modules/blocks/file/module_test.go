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
	if response.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || response.Evidence.ContentHash == "" || len(response.Evidence.EvidenceRefs) != 1 {
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
	if response.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || response.Evidence.ContentHash == "" {
		t.Fatalf("file/write response = %+v", response)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "durable\n" {
		t.Fatalf("file/write content = %q err=%v", string(content), err)
	}
}

func TestFileModuleExecutesConcreteInspectionAndPreviewEvents(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
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
	if listResponse.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || len(listResponse.Data.Entries) != 1 || listResponse.Data.Entries[0].Path != "main.txt" {
		t.Fatalf("file/list response = %+v", listResponse)
	}

	readResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteFileReadOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ExecuteFileReadOperationCommand{
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
	if readResponse.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || readResponse.Data.Content != "hello\n" || readResponse.Evidence.ContentHash == "" {
		t.Fatalf("file/read response = %+v", readResponse)
	}

	previewResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecutePatchPreviewOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ExecutePatchPreviewOperationCommand{
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
	if previewResponse.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || previewResponse.Data.WillWrite || previewResponse.Data.AfterHash == "" {
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
	if first.Status != runtimecontract.RuntimeOperationExecutionStatusRejected || first.Failure == nil || second.Failure == nil ||
		first.Failure.Code != "patch_text_replace_conflict" || first.Failure.Signature == "" || first.Failure.Signature != second.Failure.Signature || first.Failure.Message == "" {
		t.Fatalf("stable failures = first:%+v second:%+v", first.Failure, second.Failure)
	}
}

func TestFileModuleExecutesConcretePatchApplyEvent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
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
	if response.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || response.Evidence.ContentHash == "" {
		t.Fatalf("patch/apply response = %+v", response)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "hello\n" {
		t.Fatalf("patch/apply content = %q err=%v", string(content), err)
	}
}
