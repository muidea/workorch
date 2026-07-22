package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGitOwnerStateChangingReceiptReplaysWithoutSecondOperationAfterOwnerRecreation(t *testing.T) {
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

	workspace := newTestRepo(t)
	writeFile(t, filepath.Join(workspace, "generated.txt"), "first\n")
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter(true)}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	command := gitStateChangingReceiptCommand(workspace, "generated.txt")
	first := sendGitStateChangingReceiptCommand(t, ctx, hub, command)
	if first.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !first.Receipt.Changed ||
		first.Receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		t.Fatalf("first receipt = %+v", first.Receipt)
	}
	if len(first.Receipt.ResourceChanges) != 1 || first.Receipt.ResourceChanges[0].Domain != runtimecontract.RuntimeResourceRepositoryState ||
		first.Receipt.ResourceChanges[0].OwnerModuleID != ModuleID || first.Receipt.ResourceChanges[0].AfterRevision == "" {
		t.Fatalf("first receipt resource changes = %+v", first.Receipt.ResourceChanges)
	}
	if staged := gitOutputForTest(t, workspace, "diff", "--cached", "--name-only"); staged != "generated.txt" {
		t.Fatalf("first owner effect staged = %q, want generated.txt", staged)
	}

	// Recreate the owner after the durable receipt has been committed. A new
	// delivery with the same key/fence but different operation data must return
	// the persisted receipt before it can alter the repository again.
	owner.Teardown(ctx)
	writeFile(t, filepath.Join(workspace, "replayed.txt"), "must remain unstaged\n")
	recreated := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter(true)}
	if err := recreated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { recreated.Teardown(ctx) })
	command.Data.Pathspecs = []string{"replayed.txt"}
	replayed := sendGitStateChangingReceiptCommand(t, ctx, hub, command)
	if replayed.Receipt.ExecutionKey != first.Receipt.ExecutionKey || replayed.Receipt.AfterHash != first.Receipt.AfterHash ||
		replayed.Receipt.Fence != first.Receipt.Fence {
		t.Fatalf("replayed receipt = %+v, want persisted %+v", replayed.Receipt, first.Receipt)
	}
	if staged := gitOutputForTest(t, workspace, "diff", "--cached", "--name-only"); staged != "generated.txt" {
		t.Fatalf("duplicate receipt replay ran another git add: staged = %q", staged)
	}
}

func gitStateChangingReceiptCommand(workspace, pathspec string) orchestratorevents.ExecuteGitOperationCommand {
	return orchestratorevents.ExecuteGitOperationCommand{
		Execution: orchestratorevents.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws-git/run-git/dispatch-git/git-add", Fence: 1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		},
		WorkspaceID: "ws-git", RunID: "run-git", DispatchID: "dispatch-git",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-git", RequiredOutputHash: "output-git"},
		OperationID: "git-add", OperationKind: runtimecontract.StepKindGitWrite,
		EvidenceKind:                  runtimecontract.EvidenceKindStateChange,
		WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{pathspec},
		Data: orchestratorevents.GitOperationData{Operation: gitcommon.OperationAdd, Pathspecs: []string{pathspec}},
	}
}

func sendGitStateChangingReceiptCommand(t *testing.T, ctx context.Context, hub magicevent.Hub, command orchestratorevents.ExecuteGitOperationCommand) orchestratorevents.ExecuteGitOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteGitOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.ExecuteGitOperationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	return response
}

func TestValidateCapabilityStatePredecessorReceiptsRequiresExactDurableBinding(t *testing.T) {
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
		{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"},
		{RequirementID: "stage", Kind: runtimecontract.StepKindGitWrite, VariantID: "add", DependsOnRequirementIDs: []string{"repo-init"}},
	}}
	required := snapshot.RequiredOperationSequence[1]
	binding := runtimecontract.RuntimeRequiredOperationReceiptBinding{
		RequirementID: "repo-init", OperationID: "git-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init",
		ExecutionKey: "exec/ws/run/dispatch/git-init", Fence: 1, ReceiptRef: "receipt/git-init", ReceiptHash: "receipt-hash",
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
	}
	if err := validateGitCapabilityStatePredecessorReceipts(snapshot, required, []runtimecontract.RuntimeRequiredOperationReceiptBinding{binding}); err != nil {
		t.Fatalf("accepted predecessor binding: %v", err)
	}
	if err := validateGitCapabilityStatePredecessorReceipts(snapshot, required, nil); err == nil {
		t.Fatal("missing predecessor binding was accepted")
	}
	binding.VariantID = "commit"
	if err := validateGitCapabilityStatePredecessorReceipts(snapshot, required, []runtimecontract.RuntimeRequiredOperationReceiptBinding{binding}); err == nil {
		t.Fatal("variant-mismatched predecessor binding was accepted")
	}
}

