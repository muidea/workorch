package shell

import (
	"context"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Shell has no structured related-target protocol. A failing validation
// command must preserve its execution target as receipt lineage while leaving
// failure-target authority empty; the Orchestrator can then acquire, govern or
// terminate rather than treating a test entry as an implementation target.
func TestShellOwnerDoesNotInferFailureTargetFromValidationCommand(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	commits := 0
	receiptOwner := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Receipt: &receipt, Found: found}, nil)
	})
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		commits++
		receipts[command.Receipt.ExecutionKey] = command.Receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})

	owner := &Biz{Base: basebiz.New(modulecommon.ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	workspace := t.TempDir()
	command := orchestratorevents.ExecuteShellOperationCommand{
		Execution: orchestratorevents.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/validate-entry", Fence: 1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
		},
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
		OperationID: "validate-entry", EvidenceKind: runtimecontract.EvidenceKindValidation,
		WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{"validation-entry.go"},
		Data: orchestratorevents.ShellOperationData{Command: "sh", Args: []string{"-c", "echo assertion failed >&2; exit 7"}, TargetRefs: []string{"validation-entry.go"}},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteShellOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	response, ok := data.(orchestratorevents.ExecuteShellOperationResult)
	if !ok {
		t.Fatalf("shell owner result type = %T", data)
	}
	if response.Status != runtimecontract.RuntimeOperationExecutionStatusRejected || response.Failure == nil ||
		len(response.Failure.TargetRefs) != 0 || response.Failure.TargetObservation != nil {
		t.Fatalf("shell owner failure = %+v", response)
	}
	if !reflect.DeepEqual(response.Receipt.TargetRefs, []string{"validation-entry.go"}) || response.Receipt.Failure == nil ||
		len(response.Receipt.Failure.TargetRefs) != 0 || response.Receipt.Failure.TargetObservation != nil {
		t.Fatalf("shell owner receipt = %+v", response.Receipt)
	}
	if receipt, found := receipts[command.Execution.ExecutionKey]; !found || !reflect.DeepEqual(receipt, response.Receipt) {
		t.Fatalf("committed receipt = %+v, response receipt = %+v", receipt, response.Receipt)
	}

	// The owner must reuse the committed receipt for a duplicate delivery. In
	// particular, replay cannot turn the validation entry into a failure target
	// through a second adapter execution or receipt reconstruction.
	duplicateResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteShellOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	duplicateData, duplicateErr := duplicateResult.Get()
	if duplicateErr != nil {
		t.Fatal(duplicateErr)
	}
	duplicate, ok := duplicateData.(orchestratorevents.ExecuteShellOperationResult)
	if !ok || commits != 1 || !reflect.DeepEqual(duplicate.Receipt, response.Receipt) || duplicate.Failure == nil ||
		len(duplicate.Failure.TargetRefs) != 0 || duplicate.Failure.TargetObservation != nil ||
		duplicate.Receipt.Failure == nil || len(duplicate.Receipt.Failure.TargetRefs) != 0 || duplicate.Receipt.Failure.TargetObservation != nil {
		t.Fatalf("duplicate shell validation delivery = %+v commits=%d", duplicate, commits)
	}
}
