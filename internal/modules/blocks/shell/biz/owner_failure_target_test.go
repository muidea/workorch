package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	executionContext := runtimecontract.RuntimeOperationExecutionContext{
		Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/validate-entry", Fence: 1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession,
		},
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
		OperationID: "validate-entry", EvidenceKind: runtimecontract.EvidenceKindValidation,
		WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{"validation-entry.go"},
	}
	command := sealedShellTestCommand(t, owner.runtimeAdapter, executionContext, runtimecontract.RuntimeBlockKindManagedValidationSession, shellevents.OperationData{
		ValidationRecipeID: recipe.ID, TargetRefs: []string{"validation-entry.go"}, ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
	}, "implementation", []runtimecontract.RuntimeValidationRecipe{recipe}, []string{"validation-entry.go"})
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
	executionContext := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell"}, OperationID: "validate-entry",
	}
	authorization := shellevents.RuntimeExecutionAuthorization{TargetRole: "implementation", ValidationScopeTargetRefs: request.ValidationScopeTargetRefs}
	diagnostic := runtimecontract.ValidationFailureDiagnostic{
		SchemaVersion: runtimecontract.ValidationFailureDiagnosticSchemaVersion,
		Class:         runtimecontract.ValidationDiagnosticBuildFailed, OwnerCode: "shell_exit_nonzero",
		RecommendedEvidenceTargetType: runtimecontract.ValidationEvidenceTargetFailureSurface,
		Stage:                         "shell_command_failed", Operation: runtimecontract.RuntimeBlockKindManagedValidationSession, TargetRefs: targets,
	}
	observation := shellValidationFailureObservation(executionContext, authorization, diagnostic)
	if observation == nil || !reflect.DeepEqual(observation.CandidateTargetRefs, []string{"cmd/lsp-bridge/main.go"}) ||
		!reflect.DeepEqual(observation.ValidationScopeTargetRefs, request.ValidationScopeTargetRefs) {
		t.Fatalf("managed Go validation observation = %+v", observation)
	}
	if reflect.DeepEqual(observation.CandidateTargetRefs, request.TargetRefs) {
		t.Fatalf("validation entry became failure target: %+v", observation)
	}
	drifted := diagnostic.Clone()
	drifted.OwnerCode = "different_owner_code"
	ownerCodeObservation := shellValidationFailureObservation(executionContext, authorization, drifted)
	if ownerCodeObservation == nil || ownerCodeObservation.DiagnosticHash == observation.DiagnosticHash {
		t.Fatal("validation observation hash ignored exact repair-route owner code")
	}
	drifted = diagnostic.Clone()
	drifted.RecommendedEvidenceTargetType = runtimecontract.ValidationEvidenceTargetBehaviorEvidence
	targetTypeObservation := shellValidationFailureObservation(executionContext, authorization, drifted)
	if targetTypeObservation == nil || targetTypeObservation.DiagnosticHash == observation.DiagnosticHash {
		t.Fatal("validation observation hash ignored exact repair-route evidence target type")
	}
}

func TestShellManagedGoPackageConflictReportsBothManifestScopedTargets(t *testing.T) {
	workingDirectory := "/tmp/workorch-shell-validation/workspace"
	request := RuntimeCommandRequest{
		ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationScopeTargetRefs: []string{
			"main.go",
			"semantic_test.go",
			"unrelated.go",
		},
	}
	output := RuntimeCommandResult{
		WorkingDirectory: workingDirectory,
		Stderr:           "found packages main (main.go) and semantic (semantic_test.go) in " + workingDirectory + "\n",
	}
	if targets := shellValidationDiagnosticTargetRefs(output, request, "go"); !reflect.DeepEqual(targets, []string{"main.go", "semantic_test.go"}) {
		t.Fatalf("managed Go package-conflict targets = %v", targets)
	}

	output.Stderr = "found packages main (main.go) and semantic (outside_test.go) in " + workingDirectory + "\n"
	if targets := shellValidationDiagnosticTargetRefs(output, request, "go"); !reflect.DeepEqual(targets, []string{"main.go"}) {
		t.Fatalf("package-conflict target escaped manifest scope: %v", targets)
	}

	output.WorkingDirectory = ""
	if targets := shellValidationDiagnosticTargetRefs(output, request, "go"); len(targets) != 0 {
		t.Fatalf("absolute package-conflict targets were accepted without a working-directory boundary: %v", targets)
	}
}

