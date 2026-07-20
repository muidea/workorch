package shell

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellOwnerStateChangingReceiptReportsFileResourcesAndReplays(t *testing.T) {
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

	owner := &Biz{Base: basebiz.New(modulecommon.ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	workspace := t.TempDir()
	command := shellStateChangingReceiptCommand(workspace, "printf first > generated.txt")
	response := sendShellStateChangingReceiptCommand(t, ctx, hub, command)
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !response.Receipt.Changed ||
		response.Receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		t.Fatalf("first receipt = %+v", response.Receipt)
	}
	var targetChange *runtimecontract.RuntimeResourceChange
	for idx := range response.Receipt.ResourceChanges {
		change := response.Receipt.ResourceChanges[idx]
		if change.Domain == runtimecontract.RuntimeResourceWorkspaceTarget && change.SubjectRef == "generated.txt" {
			targetChange = &change
			break
		}
	}
	if targetChange == nil || targetChange.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		targetChange.BeforeRevision != "missing" || targetChange.AfterRevision == "" || targetChange.AfterRevision == "missing" {
		t.Fatalf("shell target resource change = %+v", response.Receipt.ResourceChanges)
	}
	if content, err := os.ReadFile(filepath.Join(workspace, "generated.txt")); err != nil || string(content) != "first" {
		t.Fatalf("first owner effect content=%q err=%v", string(content), err)
	}

	// The same execution key must return the durable receipt instead of running
	// a second Shell effect, even if a duplicate delivery carries different data.
	command.Data.Args = []string{"-c", "printf replayed > generated.txt"}
	replayed := sendShellStateChangingReceiptCommand(t, ctx, hub, command)
	if replayed.Receipt.ExecutionKey != response.Receipt.ExecutionKey || replayed.Receipt.AfterHash != response.Receipt.AfterHash {
		t.Fatalf("replayed receipt = %+v, want persisted %+v", replayed.Receipt, response.Receipt)
	}
	if content, err := os.ReadFile(filepath.Join(workspace, "generated.txt")); err != nil || string(content) != "first" {
		t.Fatalf("duplicate receipt replay executed shell command: content=%q err=%v", string(content), err)
	}
}

func shellStateChangingReceiptCommand(workspace, script string) orchestratorevents.ExecuteShellOperationCommand {
	return orchestratorevents.ExecuteShellOperationCommand{
		Execution: orchestratorevents.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/shell-write", Fence: 1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		},
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
		OperationID: "shell-write", EvidenceKind: runtimecontract.EvidenceKindStateChange,
		WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{"generated.txt"},
		Data: orchestratorevents.ShellOperationData{
			Command: "sh", Args: []string{"-c", script}, StateChanging: true,
			TargetRef: "generated.txt", TargetRefs: []string{"generated.txt"},
		},
	}
}

func sendShellStateChangingReceiptCommand(t *testing.T, ctx context.Context, hub magicevent.Hub, command orchestratorevents.ExecuteShellOperationCommand) orchestratorevents.ExecuteShellOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteShellOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.ExecuteShellOperationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	return response
}
