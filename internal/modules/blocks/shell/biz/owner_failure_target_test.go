package shell

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
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
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
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
	if err := os.WriteFile(filepath.Join(workspace, "validation-entry.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "failing-entry-validation", Command: "sh", Args: []string{"-c", "echo assertion failed >&2; exit 7"}}
	command := shellevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/validate-entry", Fence: 1,
				ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession,
			},
			WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
			OperationID: "validate-entry", EvidenceKind: runtimecontract.EvidenceKindValidation,
			WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{"validation-entry.go"},
		},
		TargetRole:                "implementation",
		ValidationRecipes:         []runtimecontract.RuntimeValidationRecipe{recipe},
		ValidationScopeTargetRefs: []string{"validation-entry.go"},
		Data: shellevents.OperationData{ValidationRecipeID: recipe.ID, TargetRefs: []string{"validation-entry.go"}, ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		}},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	response, ok := data.(shellevents.ExecuteOperationResult)
	if !ok {
		t.Fatalf("shell owner result type = %T", data)
	}
	diagnostic := response.Receipt.Feedback.OriginalDiagnostic
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || diagnostic == nil ||
		len(diagnostic.TargetRefs) != 0 || diagnostic.TargetObservation != nil {
		t.Fatalf("shell owner failure = %+v", response)
	}
	if diagnostic.ValidationDiagnostic == nil || diagnostic.ValidationDiagnostic.Class != runtimecontract.ValidationDiagnosticBuildFailed {
		t.Fatalf("shell owner failure lost validation diagnostic: %+v", diagnostic)
	}
	if diagnostic.ValidationFailureObservation != nil {
		t.Fatalf("shell owner promoted the validation entry into a failure candidate: %+v", diagnostic)
	}
	if !reflect.DeepEqual(response.Receipt.TargetRefs, []string{"validation-entry.go"}) || response.Receipt.Feedback == nil ||
		len(diagnostic.TargetRefs) != 0 || diagnostic.TargetObservation != nil {
		t.Fatalf("shell owner receipt = %+v", response.Receipt)
	}
	if diagnostic.ValidationDiagnostic == nil || diagnostic.ValidationDiagnostic.Class != runtimecontract.ValidationDiagnosticBuildFailed {
		t.Fatalf("shell owner receipt lost validation diagnostic: %+v", diagnostic)
	}
	if diagnostic.ValidationFailureObservation != nil {
		t.Fatalf("shell owner receipt promoted unstructured output: %+v", diagnostic)
	}
	if response.Receipt.Feedback == nil || response.Receipt.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainOperation ||
		response.Receipt.Feedback.WorkspaceID != command.WorkspaceID || response.Receipt.Feedback.RunID != command.RunID ||
		response.Receipt.Feedback.AuthoritySnapshotID != command.Transaction.AuthoritySnapshotID || response.Receipt.Feedback.DispatchID != command.DispatchID ||
		response.Receipt.Feedback.OriginalDiagnostic == nil || response.Receipt.Feedback.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleShell ||
		response.Receipt.Feedback.OriginalDiagnostic.OperationID != command.OperationID || response.Receipt.Feedback.OriginalDiagnostic.OriginalCode != diagnostic.OriginalCode ||
		response.Receipt.Feedback.OriginalDiagnostic.OriginalMessage != diagnostic.OriginalMessage {
		t.Fatalf("shell owner receipt lost scoped original feedback: %+v", response.Receipt.Feedback)
	}
	if receipt, found := receipts[command.Execution.ExecutionKey]; !found || !runtimecontract.SameRuntimeOperationReceipt(receipt, response.Receipt) {
		t.Fatalf("committed receipt = %+v, response receipt = %+v", receipt, response.Receipt)
	}

	// The owner must reuse the committed receipt for a duplicate delivery. In
	// particular, replay cannot turn the validation entry into a failure target
	// through a second adapter execution or receipt reconstruction.
	duplicateResult := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	duplicateData, duplicateErr := duplicateResult.Get()
	if duplicateErr != nil {
		t.Fatal(duplicateErr)
	}
	duplicate, ok := duplicateData.(shellevents.ExecuteOperationResult)
	duplicateDiagnostic := duplicate.Receipt.Feedback.OriginalDiagnostic
	if !ok || commits != 1 || !runtimecontract.SameRuntimeOperationReceipt(duplicate.Receipt, response.Receipt) || duplicateDiagnostic == nil ||
		len(duplicateDiagnostic.TargetRefs) != 0 || duplicateDiagnostic.TargetObservation != nil {
		t.Fatalf("duplicate shell validation delivery = %+v commits=%d", duplicate, commits)
	}
	if duplicateDiagnostic.ValidationDiagnostic == nil || duplicateDiagnostic.ValidationDiagnostic.Class != runtimecontract.ValidationDiagnosticBuildFailed {
		t.Fatalf("duplicate shell validation lost persisted diagnostic: %+v", duplicateDiagnostic)
	}
}

func TestShellValidationFailureObservationOnlyExportsImplementationEffectCategories(t *testing.T) {
	if !shellValidationDiagnosticMayIdentifyImplementationEffect(runtimecontract.ValidationDiagnosticBuildFailed) ||
		!shellValidationDiagnosticMayIdentifyImplementationEffect(runtimecontract.ValidationDiagnosticTestFailed) ||
		shellValidationDiagnosticMayIdentifyImplementationEffect(runtimecontract.ValidationDiagnosticEnvironmentMissing) ||
		shellValidationDiagnosticMayIdentifyImplementationEffect(runtimecontract.ValidationDiagnosticCommandInvalid) {
		t.Fatal("Shell validation category classification permits non-implementation failures to create repair candidates")
	}
}

