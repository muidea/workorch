package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cd "github.com/muidea/magicCommon/def"
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
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
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
	executionContext := runtimecontract.RuntimeOperationExecutionContext{
		Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/validate", Fence: 1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession,
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
	}
	recipes := []runtimecontract.RuntimeValidationRecipe{{
		ID: "go-test-zero-work", Command: "go", Args: []string{"test", "-v", "-run", "TestMissing", "./..."},
	}}
	command := sealedShellTestCommand(t, owner.runtimeAdapter, executionContext, runtimecontract.RuntimeBlockKindManagedValidationSession, shellevents.OperationData{
		ValidationRecipeID: "go-test-zero-work",
		TargetRefs:         []string{"fixture_test.go"},
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
	}, "", recipes, nil)
	first := sendShellStateChangingReceiptCommand(t, ctx, hub, command)
	if first.Receipt.Evidence.Quality != "inconclusive" || !first.Receipt.Evidence.ZeroWork ||
		first.Receipt.Evidence.Validation == nil || first.Receipt.Evidence.EvidenceReason != "no matching validation checks" {
		t.Fatalf("zero-work receipt = %+v", first.Receipt)
	}
	firstHash := runtimecontract.StableRuntimeOperationReceiptHash(first.Receipt)
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
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
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
	command := shellStateChangingReceiptCommand(t, owner.runtimeAdapter, workspace, "printf first > generated.txt")
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

	// The same sealed delivery must return the durable receipt instead of running
	// a second Shell effect.
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
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
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
	command := shellStateChangingReceiptCommand(t, owner.runtimeAdapter, workspace, "printf declared > generated.txt; printf unexpected > rogue.txt")
	response := sendShellStateChangingReceiptCommand(t, ctx, hub, command)
	diagnostic := response.Receipt.Feedback.OriginalDiagnostic
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || diagnostic == nil ||
		diagnostic.OriginalClass != string(runtimecontract.FailureClassEffectMismatch) || diagnostic.OriginalCode != "undeclared_workspace_effect" ||
		!response.Receipt.Changed ||
		response.Receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		t.Fatalf("undeclared workspace effect was accepted: %+v", response)
	}
	if len(diagnostic.TargetRefs) != 0 || diagnostic.TargetObservation != nil || diagnostic.ValidationFailureObservation != nil {
		t.Fatalf("undeclared audit subject became failure-target authority: %+v", diagnostic)
	}
	if len(response.Receipt.TargetRefs) != 1 || response.Receipt.TargetRefs[0] != "generated.txt" {
		t.Fatalf("rejected receipt targets escaped sealed operation: %+v", response.Receipt.TargetRefs)
	}
	changedTargets := map[string]bool{}
	for _, change := range response.Receipt.ResourceChanges {
		if change.Domain == runtimecontract.RuntimeResourceWorkspaceTarget && change.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			changedTargets[change.SubjectRef] = true
		}
	}
	if !changedTargets["generated.txt"] || !changedTargets["rogue.txt"] {
		t.Fatalf("undeclared workspace effect audit was lost: %+v", response.Receipt.ResourceChanges)
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
	if len(response.Acquisition.Admissibility.ValidationSurfaces) != 2 {
		t.Fatalf("owner validation surfaces = %+v", response.Acquisition.Admissibility.ValidationSurfaces)
	}
	surface, found := shellValidationSurfaceForTest(response.Acquisition.Admissibility.ValidationSurfaces, "confirmed")
	if !found {
		t.Fatalf("owner omitted confirmed validation surface: %+v", response.Acquisition.Admissibility.ValidationSurfaces)
	}
	if surface.ID != "confirmed" || surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady ||
		surface.InputBindingRef == "" || !strings.Contains(surface.InputBindingRef, "tree-shell") || surface.PrerequisiteRef != "" {
		t.Fatalf("owner-confirmed validation surface = %+v", surface)
	}
	unavailable, found := shellValidationSurfaceForTest(response.Acquisition.Admissibility.ValidationSurfaces, "unavailable")
	if !found || unavailable.Readiness != runtimecontract.RuntimeValidationSurfaceUnavailable ||
		unavailable.ReadinessReason != shellValidationStaticUnavailableReason || unavailable.InputBindingRef != "" || unavailable.PrerequisiteRef != "" {
		t.Fatalf("owner unavailable validation surface = %+v found=%t", unavailable, found)
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

func TestShellOwnerAdmissibilityPublishesManifestPrerequisiteBeforeValidationIsReady(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
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
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "go-test", Command: "go", Args: []string{"test", "./..."}}
	acquire := func(prerequisiteID, revision string, targetRefs []string) runtimecontract.RuntimeOperationAdmissibility {
		t.Helper()
		command := shellevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: "ws-shell-prerequisite", RunID: "run-shell-prerequisite", AuthoritySnapshotID: "authority-shell-prerequisite",
			PrerequisiteID: prerequisiteID, OwnerModuleID: modulecommon.ModuleID,
			WorkspaceDeliverableDirectory: workspace, RequiredKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			RequiredTargetRefs: targetRefs, MaxItems: 8, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
			WorkspaceManifestBinding: shellTestWorkspaceManifestBinding(revision, targetRefs),
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
		return response.Acquisition.Admissibility
	}

	missingModule := acquire("admissibility-shell-missing-module", "tree-test-without-module", []string{"main.go", "main_test.go"})
	missingModuleSurface, found := shellValidationSurfaceForTest(missingModule.ValidationSurfaces, recipe.ID)
	if !found || missingModuleSurface.Readiness != runtimecontract.RuntimeValidationSurfacePrerequisiteRequired ||
		missingModuleSurface.ReadinessReason != shellValidationApplicableInputRequiredReason || missingModuleSurface.PrerequisiteRef == "" ||
		missingModuleSurface.ProspectiveSatisfactionContractRef != modulecommon.ProspectiveSatisfactionContractRef || missingModuleSurface.ProspectiveTargetSatisfactionConstraint == nil ||
		missingModuleSurface.InputBindingRef != "" || len(missingModule.ValidationRecipes) != 0 || len(missingModule.ValidationRecipeRefs) != 0 {
		t.Fatalf("missing-module validation inventory = %+v surface=%+v found=%t", missingModule, missingModuleSurface, found)
	}
	if !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint([]string{"go.mod"}, *missingModuleSurface.ProspectiveTargetSatisfactionConstraint) {
		t.Fatalf("missing-module prospective contract did not admit exact root module: %+v", missingModuleSurface.ProspectiveTargetSatisfactionConstraint)
	}

	multipleMissing := acquire("admissibility-shell-multiple-missing", "tree-without-module-or-tests", []string{"main.go"})
	multipleMissingSurface, found := shellValidationSurfaceForTest(multipleMissing.ValidationSurfaces, recipe.ID)
	if !found || multipleMissingSurface.Readiness != runtimecontract.RuntimeValidationSurfacePrerequisiteRequired ||
		multipleMissingSurface.ProspectiveSatisfactionContractRef != modulecommon.ProspectiveSatisfactionContractRef || multipleMissingSurface.ProspectiveTargetSatisfactionConstraint == nil {
		t.Fatalf("multi-gap validation inventory did not publish a bounded all-of contract = %+v surface=%+v found=%t", multipleMissing, multipleMissingSurface, found)
	}
	if !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint([]string{"go.mod", "runtime_test.go"}, *multipleMissingSurface.ProspectiveTargetSatisfactionConstraint) {
		t.Fatalf("multi-gap prospective contract did not close module and test clauses: %+v", multipleMissingSurface.ProspectiveTargetSatisfactionConstraint)
	}

	waiting := acquire("admissibility-shell-waiting", "tree-module-without-tests", []string{"go.mod", "main.go"})
	waitingSurface, found := shellValidationSurfaceForTest(waiting.ValidationSurfaces, recipe.ID)
	if !found || waitingSurface.Readiness != runtimecontract.RuntimeValidationSurfacePrerequisiteRequired ||
		waitingSurface.ReadinessReason != shellValidationApplicableInputRequiredReason || waitingSurface.PrerequisiteRef == "" ||
		waitingSurface.ProspectiveSatisfactionContractRef != modulecommon.ProspectiveSatisfactionContractRef ||
		waitingSurface.ProspectiveTargetSatisfactionConstraint == nil ||
		waitingSurface.ProspectiveTargetSatisfactionConstraint.CatalogObservedRevision != waiting.ObservedRevision ||
		!runtimecontract.RuntimeProspectiveTargetSatisfiesConstraint("owner_fact_test.go", *waitingSurface.ProspectiveTargetSatisfactionConstraint) ||
		waitingSurface.InputBindingRef != "" || len(waiting.ValidationRecipes) != 0 || len(waiting.ValidationRecipeRefs) != 0 {
		t.Fatalf("manifest-prerequisite validation inventory = %+v surface=%+v found=%t", waiting, waitingSurface, found)
	}

	ready := acquire("admissibility-shell-ready", "tree-with-module-and-tests", []string{"go.mod", "main.go", "main_test.go"})
	readySurface, found := shellValidationSurfaceForTest(ready.ValidationSurfaces, recipe.ID)
	if !found || readySurface.Readiness != runtimecontract.RuntimeValidationSurfaceReady ||
		!strings.Contains(readySurface.InputBindingRef, "tree-with-module-and-tests") || readySurface.PrerequisiteRef != "" ||
		readySurface.ProspectiveSatisfactionContractRef != "" ||
		readySurface.ProspectiveTargetSatisfactionConstraint != nil ||
		len(ready.ValidationRecipes) != 1 || len(ready.ValidationRecipeRefs) != 1 || ready.ValidationRecipeRefs[0] != recipe.ID {
		t.Fatalf("ready validation inventory = %+v surface=%+v found=%t", ready, readySurface, found)
	}
	if waiting.SnapshotID == ready.SnapshotID {
		t.Fatal("validation readiness transition did not change owner snapshot identity")
	}
}

func shellValidationSurfaceForTest(values []runtimecontract.RuntimeValidationSurface, id string) (runtimecontract.RuntimeValidationSurface, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return runtimecontract.RuntimeValidationSurface{}, false
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

func shellStateChangingReceiptCommand(t *testing.T, adapter *RuntimeAdapter, workspace, script string) shellevents.ExecuteOperationCommand {
	t.Helper()
	executionContext := runtimecontract.RuntimeOperationExecutionContext{
		Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/shell-write", Fence: 1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		},
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell", RequiredOutputHash: "output-shell"},
		OperationID: "shell-write", EvidenceKind: runtimecontract.EvidenceKindStateChange,
		WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{"generated.txt"},
	}
	return sealedShellTestCommand(t, adapter, executionContext, runtimecontract.StepKindShellCommand, shellevents.OperationData{
		Command: "sh", Args: []string{"-c", script}, StateChanging: true,
		TargetRef: "generated.txt", TargetRefs: []string{"generated.txt"},
	}, "", nil, nil)
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

func TestShellExecuteHandlerRejectsNilResultBeforeReceiptQueryOrEffect(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	owner := &Biz{runtimeAdapter: NewRuntimeAdapter()}
	command := shellStateChangingReceiptCommand(t, owner.runtimeAdapter, workspace, "printf changed > generated.txt")
	event := magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	)
	owner.handleShellOperation(event, nil)
	if _, err := os.Stat(filepath.Join(workspace, "generated.txt")); !os.IsNotExist(err) {
		t.Fatalf("nil-result Shell execute changed workspace: %v", err)
	}
}

func TestShellExecuteRejectsUntrustedSourceAndReceiptQueryFailureBeforeEffect(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	queries, commits := 0, 0
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		queries++
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "receipt authority unavailable"})
	})
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		commits++
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "unexpected commit"})
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
	command := shellStateChangingReceiptCommand(t, owner.runtimeAdapter, workspace, "printf changed > generated.txt")

	spoofed := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleFile, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	if _, err := spoofed.Get(); err == nil {
		t.Fatal("Shell accepted execute command from a non-Orchestrator source")
	}
	if queries != 0 || commits != 0 {
		t.Fatalf("untrusted source reached receipt authority: queries=%d commits=%d", queries, commits)
	}

	missing := command
	missing.SealedOperation = shellevents.SealedRuntimeOperation{}
	missingResult := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, missing,
	))
	if _, err := missingResult.Get(); err == nil {
		t.Fatal("Shell accepted missing sealed payload")
	}
	tampered := command
	tampered.SealedOperation.Authorization.TargetRole = "tampered-role"
	tamperedResult := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, tampered,
	))
	if _, err := tamperedResult.Get(); err == nil {
		t.Fatal("Shell accepted tampered detached authorization")
	}
	tamperedExecution := command
	tamperedExecution.Execution.AdmissibilitySnapshotID = "another-owner-catalog"
	tamperedExecutionResult := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, tamperedExecution,
	))
	if _, err := tamperedExecutionResult.Get(); err == nil {
		t.Fatal("Shell accepted execution record bound to another owner catalog")
	}
	if queries != 0 || commits != 0 {
		t.Fatalf("missing/tampered Shell seal or execution record reached receipt authority: queries=%d commits=%d", queries, commits)
	}
	if _, err := os.Stat(filepath.Join(workspace, "generated.txt")); !os.IsNotExist(err) {
		t.Fatalf("missing/tampered Shell seal crossed process fence: %v", err)
	}

	failedQuery := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	if _, err := failedQuery.Get(); err == nil {
		t.Fatal("Shell ignored receipt authorization query failure")
	}
	if queries != 1 || commits != 0 {
		t.Fatalf("receipt query failure crossed effect fence: queries=%d commits=%d", queries, commits)
	}
	if _, err := os.Stat(filepath.Join(workspace, "generated.txt")); !os.IsNotExist(err) {
		t.Fatalf("receipt query failure changed workspace: %v", err)
	}
}