func TestShellManagedGoModuleDiagnosticReportsOnlyManifestScopedModuleTarget(t *testing.T) {
	request := RuntimeCommandRequest{
		ManagedValidation:         runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationScopeTargetRefs: []string{"go.mod", "main.go"},
	}
	output := RuntimeCommandResult{
		WorkingDirectory: "/tmp/workorch-shell-validation/workspace",
		Stderr:           "go: errors parsing go.mod:\ngo.mod:3: unknown directive: package\noutside/go.mod:1: unknown directive: invalid",
	}
	if targets := shellValidationDiagnosticTargetRefs(output, request, "go"); !reflect.DeepEqual(targets, []string{"go.mod"}) {
		t.Fatalf("managed Go module targets = %v", targets)
	}
}

func TestShellOwnerEmitsOnlySealedScopeValidationFailureObservation(t *testing.T) {
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
	executionContext := runtimecontract.RuntimeOperationExecutionContext{
		Execution:   runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/structured", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession},
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
		OperationID: "structured", EvidenceKind: runtimecontract.EvidenceKindValidation, WorkspaceDeliverableDirectory: t.TempDir(),
	}
	command := sealedShellTestCommand(t, owner.runtimeAdapter, executionContext, runtimecontract.RuntimeBlockKindManagedValidationSession, shellevents.OperationData{
		ValidationRecipeID: recipe.ID,
		ValidationEvidence: runtimecontract.RuntimeValidationEvidence{DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed},
		ManagedValidation:  runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
	}, "implementation", []runtimecontract.RuntimeValidationRecipe{recipe}, []string{"implementation.go"})
	result := hub.Send(magicevent.NewEventWithContext(shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(shellevents.ExecuteOperationResult)
	diagnostic := response.Receipt.Feedback.OriginalDiagnostic
	if !ok || diagnostic == nil || diagnostic.TargetObservation != nil || diagnostic.ValidationFailureObservation == nil {
		t.Fatalf("structured shell failure = %#v", data)
	}
	observation := diagnostic.ValidationFailureObservation
	if observation.TargetRole != "implementation" || len(observation.CandidateTargetRefs) != 1 || observation.CandidateTargetRefs[0] != "implementation.go" ||
		len(observation.ValidationScopeTargetRefs) != 1 || observation.ValidationScopeTargetRefs[0] != "implementation.go" ||
		observation.DiagnosticClass != runtimecontract.ValidationDiagnosticBuildFailed || len(observation.DiagnosticHash) != 64 {
		t.Fatalf("structured failure observation = %+v", observation)
	}
	if len(diagnostic.TargetRefs) != 0 || response.Receipt.Feedback.OriginalDiagnostic.TargetObservation != nil ||
		!reflect.DeepEqual(response.Receipt.Feedback.OriginalDiagnostic.ValidationFailureObservation, observation) {
		t.Fatalf("structured receipt lost target binding: response=%+v receipt=%+v", diagnostic, response.Receipt)
	}
}

func TestShellOwnerFailureTargetMarkerAndParserCandidateUnionFailsClosed(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
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

	overflowMarkerRefs := make([]string, 0, shellValidationDiagnosticMaxTargetRefs+1)
	for index := 0; index <= shellValidationDiagnosticMaxTargetRefs; index++ {
		overflowMarkerRefs = append(overflowMarkerRefs, fmt.Sprintf("marker-%02d.go", index))
	}
	overflowMarker := fmt.Sprintf(`WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":[%s],"diagnosticClass":"validation_build_failed"}`,
		quotedShellTestStrings(overflowMarkerRefs))
	for index, test := range []struct {
		name              string
		marker            string
		markerScope       []string
		wantCandidateRefs []string
	}{
		{name: "marker absent keeps parser candidate", wantCandidateRefs: []string{"parser.go"}},
		{
			name:        "valid marker unions with parser candidate",
			marker:      `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["marker.go"],"diagnosticClass":"validation_build_failed"}`,
			markerScope: []string{"marker.go"}, wantCandidateRefs: []string{"marker.go", "parser.go"},
		},
		{name: "malformed marker invalidates parser candidate", marker: `WORKORCH_FAILURE_TARGET={not-json`},
		{name: "out of scope marker invalidates parser candidate", marker: `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["outside.go"],"diagnosticClass":"validation_build_failed"}`},
		{name: "over bound marker invalidates parser candidate", marker: overflowMarker, markerScope: overflowMarkerRefs},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			source := fmt.Sprintf(`package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(%q)
	fmt.Fprintln(os.Stderr, "parser.go:1:1: synthetic managed-validation diagnostic")
	os.Exit(7)
}
`, test.marker)
			for name, content := range map[string]string{
				"main.go":   source,
				"parser.go": "package main\n",
			} {
				if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, targetRef := range test.markerScope {
				if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte("package main\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			recipe := runtimecontract.RuntimeValidationRecipe{
				ID: fmt.Sprintf("marker-parser-%d", index), Command: "go", Args: []string{"run", "main.go"}, TimeoutMillis: 60_000,
			}
			operationID := fmt.Sprintf("marker-parser-%d", index)
			dispatchID := "dispatch-shell-" + operationID
			executionContext := runtimecontract.RuntimeOperationExecutionContext{
				Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
					ExecutionKey: fmt.Sprintf("exec/ws-shell/run-shell/%s/%s", dispatchID, operationID), Fence: 1,
					ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession,
				},
				WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: dispatchID,
				Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
				OperationID: operationID, EvidenceKind: runtimecontract.EvidenceKindValidation,
				WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{"main.go"},
			}
			validationScope := append([]string{"main.go", "parser.go"}, test.markerScope...)
			command := sealedShellTestCommand(t, owner.runtimeAdapter, executionContext, runtimecontract.RuntimeBlockKindManagedValidationSession, shellevents.OperationData{
				ValidationRecipeID: recipe.ID,
				ValidationEvidence: runtimecontract.RuntimeValidationEvidence{DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed},
				ManagedValidation:  runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
			}, "implementation", []runtimecontract.RuntimeValidationRecipe{recipe}, validationScope)
			result := hub.Send(magicevent.NewEventWithContext(
				shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
				magicevent.NewHeader(), ctx, command,
			))
			data, resultErr := result.Get()
			if resultErr != nil {
				t.Fatal(resultErr)
			}
			response, ok := data.(shellevents.ExecuteOperationResult)
			if !ok || response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || response.Receipt.Feedback == nil ||
				response.Receipt.Feedback.OriginalDiagnostic == nil {
				t.Fatalf("Shell owner rejected receipt = %#v", data)
			}
			diagnostic := response.Receipt.Feedback.OriginalDiagnostic
			if diagnostic.TargetObservation != nil || len(diagnostic.TargetRefs) != 0 || diagnostic.ValidationDiagnostic == nil {
				t.Fatalf("Shell owner leaked direct target authority: %+v", diagnostic)
			}
			observation := diagnostic.ValidationFailureObservation
			if len(test.wantCandidateRefs) == 0 {
				if observation != nil || len(diagnostic.ValidationDiagnostic.TargetRefs) != 0 {
					t.Fatalf("invalid marker fell back to parser candidate: diagnostic=%+v observation=%+v", diagnostic.ValidationDiagnostic, observation)
				}
			} else if observation == nil || !reflect.DeepEqual(observation.CandidateTargetRefs, test.wantCandidateRefs) ||
				!reflect.DeepEqual(diagnostic.ValidationDiagnostic.TargetRefs, test.wantCandidateRefs) ||
				!reflect.DeepEqual(observation.ValidationScopeTargetRefs, canonicalShellFailureTargetRefs(validationScope)) {
				t.Fatalf("bounded marker/parser candidate union = diagnostic=%+v observation=%+v", diagnostic.ValidationDiagnostic, observation)
			}

			duplicateResult := hub.Send(magicevent.NewEventWithContext(
				shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
				magicevent.NewHeader(), ctx, command,
			))
			duplicateData, duplicateErr := duplicateResult.Get()
			if duplicateErr != nil {
				t.Fatal(duplicateErr)
			}
			duplicate, ok := duplicateData.(shellevents.ExecuteOperationResult)
			if !ok || !runtimecontract.SameRuntimeOperationReceipt(duplicate.Receipt, response.Receipt) {
				t.Fatalf("duplicate marker/parser receipt drifted: first=%+v duplicate=%+v", response.Receipt, duplicate)
			}
		})
	}
}

func quotedShellTestStrings(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ",")
}