func TestGitReceiptResourceChangesUseOwnerObservedRepositoryRevision(t *testing.T) {
	changes := gitReceiptResourceChanges([]runtimecontract.RuntimeCapabilityStateSnapshot{{
		OwnerModuleID: ModuleID, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-before",
	}}, runtimecontract.RuntimeCapabilityStateSnapshot{
		OwnerModuleID: ModuleID, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-after",
	})
	if len(changes) != 1 || changes[0].Domain != runtimecontract.RuntimeResourceRepositoryState || changes[0].BeforeRevision != "repo-before" || changes[0].AfterRevision != "repo-after" {
		t.Fatalf("repository resource changes = %+v", changes)
	}
	if changes := gitReceiptResourceChanges(nil, runtimecontract.RuntimeCapabilityStateSnapshot{}); changes != nil {
		t.Fatalf("invalid post-observation produced changes: %+v", changes)
	}
}

func TestGitObservationsIncludeFileOwnedWorkspaceTreeDependency(t *testing.T) {
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: "git-state", OwnerModuleID: ModuleID, CapabilityStateKind: "repository_state",
		SubjectRef: "workspace_deliverable", ObservedRevision: "repo-v1", WorkspaceTreeRevision: "tree-v1", ContentHash: "content-v1",
	}
	stateObservation := gitCapabilityStateObservation("git-state", snapshot)
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "git-admissibility", OwnerModuleID: ModuleID,
		OperationKinds: []string{runtimecontract.StepKindGitWrite}, ObservedRevision: "repo-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	admissibilityObservation := gitAdmissibilityObservation("git-admissibility", inventory, snapshot.WorkspaceTreeRevision)
	change := runtimecontract.RuntimeResourceChange{
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2",
	}
	for _, observation := range []runtimecontract.RuntimePrerequisiteObservation{stateObservation, admissibilityObservation} {
		if err := observation.Validate(); err != nil {
			t.Fatalf("observation invalid: %v", err)
		}
		matched := false
		for _, dependency := range observation.Dependencies {
			matched = matched || runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change)
		}
		if !matched {
			t.Fatalf("File workspace tree change did not intersect Git observation: %+v", observation)
		}
	}
}

