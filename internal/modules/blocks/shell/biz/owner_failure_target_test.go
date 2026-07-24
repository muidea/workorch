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
	if response.Failure.ValidationDiagnostic == nil || response.Failure.ValidationDiagnostic.Class != runtimecontract.ValidationDiagnosticBuildFailed {
		t.Fatalf("shell owner failure lost validation diagnostic: %+v", response.Failure)
	}
	if !reflect.DeepEqual(response.Receipt.TargetRefs, []string{"validation-entry.go"}) || response.Receipt.Failure == nil ||
		len(response.Receipt.Failure.TargetRefs) != 0 || response.Receipt.Failure.TargetObservation != nil {
		t.Fatalf("shell owner receipt = %+v", response.Receipt)
	}
	if response.Receipt.Failure.ValidationDiagnostic == nil || response.Receipt.Failure.ValidationDiagnostic.Class != runtimecontract.ValidationDiagnosticBuildFailed {
		t.Fatalf("shell owner receipt lost validation diagnostic: %+v", response.Receipt.Failure)
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
	if duplicate.Failure.ValidationDiagnostic == nil || duplicate.Failure.ValidationDiagnostic.Class != runtimecontract.ValidationDiagnosticBuildFailed {
		t.Fatalf("duplicate shell validation lost persisted diagnostic: %+v", duplicate.Failure)
	}
}

func TestShellOwnerEmitsOnlyContentBoundStructuredFailureTarget(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	receiptOwner := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Receipt: &receipt, Found: found}, nil)
	})
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
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
	command := orchestratorevents.ExecuteShellOperationCommand{
		Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/structured", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone},
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
		OperationID: "structured", EvidenceKind: runtimecontract.EvidenceKindValidation, TargetRole: "implementation", WorkspaceDeliverableDirectory: t.TempDir(),
		Data: orchestratorevents.ShellOperationData{Command: "sh", Args: []string{"-c", "printf '%s\\n' 'WORKORCH_FAILURE_TARGET_V1={\"schemaVersion\":\"workorch.shell.failure_target.v1\",\"targetRole\":\"implementation\",\"targetRefs\":[\"implementation.go\"],\"diagnosticClass\":\"validation_build_failed\"}'; exit 7"}, ValidationEvidence: runtimecontract.RuntimeValidationEvidence{DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed}},
	}
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteShellOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.ExecuteShellOperationResult)
	if !ok || response.Failure == nil || response.Failure.TargetObservation == nil {
		t.Fatalf("structured shell failure = %#v", data)
	}
	observation := response.Failure.TargetObservation
	if observation.OwnerModuleID != ModuleID || observation.SubjectRef != shellFailureTargetObservationSubject || observation.TargetRole != "implementation" ||
		len(observation.TargetRefs) != 1 || observation.TargetRefs[0] != "implementation.go" || observation.DiagnosticClass != runtimecontract.ValidationDiagnosticBuildFailed ||
		len(observation.EvidenceRefs) != 1 || len(observation.ContentHash) != 64 {
		t.Fatalf("structured failure observation = %+v", observation)
	}
	if !reflect.DeepEqual(response.Failure.TargetRefs, observation.TargetRefs) || !reflect.DeepEqual(response.Receipt.Failure.TargetObservation, observation) {
		t.Fatalf("structured receipt lost target binding: response=%+v receipt=%+v", response.Failure, response.Receipt)
	}
}

func TestShellFailureTargetObservationRejectsUnstructuredOrMismatchedRecords(t *testing.T) {
	base := RuntimeCommandResult{
		ValidationFailureDiagnostic: &runtimecontract.ValidationFailureDiagnostic{Class: runtimecontract.ValidationDiagnosticBuildFailed},
	}
	valid := `WORKORCH_FAILURE_TARGET_V1={"schemaVersion":"workorch.shell.failure_target.v1","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_build_failed"}`
	for name, test := range map[string]struct {
		stdout string
		role   string
		want   bool
	}{
		"valid":               {stdout: valid, role: "implementation", want: true},
		"missing":             {stdout: "implementation.go failed", role: "implementation"},
		"multiple":            {stdout: valid + "\n" + valid, role: "implementation"},
		"unknown_field":       {stdout: `WORKORCH_FAILURE_TARGET_V1={"schemaVersion":"workorch.shell.failure_target.v1","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_build_failed","extra":true}`, role: "implementation"},
		"role_mismatch":       {stdout: valid, role: "validation"},
		"diagnostic_mismatch": {stdout: `WORKORCH_FAILURE_TARGET_V1={"schemaVersion":"workorch.shell.failure_target.v1","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_test_failed"}`, role: "implementation"},
	} {
		t.Run(name, func(t *testing.T) {
			result := base
			result.Stdout = test.stdout
			got := shellFailureTargetObservation(result, test.role, "shell-policy-v1")
			if (got != nil) != test.want {
				t.Fatalf("shellFailureTargetObservation() = %+v, want present=%t", got, test.want)
			}
		})
	}
}