func TestShellOwnerSelectorOnlyMarkerDoesNotCreateRepairAuthority(t *testing.T) {
	result := RuntimeCommandResult{
		Stdout:                      `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_build_failed"}`,
		ValidationFailureDiagnostic: &runtimecontract.ValidationFailureDiagnostic{Class: runtimecontract.ValidationDiagnosticBuildFailed},
	}
	executionContext := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell"}, OperationID: "validate-entry",
	}
	authorization := shellevents.RuntimeExecutionAuthorization{TargetRole: "implementation"}
	diagnostic := result.ValidationFailureDiagnostic.Clone()
	targetRefs, valid := shellFailureTargetCandidateUnion(result, authorization.TargetRole, authorization.ValidationScopeTargetRefs)
	if valid {
		diagnostic.TargetRefs = targetRefs
	} else {
		diagnostic.TargetRefs = nil
	}
	if observation := shellValidationFailureObservation(executionContext, authorization, diagnostic); valid || observation != nil || len(diagnostic.TargetRefs) != 0 {
		t.Fatalf("selector-only marker created repair authority: diagnostic=%+v observation=%+v", diagnostic, observation)
	}
}

func TestShellFailureTargetMarkerOutsideSealedScopeIsRejected(t *testing.T) {
	result := RuntimeCommandResult{
		Stdout:                      `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["outside.go"],"diagnosticClass":"validation_build_failed"}`,
		ValidationFailureDiagnostic: &runtimecontract.ValidationFailureDiagnostic{Class: runtimecontract.ValidationDiagnosticBuildFailed},
	}
	executionContext := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell"}, OperationID: "validate-entry",
	}
	authorization := shellevents.RuntimeExecutionAuthorization{TargetRole: "implementation", ValidationScopeTargetRefs: []string{"entry.go", "implementation.go"}}
	diagnostic := result.ValidationFailureDiagnostic.Clone()
	marker := decodeShellFailureTargetMarkerCandidate(result, authorization.TargetRole)
	if !marker.Present || !marker.Valid {
		t.Fatal("closed marker fixture was not decoded")
	}
	targetRefs, valid := shellFailureTargetCandidateUnion(result, authorization.TargetRole, authorization.ValidationScopeTargetRefs)
	if valid {
		diagnostic.TargetRefs = targetRefs
	} else {
		diagnostic.TargetRefs = nil
	}
	if observation := shellValidationFailureObservation(executionContext, authorization, diagnostic); valid || observation != nil || len(diagnostic.TargetRefs) != 0 {
		t.Fatalf("out-of-scope marker created a repair observation: diagnostic=%+v observation=%+v", diagnostic, observation)
	}
}