func TestGitOwnerCapabilityBoundAdmissibilityRejectsMismatchedProbeBindingAsStale(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	store, err := NewCapabilityStateReceiptStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fileObserver := magicevent.NewSimpleObserver(orchestratorevents.PrerequisiteCommandDestination, hub)
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.AcquireWorkspaceManifestCommand)
		result.Set(orchestratorevents.AcquireWorkspaceManifestResult{
			PrerequisiteID: command.PrerequisiteID,
			Manifest:       runtimecontract.WorkspaceManifest{Revision: "file-tree-v1", TargetRefs: []string{"main.go"}},
		}, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest) })
	workspace := newTestRepo(t)
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter(true), capabilityReceipts: store}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	capabilityCommand := orchestratorevents.AcquireGitCapabilityStateCommand{
		WorkspaceID: "ws-binding", RunID: "run-binding", AuthoritySnapshotID: "authority-binding",
		PrerequisiteID: "capability-state:git", WorkspaceDeliverableDirectory: workspace,
		CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", AcquisitionKey: "capability-state/ws-binding/run-binding/git/0",
	}
	capabilityResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorAcquireGitCapabilityState, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, capabilityCommand,
	))
	capabilityData, err := capabilityResult.Get()
	if err != nil && err.Error() != "<nil>" {
		t.Fatal(err)
	}
	capability, ok := capabilityData.(orchestratorevents.AcquireGitCapabilityStateResult)
	if !ok || capability.Snapshot.AcquisitionBinding != capabilityCommand.AcquisitionKey || capability.Snapshot.WorkspaceTreeRevision != "file-tree-v1" {
		t.Fatalf("capability acquisition = %#v", capabilityData)
	}

	admissibilityCommand := orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-binding", RunID: "run-binding", AuthoritySnapshotID: "authority-binding",
		PrerequisiteID: "admissibility:git", OwnerModuleID: ModuleID, WorkspaceDeliverableDirectory: workspace,
		RequiredKinds: []string{StepKindRead}, CapabilityStateBinding: capability.Snapshot.AcquisitionBinding, MaxItems: 4,
	}
	admissibilityResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, admissibilityCommand,
	))
	admissibilityData, err := admissibilityResult.Get()
	if err != nil && err.Error() != "<nil>" {
		t.Fatal(err)
	}
	admissibility, ok := admissibilityData.(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || admissibility.Admissibility.AcquisitionBinding != capability.Snapshot.AcquisitionBinding {
		t.Fatalf("bound admissibility = %#v", admissibilityData)
	}

	operationName := "status"
	observation := gitAdmissibilityObservation(admissibilityCommand.PrerequisiteID, admissibility.Admissibility, capability.Snapshot.WorkspaceTreeRevision)
	probe := orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{
		WorkspaceID: "ws-binding", RunID: "run-binding", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-binding"},
		WorkspaceDeliverableDirectory: workspace, Admissibility: admissibility.Admissibility, Observation: &observation,
		// This simulates a stale aggregate combining an old capability binding
		// with a newer admissibility inventory. It is stale, never infeasible.
		CapabilityStateBinding: "capability-state/ws-binding/run-binding/git/old",
		Operation:              runtimecontract.RuntimeOperation{ID: "git-status", Kind: StepKindRead, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &operationName}}},
	}
	probeResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, probe,
	))
	probeData, err := probeResult.Get()
	if err != nil && err.Error() != "<nil>" {
		t.Fatal(err)
	}
	response, ok := probeData.(orchestratorevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok || response.Feasible || response.Failure != nil || response.Stale == nil || response.CapabilityStateBinding != capability.Snapshot.AcquisitionBinding {
		t.Fatalf("mismatched binding probe = %#v", probeData)
	}
}

func TestRuntimeOperationAdmissibilityProbeUsesCurrentRepositorySnapshot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	workspace := t.TempDir()
	readme := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readme, []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.local"},
		{"config", "user.name", "WorkOrch Test"},
		{"add", "README.md"},
		{"commit", "-m", "initial"},
	} {
		if output, err := exec.Command("git", append([]string{"-C", workspace}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	module := &Biz{runtimeAdapter: NewRuntimeAdapter(true)}
	snapshot, err := module.runtimeAdapter.CapabilityState(context.Background(), "admissibility-git", "workspace_deliverable", workspace)
	if err != nil {
		t.Fatalf("acquire read-only repository state: %v", err)
	}
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility(
		"admissibility-git", ModuleID, []string{StepKindRead}, nil, nil,
		[]string{"repository-state-snapshot:" + snapshot.SnapshotID}, snapshot.ObservedRevision,
	)
	if err != nil {
		t.Fatal(err)
	}
	operationName := "status"
	command := orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{
		WorkspaceID: "ws-git", RunID: "run-git", WorkspaceDeliverableDirectory: workspace,
		Transaction:   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-git"},
		Admissibility: inventory,
		Operation:     runtimecontract.RuntimeOperation{ID: "status", Kind: StepKindRead, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &operationName}}},
	}
	if failure := module.runtimeOperationFeasibility(context.Background(), command); failure != nil {
		t.Fatalf("current repository snapshot rejected admissible status: %+v", failure)
	}
	if err := os.WriteFile(readme, []byte("changed after inventory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failure := module.runtimeOperationFeasibility(context.Background(), command)
	if failure == nil || failure.Code != "operation_admissibility_stale" {
		t.Fatalf("stale repository snapshot failure = %+v", failure)
	}
}
