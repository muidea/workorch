package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellOwnerZeroWorkValidationReceiptReplaysQualityWithoutReexecution(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
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
	workspace := newGoValidationWorkspace(t, "package fixture\nimport \"testing\"\nfunc TestExisting(t *testing.T) {}\n")
	command := shellevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/validate", Fence: 1,
				ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
			},
			WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
			OperationID: "validate", EvidenceKind: runtimecontract.EvidenceKindValidation,
			WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{"fixture_test.go"},
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
				ID: "zero-work", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "go", ArgsContain: []string{"test"},
				ZeroWorkMarkers: []string{"no tests to run", "[no tests to run]"}, WorkEvidenceMarkers: []string{"=== RUN", "--- PASS:"},
				QualityOnZeroWork: "inconclusive", EvidenceReason: "no matching validation checks", EvidenceTags: []string{"zero_work"},
			}},
		},
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{{
			ID: "go-test-zero-work", Command: "go", Args: []string{"test", "-v", "-run", "TestMissing", "./..."},
		}},
		Data: shellevents.OperationData{
			ValidationRecipeID: "go-test-zero-work",
			TargetRefs:         []string{"fixture_test.go"},
			ManagedValidation: runtimecontract.RuntimeManagedValidation{
				RuntimeManaged: true,
				Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
			},
		},
	}
	first := sendShellStateChangingReceiptCommand(t, ctx, hub, command)
	if first.Receipt.Evidence.Quality != "inconclusive" || !first.Receipt.Evidence.ZeroWork ||
		first.Receipt.Evidence.Validation == nil || first.Receipt.Evidence.EvidenceReason != "no matching validation checks" {
		t.Fatalf("zero-work receipt = %+v", first.Receipt)
	}
	firstHash := runtimecontract.StableRuntimeOperationReceiptHash(first.Receipt)
	command.ValidationRecipes[0].Args = []string{"test", "-v", "-run", "TestExisting", "./..."}
	replayed := sendShellStateChangingReceiptCommand(t, ctx, hub, command)
	if runtimecontract.StableRuntimeOperationReceiptHash(replayed.Receipt) != firstHash ||
		replayed.Receipt.Evidence.Quality != first.Receipt.Evidence.Quality || !replayed.Receipt.Evidence.ZeroWork {
		t.Fatalf("replayed validation receipt changed quality: first=%+v replayed=%+v", first.Receipt, replayed.Receipt)
	}
}

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

func TestShellOwnerRejectsWorkspaceEffectOutsideSealedTargets(t *testing.T) {
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
	command := shellStateChangingReceiptCommand(workspace, "printf declared > generated.txt; printf unexpected > rogue.txt")
	response := sendShellStateChangingReceiptCommand(t, ctx, hub, command)
	diagnostic := response.Receipt.Feedback.OriginalDiagnostic
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || diagnostic == nil ||
		diagnostic.OriginalClass != string(runtimecontract.FailureClassEffectMismatch) || diagnostic.OriginalCode != "undeclared_workspace_effect" ||
		response.Receipt.Changed ||
		response.Receipt.ObservedEffectScope != runtimecontract.RuntimeEffectUnknown {
		t.Fatalf("undeclared workspace effect was accepted: %+v", response)
	}
	if len(diagnostic.TargetRefs) != 1 || diagnostic.TargetRefs[0] != "rogue.txt" {
		t.Fatalf("undeclared target lineage = %+v", diagnostic)
	}
}

func TestShellOwnerAcceptsObservedEffectsWithinMultipleSealedTargets(t *testing.T) {
	adapter := NewRuntimeAdapter()
	workspace := t.TempDir()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace, Command: "sh", Args: []string{"-c", "printf one > first.txt; printf two > second.txt"},
		StateChanging: true, TargetRefs: []string{"first.txt", "second.txt"},
	})
	outside := shellUndeclaredWorkspaceTargets(result.WorkspaceObservedTargetChanges, []string{"first.txt", "second.txt"})
	if err != nil || !shellWorkspaceTargetChangesChanged(result.WorkspaceObservedTargetChanges) || len(outside) != 0 {
		t.Fatalf("declared multi-target observation result=%+v outside=%v err=%v", result, outside, err)
	}
	if got := shellObservedChangedWorkspaceTargets(result.WorkspaceObservedTargetChanges); len(got) != 2 || got[0] != "first.txt" || got[1] != "second.txt" {
		t.Fatalf("observed changed targets = %v", got)
	}
}