func TestShellFailureTargetMarkerRejectsUnstructuredOrMismatchedRecords(t *testing.T) {
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
			got := decodeShellFailureTargetMarkerCandidate(result, test.role)
			if got.Valid != test.want || (name == "missing" && got.Present) || (name != "missing" && !got.Present) {
				t.Fatalf("decodeShellFailureTargetMarkerCandidate() = %+v, want present=%t", got, test.want)
			}
		})
	}
}

func TestShellFailureCandidateMergeRejectsCombinedBoundOverflow(t *testing.T) {
	diagnosticRefs := make([]string, 0, shellValidationDiagnosticMaxTargetRefs)
	markerRefs := make([]string, 0, shellValidationDiagnosticMaxTargetRefs)
	for index := 0; index < shellValidationDiagnosticMaxTargetRefs; index++ {
		diagnosticRefs = append(diagnosticRefs, fmt.Sprintf("diagnostic-%d.go", index))
		markerRefs = append(markerRefs, fmt.Sprintf("marker-%d.go", index))
	}
	for _, test := range []struct {
		name   string
		marker []string
	}{
		{name: "eight plus one", marker: markerRefs[:1]},
		{name: "eight plus eight", marker: markerRefs},
	} {
		t.Run(test.name, func(t *testing.T) {
			if refs, valid := shellMergeFailureTargetCandidateRefs(diagnosticRefs, test.marker); valid || len(refs) != 0 {
				t.Fatalf("overflow candidate union was partially admitted: %v", refs)
			}
		})
	}
	if refs, valid := shellMergeFailureTargetCandidateRefs(diagnosticRefs, diagnosticRefs); !valid || !reflect.DeepEqual(refs, diagnosticRefs) {
		t.Fatalf("deduplicated bounded candidate union = %v, want %v", refs, diagnosticRefs)
	}
}

func TestShellFailureCandidateUnionRejectsParserOverflowEvenWithValidMarker(t *testing.T) {
	scope := make([]string, 0, shellValidationDiagnosticMaxTargetRefs+1)
	var stderr strings.Builder
	for index := 0; index <= shellValidationDiagnosticMaxTargetRefs; index++ {
		ref := fmt.Sprintf("candidate-%d.go", index)
		scope = append(scope, ref)
		fmt.Fprintf(&stderr, "%s:1:1: compile failure\n", ref)
	}
	request := RuntimeCommandRequest{
		ManagedValidation:         runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationScopeTargetRefs: scope,
	}
	result := RuntimeCommandResult{
		ExitCode: 1,
		Stderr:   stderr.String(),
		Stdout:   `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["candidate-0.go"],"diagnosticClass":"validation_build_failed"}`,
	}
	attachShellValidationFailureDiagnostic(&result, request, "go", 1, false, false, fmt.Errorf("exit status 1"))
	if !result.validationFailureTargetCandidatesInvalid || result.ValidationFailureDiagnostic == nil {
		t.Fatalf("parser overflow was not retained as a fail-closed candidate transaction: %+v", result)
	}
	if refs, valid := shellFailureTargetCandidateUnion(result, "implementation", scope); valid || len(refs) != 0 {
		t.Fatalf("valid marker revived over-bounded parser candidates: refs=%v valid=%t", refs, valid)
	}
}