func TestShellManagedGoDiagnosticReportsActualManifestScopedFailureTarget(t *testing.T) {
	request := RuntimeCommandRequest{
		ManagedValidation:         runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationScopeTargetRefs: []string{"cmd/lsp-bridge/main.go", "internal/lspbridge/validation_surface_test.go"},
		TargetRefs:                []string{"internal/lspbridge/validation_surface_test.go"},
	}
	output := RuntimeCommandResult{
		WorkingDirectory: "/tmp/workorch-shell-validation/workspace",
		Stderr:           "cmd/lsp-bridge/main.go:178:23: undefined: context\ncmd/lsp-bridge/main.go:952:5: undefined: appendVal\n",
	}
	targets := shellValidationDiagnosticTargetRefs(output, request, "go")
	if !reflect.DeepEqual(targets, []string{"cmd/lsp-bridge/main.go"}) {
		t.Fatalf("managed Go failure targets = %v", targets)
	}
	command := shellevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell"}, OperationID: "validate-entry",
		},
		TargetRole: "implementation", ValidationScopeTargetRefs: request.ValidationScopeTargetRefs,
	}
	diagnostic := runtimecontract.ValidationFailureDiagnostic{
		Class: runtimecontract.ValidationDiagnosticBuildFailed, Stage: "shell_command_failed", Operation: "shell/command", TargetRefs: targets,
	}
	observation := shellValidationFailureObservation(command, diagnostic)
	if observation == nil || !reflect.DeepEqual(observation.CandidateTargetRefs, []string{"cmd/lsp-bridge/main.go"}) ||
		!reflect.DeepEqual(observation.ValidationScopeTargetRefs, request.ValidationScopeTargetRefs) {
		t.Fatalf("managed Go validation observation = %+v", observation)
	}
	if reflect.DeepEqual(observation.CandidateTargetRefs, request.TargetRefs) {
		t.Fatalf("validation entry became failure target: %+v", observation)
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
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
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
	recipe := runtimecontract.RuntimeValidationRecipe{
		ID:      "structured-failure-target",
		Command: "sh",
		Args:    []string{"-c", "printf '%s\\n' 'WORKORCH_FAILURE_TARGET={\"schemaVersion\":\"workorch.shell.failure_target\",\"targetRole\":\"implementation\",\"targetRefs\":[\"implementation.go\"],\"diagnosticClass\":\"validation_build_failed\"}'; exit 7"},
	}
	command := shellevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution:   runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/structured", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession},
			WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
			OperationID: "structured", EvidenceKind: runtimecontract.EvidenceKindValidation, WorkspaceDeliverableDirectory: t.TempDir(),
		},
		TargetRole:        "implementation",
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
		Data: shellevents.OperationData{
			ValidationRecipeID: recipe.ID,
			ValidationEvidence: runtimecontract.RuntimeValidationEvidence{DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed},
			ManagedValidation:  runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
		},
	}
	result := hub.Send(magicevent.NewEventWithContext(shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(shellevents.ExecuteOperationResult)
	diagnostic := response.Receipt.Feedback.OriginalDiagnostic
	if !ok || diagnostic == nil || diagnostic.TargetObservation == nil {
		t.Fatalf("structured shell failure = %#v", data)
	}
	observation := diagnostic.TargetObservation
	if observation.OwnerModuleID != ModuleID || observation.SubjectRef != shellFailureTargetObservationSubject || observation.TargetRole != "implementation" ||
		len(observation.TargetRefs) != 1 || observation.TargetRefs[0] != "implementation.go" || observation.DiagnosticClass != runtimecontract.ValidationDiagnosticBuildFailed ||
		len(observation.EvidenceRefs) != 1 || len(observation.ContentHash) != 64 {
		t.Fatalf("structured failure observation = %+v", observation)
	}
	if !reflect.DeepEqual(diagnostic.TargetRefs, observation.TargetRefs) || !reflect.DeepEqual(response.Receipt.Feedback.OriginalDiagnostic.TargetObservation, observation) {
		t.Fatalf("structured receipt lost target binding: response=%+v receipt=%+v", diagnostic, response.Receipt)
	}
}

func TestShellFailureTargetObservationRejectsUnstructuredOrMismatchedRecords(t *testing.T) {
	base := RuntimeCommandResult{
		ValidationFailureDiagnostic: &runtimecontract.ValidationFailureDiagnostic{Class: runtimecontract.ValidationDiagnosticBuildFailed},
	}
	valid := `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_build_failed"}`
	for name, test := range map[string]struct {
		stdout string
		role   string
		want   bool
	}{
		"valid":               {stdout: valid, role: "implementation", want: true},
		"missing":             {stdout: "implementation.go failed", role: "implementation"},
		"multiple":            {stdout: valid + "\n" + valid, role: "implementation"},
		"unknown_field":       {stdout: `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_build_failed","extra":true}`, role: "implementation"},
		"role_mismatch":       {stdout: valid, role: "validation"},
		"diagnostic_mismatch": {stdout: `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_test_failed"}`, role: "implementation"},
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