func TestShellOwnerAdmissibilityPersistsOnlyPreflightConfirmedValidationRecipes(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(modulecommon.ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "validation.txt"), []byte("ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := shellevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-shell", RunID: "run-shell", AuthoritySnapshotID: "authority-shell",
		PrerequisiteID: "admissibility-shell", OwnerModuleID: modulecommon.ModuleID,
		WorkspaceDeliverableDirectory: workspace, RequiredKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, RequiredTargetRefs: []string{"validation.txt"}, MaxItems: 4,
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{
			{ID: "confirmed", Command: "sh", Args: []string{"-c", "true"}},
			{ID: "unavailable", Command: "workorch-command-that-does-not-exist", Args: []string{"check"}},
		},
		WorkspaceManifestBinding: shellTestWorkspaceManifestBinding("tree-shell", []string{"validation.txt"}),
	}
	result := hub.Send(magicevent.NewEventWithContext(shellevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command))
	if result == nil {
		t.Fatal("Shell owner returned no admissibility result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("Shell admissibility: %v", err)
	}
	response, ok := data.(shellevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok {
		t.Fatalf("Shell admissibility response = %T", data)
	}
	if len(response.Acquisition.Admissibility.ValidationRecipes) != 1 || response.Acquisition.Admissibility.ValidationRecipes[0].ID != "confirmed" {
		t.Fatalf("owner-confirmed recipes = %+v", response.Acquisition.Admissibility.ValidationRecipes)
	}
	if len(response.Acquisition.Admissibility.TargetRefs) != 1 || response.Acquisition.Admissibility.TargetRefs[0] != "validation.txt" {
		t.Fatalf("owner-confirmed targets = %+v", response.Acquisition.Admissibility.TargetRefs)
	}
	if err := response.Acquisition.Admissibility.Validate(); err != nil {
		t.Fatalf("owner inventory: %v", err)
	}
	if response.Acquisition.Observation.ContentHash != response.Acquisition.Admissibility.SnapshotID {
		t.Fatalf("owner observation does not bind recipe inventory: %+v", response)
	}
	foundWorkspaceTree := false
	for _, dependency := range response.Acquisition.Observation.Dependencies {
		if dependency.Domain == runtimecontract.RuntimeResourceWorkspaceTree && dependency.OwnerModuleID == runtimecontract.RuntimeModuleFile && dependency.Revision == "tree-shell" {
			foundWorkspaceTree = true
		}
	}
	if !foundWorkspaceTree {
		t.Fatalf("owner observation omitted File workspace-tree dependency: %+v", response.Acquisition.Observation)
	}
}

func shellTestWorkspaceManifestBinding(revision string, targetRefs []string) runtimecontract.RuntimeWorkspaceManifestBinding {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "manifest-shell", Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		ContentHash: revision, Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: revision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return runtimecontract.RuntimeWorkspaceManifestBinding{
		Manifest: runtimecontract.WorkspaceManifest{Revision: revision, TargetRefs: append([]string(nil), targetRefs...)}, Observation: observation,
	}
}

func TestResolveManagedValidationRecipeDoesNotFallbackToActivationPolicy(t *testing.T) {
	request := RuntimeCommandRequest{
		ValidationRecipeID: "confirmed",
		ManagedValidation:  runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "confirmed", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "false"},
		}},
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{{ID: "confirmed", Command: "sh", Args: []string{"-c", "true"}}},
	}
	if err := resolveManagedValidationRecipe(&request); err != nil {
		t.Fatalf("resolve confirmed recipe: %v", err)
	}
	if request.Command != "sh" || len(request.Args) != 2 || request.Args[1] != "true" {
		t.Fatalf("resolver used activation declaration instead of owner-confirmed recipe: %+v", request)
	}
	request.ValidationRecipes = nil
	if err := resolveManagedValidationRecipe(&request); err == nil {
		t.Fatal("resolver accepted an activation policy without an owner-confirmed recipe")
	}
}

func shellStateChangingReceiptCommand(workspace, script string) shellevents.ExecuteOperationCommand {
	return shellevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/shell-write", Fence: 1,
				ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			},
			WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
			OperationID: "shell-write", EvidenceKind: runtimecontract.EvidenceKindStateChange,
			WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{"generated.txt"},
		},
		Data: shellevents.OperationData{
			Command: "sh", Args: []string{"-c", script}, StateChanging: true,
			TargetRef: "generated.txt", TargetRefs: []string{"generated.txt"},
		},
	}
}

func sendShellStateChangingReceiptCommand(t *testing.T, ctx context.Context, hub magicevent.Hub, command shellevents.ExecuteOperationCommand) shellevents.ExecuteOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(shellevents.ExecuteOperationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	return response
}
