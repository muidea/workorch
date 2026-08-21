package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustRuntimeOperationAuthorizationIdentity(t *testing.T, kind string) runtimecontract.RuntimeCandidateOperationManifest {
	t.Helper()
	authorization, err := runtimecontract.RuntimeOperationAuthorizationIdentity(kind)
	if err != nil {
		t.Fatalf("runtime operation authorization identity for %q: %v", kind, err)
	}
	return authorization
}

func gitFileManifestResponse(command fileevents.AcquireWorkspaceManifestCommand) fileevents.AcquireWorkspaceManifestResult {
	manifest := runtimecontract.WorkspaceManifest{Revision: "file-tree-v1", TargetRefs: []string{"main.go"}}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: command.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: manifest.Revision,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: manifest.Revision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return fileevents.AcquireWorkspaceManifestResult{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID,
		Manifest: manifest, Observation: observation,
	}
}

func TestGitFileWorkspaceTreeResponderMatrixFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		terminate  bool
		responder  func(magicevent.Event, magicevent.Result)
		wantResult bool
	}{
		{name: "valid", wantResult: true, responder: func(event magicevent.Event, result magicevent.Result) {
			result.Set(gitFileManifestResponse(event.Data().(fileevents.AcquireWorkspaceManifestCommand)), nil)
		}},
		{name: "missing responder"},
		{name: "nil result", terminate: true},
		{name: "owner error", responder: func(_ magicevent.Event, result magicevent.Result) {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "injected File failure", Cause: common.ErrConflict})
		}},
		{name: "wrong result type", responder: func(_ magicevent.Event, result magicevent.Result) { result.Set("wrong", nil) }},
		{name: "wrong workspace", responder: func(event magicevent.Event, result magicevent.Result) {
			response := gitFileManifestResponse(event.Data().(fileevents.AcquireWorkspaceManifestCommand))
			response.WorkspaceID = "ws-other"
			result.Set(response, nil)
		}},
		{name: "wrong run", responder: func(event magicevent.Event, result magicevent.Result) {
			response := gitFileManifestResponse(event.Data().(fileevents.AcquireWorkspaceManifestCommand))
			response.RunID = "run-other"
			result.Set(response, nil)
		}},
		{name: "wrong authority", responder: func(event magicevent.Event, result magicevent.Result) {
			response := gitFileManifestResponse(event.Data().(fileevents.AcquireWorkspaceManifestCommand))
			response.AuthoritySnapshotID = "authority-other"
			result.Set(response, nil)
		}},
		{name: "wrong prerequisite", responder: func(event magicevent.Event, result magicevent.Result) {
			response := gitFileManifestResponse(event.Data().(fileevents.AcquireWorkspaceManifestCommand))
			response.Observation.PrerequisiteID = "prerequisite-other"
			response.Observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(response.Observation)
			result.Set(response, nil)
		}},
		{name: "invalid manifest binding", responder: func(event magicevent.Event, result magicevent.Result) {
			response := gitFileManifestResponse(event.Data().(fileevents.AcquireWorkspaceManifestCommand))
			response.Manifest.Revision = "file-tree-other"
			result.Set(response, nil)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(8)
			if test.terminate {
				hub.Terminate(ctx)
			} else {
				t.Cleanup(func() { hub.Terminate(ctx) })
			}
			if test.responder != nil {
				observer := magicevent.NewSimpleObserver(fileevents.PrerequisiteCommandDestination, hub)
				observer.Subscribe(fileevents.TopicAcquireWorkspaceManifest, test.responder)
				t.Cleanup(func() { observer.Unsubscribe(fileevents.TopicAcquireWorkspaceManifest) })
			}
			owner := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			revision, err := owner.acquireFileWorkspaceTreeRevision(ctx, "ws-1", "run-1", "authority-1", "prerequisite-1", t.TempDir())
			if test.wantResult {
				if err != nil || revision != "file-tree-v1" {
					t.Fatalf("revision=%q err=%v", revision, err)
				}
				return
			}
			if err == nil || revision != "" {
				t.Fatalf("revision=%q err=%v, want fail closed", revision, err)
			}
		})
	}
}

func TestGitOwnerStateChangingReceiptReplaysWithoutSecondOperationAfterOwnerRecreation(t *testing.T) {
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

	workspace := newTestRepo(t)
	writeFile(t, filepath.Join(workspace, "generated.txt"), "first\n")
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter(true)}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	command := gitStateChangingReceiptCommand(t, workspace, "generated.txt")
	first := sendGitStateChangingReceiptCommand(t, ctx, hub, command)
	if first.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !first.Receipt.Changed ||
		first.Receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		t.Fatalf("first receipt = %+v", first.Receipt)
	}
	if len(first.Receipt.ResourceChanges) != 1 || first.Receipt.ResourceChanges[0].Domain != runtimecontract.RuntimeResourceRepositoryState ||
		first.Receipt.ResourceChanges[0].OwnerModuleID != ModuleID || first.Receipt.ResourceChanges[0].AfterRevision == "" {
		t.Fatalf("first receipt resource changes = %+v", first.Receipt.ResourceChanges)
	}
	if len(first.Receipt.Evidence.ContentHash) != 64 || len(first.Receipt.Evidence.EvidenceRefs) != 1 ||
		first.Receipt.Evidence.EvidenceRefs[0] != "workorch://git/change/generated.txt" ||
		first.Receipt.BeforeHash == "" || first.Receipt.AfterHash == "" || first.Receipt.BeforeHash == first.Receipt.AfterHash {
		t.Fatalf("first receipt bounded Git evidence = %+v", first.Receipt)
	}
	if staged := gitOutputForTest(t, workspace, "diff", "--cached", "--name-only"); staged != "generated.txt" {
		t.Fatalf("first owner effect staged = %q, want generated.txt", staged)
	}

	// Recreate the owner after the durable receipt has been committed. A new
	// delivery with the same key/fence and exact sealed payload must return the
	// persisted receipt before it can alter the repository again.
	owner.Teardown(ctx)
	writeFile(t, filepath.Join(workspace, "replayed.txt"), "must remain unstaged\n")
	recreated := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter(true)}
	if err := recreated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { recreated.Teardown(ctx) })
	replayed := sendGitStateChangingReceiptCommand(t, ctx, hub, command)
	if replayed.Receipt.ExecutionKey != first.Receipt.ExecutionKey || replayed.Receipt.AfterHash != first.Receipt.AfterHash ||
		replayed.Receipt.Fence != first.Receipt.Fence {
		t.Fatalf("replayed receipt = %+v, want persisted %+v", replayed.Receipt, first.Receipt)
	}
	if staged := gitOutputForTest(t, workspace, "diff", "--cached", "--name-only"); staged != "generated.txt" {
		t.Fatalf("duplicate receipt replay ran another git add: staged = %q", staged)
	}
}

func gitStateChangingReceiptCommand(t *testing.T, workspace, pathspec string) gitevents.ExecuteOperationCommand {
	t.Helper()
	operationName := gitcommon.OperationAdd
	operation := runtimecontract.RuntimeOperation{
		ID: "git-add", Kind: runtimecontract.StepKindGitWrite, TargetRefs: []string{pathspec},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "operation", String: &operationName},
			{Name: "pathspecs", StringList: []string{pathspec}},
		},
	}
	sealed, err := gitevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatalf("seal Git operation: %v", err)
	}
	return gitevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-git/run-git/dispatch-git/git-add", Fence: 1,
				ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			},
			WorkspaceID: "ws-git", RunID: "run-git", DispatchID: "dispatch-git",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-git", RequiredOutputHash: "output-git"},
			OperationID: "git-add", EvidenceKind: runtimecontract.EvidenceKindStateChange,
			WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{pathspec},
		},
		SealedOperation: sealed,
	}
}

func sendGitStateChangingReceiptCommand(t *testing.T, ctx context.Context, hub magicevent.Hub, command gitevents.ExecuteOperationCommand) gitevents.ExecuteOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(gitevents.ExecuteOperationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	return response
}

func TestGitOwnerExecutionRejectsPostWrongSourceAndPayloadDriftBeforeEffect(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	workspace := newTestRepo(t)
	writeFile(t, filepath.Join(workspace, "generated.txt"), "content\n")
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter(true)}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	command := gitStateChangingReceiptCommand(t, workspace, "generated.txt")

	owner.handleGitOperation(magicevent.NewEventWithContext(
		gitevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	), nil)
	assertGitIndexEmpty(t, workspace)

	wrongSource := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicExecuteOperation, "not-orchestrator", ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	if _, err := wrongSource.Get(); err == nil {
		t.Fatal("Git execution accepted a non-Orchestrator source")
	}
	assertGitIndexEmpty(t, workspace)

	tampered := command
	tampered.SealedOperation.Operation.Parameters[1].StringList = []string{"other.txt"}
	tamperedResult := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, tampered,
	))
	if _, err := tamperedResult.Get(); err == nil {
		t.Fatal("Git execution accepted operation data that drifted from the sealed hash")
	}
	assertGitIndexEmpty(t, workspace)

	writeFile(t, filepath.Join(workspace, "other.txt"), "must remain unstaged\n")
	mismatchedOperation := command.SealedOperation.Operation
	mismatchedOperation.Parameters[1].StringList = []string{"other.txt"}
	mismatchedSeal, err := gitevents.SealRuntimeOperation(mismatchedOperation)
	if err != nil {
		t.Fatal(err)
	}
	mismatched := command
	mismatched.SealedOperation = mismatchedSeal
	mismatchedResult := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, mismatched,
	))
	if _, err := mismatchedResult.Get(); err == nil {
		t.Fatal("Git execution accepted a sealed pathspec outside the sealed target refs")
	}
	assertGitIndexEmpty(t, workspace)
}

func assertGitIndexEmpty(t *testing.T, workspace string) {
	t.Helper()
	command := exec.Command("git", "diff", "--cached", "--name-only")
	command.Dir = workspace
	output, err := command.Output()
	if err != nil {
		t.Fatalf("inspect Git index: %v", err)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("Git index changed before execution authorization: %q", output)
	}
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
	if changes := gitReceiptResourceChanges([]runtimecontract.RuntimeCapabilityStateSnapshot{{
		OwnerModuleID: ModuleID, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-same",
	}}, runtimecontract.RuntimeCapabilityStateSnapshot{
		OwnerModuleID: ModuleID, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-same",
	}); changes != nil {
		t.Fatalf("unchanged repository revision produced changes: %+v", changes)
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
	fileObserver.Subscribe(fileevents.TopicAcquireWorkspaceManifest, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.AcquireWorkspaceManifestCommand)
		manifest := runtimecontract.WorkspaceManifest{Revision: "file-tree-v1", TargetRefs: []string{"main.go"}}
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: command.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: manifest.Revision,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: manifest.Revision}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		result.Set(fileevents.AcquireWorkspaceManifestResult{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID,
			Manifest: manifest, Observation: observation,
		}, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(fileevents.TopicAcquireWorkspaceManifest) })
	workspace := newTestRepo(t)
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter(true), capabilityReceipts: store}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	capabilityCommand := gitevents.AcquireCapabilityStateCommand{
		WorkspaceID: "ws-binding", RunID: "run-binding", AuthoritySnapshotID: "authority-binding",
		PrerequisiteID: "capability-state:git", WorkspaceDeliverableDirectory: workspace,
		CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", AcquisitionKey: "capability-state/ws-binding/run-binding/git/0",
	}
	capabilityResult := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicAcquireCapabilityState, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, capabilityCommand,
	))
	capabilityData, err := capabilityResult.Get()
	if err != nil && err.Error() != "<nil>" {
		t.Fatal(err)
	}
	capability, ok := capabilityData.(gitevents.AcquireCapabilityStateResult)
	if !ok || capability.Snapshot.AcquisitionBinding != capabilityCommand.AcquisitionKey || capability.Snapshot.WorkspaceTreeRevision != "file-tree-v1" {
		t.Fatalf("capability acquisition = %#v", capabilityData)
	}

	admissibilityCommand := gitevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-binding", RunID: "run-binding", AuthoritySnapshotID: "authority-binding",
		PrerequisiteID: "admissibility:git", OwnerModuleID: ModuleID, WorkspaceDeliverableDirectory: workspace,
		RequiredKinds: []string{StepKindRead}, CapabilityStateBinding: capability.Snapshot.AcquisitionBinding, MaxItems: 4,
	}
	admissibilityResult := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, admissibilityCommand,
	))
	admissibilityData, err := admissibilityResult.Get()
	if err != nil && err.Error() != "<nil>" {
		t.Fatal(err)
	}
	admissibility, ok := admissibilityData.(gitevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || admissibility.Acquisition.Admissibility.AcquisitionBinding != capability.Snapshot.AcquisitionBinding {
		t.Fatalf("bound admissibility = %#v", admissibilityData)
	}
	if got := admissibility.Acquisition.Admissibility.SelectorRefs; len(got) != 1 || got[0] != gitWorkspaceRepositorySelector {
		t.Fatalf("Git semantic selector refs = %+v, want %q", got, gitWorkspaceRepositorySelector)
	}
	variantRefs := make([]runtimecontract.RuntimeOperationVariantRef, 0, len(admissibility.Acquisition.Admissibility.OperationVariants))
	for _, variant := range admissibility.Acquisition.Admissibility.OperationVariants {
		variantRefs = append(variantRefs, runtimecontract.RuntimeOperationVariantRef{Kind: variant.Kind, VariantID: variant.VariantID})
	}
	wantVariantRefs := []runtimecontract.RuntimeOperationVariantRef{{Kind: StepKindRead, VariantID: "status"}, {Kind: StepKindRead, VariantID: "diff"}, {Kind: StepKindRead, VariantID: "log"}, {Kind: StepKindRead, VariantID: "show"}, {Kind: StepKindRead, VariantID: "branch"}}
	if !runtimecontract.SameRuntimeOperationVariantRefs(variantRefs, wantVariantRefs) {
		t.Fatalf("Git owner variant facts = %+v, want %+v", variantRefs, wantVariantRefs)
	}

	operationName := "status"
	observation := gitAdmissibilityObservation(admissibilityCommand.PrerequisiteID, admissibility.Acquisition.Admissibility, capability.Snapshot.WorkspaceTreeRevision)
	probe := gitevents.ProbeRuntimeOperationFeasibilityCommand{
		WorkspaceID: "ws-binding", RunID: "run-binding", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-binding"},
		WorkspaceDeliverableDirectory: workspace, Admissibility: admissibility.Acquisition.Admissibility, Observation: &observation,
		CandidateManifestHash: "manifest-git-binding", OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, StepKindRead),
		// This simulates a stale aggregate combining an old capability binding
		// with a newer admissibility inventory. It is stale, never infeasible.
		CapabilityStateBinding: "capability-state/ws-binding/run-binding/git/old",
		Operation:              runtimecontract.RuntimeOperation{ID: "git-status", Kind: StepKindRead, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &operationName}}},
	}
	probeResult := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, probe,
	))
	probeData, err := probeResult.Get()
	if err != nil && err.Error() != "<nil>" {
		t.Fatal(err)
	}
	response, ok := probeData.(gitevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok || response.Outcome != runtimecontract.RuntimeOwnerFeasibilityStale || response.Feedback != nil || response.Stale == nil || response.CapabilityStateBinding != capability.Snapshot.AcquisitionBinding {
		t.Fatalf("mismatched binding probe = %#v", probeData)
	}
}

func TestGitOwnerRechecksFileWorkspaceTreeBeforeFeasibilityAndExecution(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	store, err := NewCapabilityStateReceiptStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	treeRevision := "file-tree-v1"
	fileObserver := magicevent.NewSimpleObserver(orchestratorevents.PrerequisiteCommandDestination, hub)
	fileObserver.Subscribe(fileevents.TopicAcquireWorkspaceManifest, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.AcquireWorkspaceManifestCommand)
		manifest := runtimecontract.WorkspaceManifest{Revision: treeRevision, TargetRefs: []string{"generated.txt"}}
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: command.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindManifest,
			OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: manifest.Revision,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				SubjectRef: "workspace_deliverable", Revision: manifest.Revision,
			}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		result.Set(fileevents.AcquireWorkspaceManifestResult{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID,
			Manifest: manifest, Observation: observation,
		}, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(fileevents.TopicAcquireWorkspaceManifest) })

	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: false}, nil)
	})
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})

	workspace := newTestRepo(t)
	writeFile(t, filepath.Join(workspace, "generated.txt"), "sealed content\n")
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter(true), capabilityReceipts: store}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	capabilityCommand := gitevents.AcquireCapabilityStateCommand{
		WorkspaceID: "ws-tree", RunID: "run-tree", AuthoritySnapshotID: "authority-tree",
		PrerequisiteID: "capability-state:git", WorkspaceDeliverableDirectory: workspace,
		CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
		AcquisitionKey: "capability-state/ws-tree/run-tree/git/0",
	}
	capabilityResult := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicAcquireCapabilityState, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, capabilityCommand,
	))
	capabilityData, resultErr := capabilityResult.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	capability := capabilityData.(gitevents.AcquireCapabilityStateResult)

	admissibilityCommand := gitevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: capabilityCommand.WorkspaceID, RunID: capabilityCommand.RunID,
		AuthoritySnapshotID: capabilityCommand.AuthoritySnapshotID, PrerequisiteID: "admissibility:git",
		OwnerModuleID: ModuleID, WorkspaceDeliverableDirectory: workspace,
		RequiredKinds:          []string{runtimecontract.StepKindGitWrite},
		CapabilityStateBinding: capability.Snapshot.AcquisitionBinding, MaxItems: 4,
	}
	admissibilityResult := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, admissibilityCommand,
	))
	admissibilityData, resultErr := admissibilityResult.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	admissibility := admissibilityData.(gitevents.AcquireRuntimeOperationAdmissibilityResult)
	operationName := gitcommon.OperationAdd
	operation := runtimecontract.RuntimeOperation{
		ID: "git-add-tree", Kind: runtimecontract.StepKindGitWrite, RequiredOperationRef: "stage-changes",
		TargetRefs: []string{"generated.txt"}, Parameters: []runtimecontract.OperationParameter{
			{Name: "operation", String: &operationName}, {Name: "pathspecs", StringList: []string{"generated.txt"}},
		},
	}
	observation := gitAdmissibilityObservation(admissibilityCommand.PrerequisiteID, admissibility.Acquisition.Admissibility, capability.Snapshot.WorkspaceTreeRevision)
	treeRevision = "file-tree-v2"
	probe := gitevents.ProbeRuntimeOperationFeasibilityCommand{
		WorkspaceID: capabilityCommand.WorkspaceID, RunID: capabilityCommand.RunID,
		Transaction:                   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: capabilityCommand.AuthoritySnapshotID},
		WorkspaceDeliverableDirectory: workspace, Operation: operation,
		Admissibility:          admissibility.Acquisition.Admissibility,
		CapabilityStateBinding: capability.Snapshot.AcquisitionBinding, Observation: &observation,
		CandidateManifestHash: "manifest-tree", OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, operation.Kind),
	}
	probeResult := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, probe,
	))
	probeData, resultErr := probeResult.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	probeResponse := probeData.(gitevents.ProbeRuntimeOperationFeasibilityResult)
	if probeResponse.Outcome != runtimecontract.RuntimeOwnerFeasibilityStale || probeResponse.Stale == nil ||
		probeResponse.Stale.Domain != runtimecontract.RuntimeResourceWorkspaceTree || probeResponse.Stale.ExpectedRevision != "file-tree-v1" || probeResponse.Stale.ObservedRevision != "file-tree-v2" {
		t.Fatalf("workspace-tree feasibility response = %+v", probeResponse)
	}

	sealed, err := gitevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	execute := gitevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-tree/run-tree/dispatch-tree/git-add-tree", Fence: 1,
				ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			},
			WorkspaceID: capabilityCommand.WorkspaceID, RunID: capabilityCommand.RunID, DispatchID: "dispatch-tree",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: capabilityCommand.AuthoritySnapshotID, RequiredOutputHash: "output-tree"},
			OperationID: operation.ID, EvidenceKind: runtimecontract.EvidenceKindStateChange,
			WorkspaceDeliverableDirectory: workspace, TargetRefs: append([]string(nil), operation.TargetRefs...),
		},
		SealedOperation: sealed, CapabilityStates: []runtimecontract.RuntimeCapabilityStateSnapshot{capability.Snapshot},
	}
	executeResult := sendGitStateChangingReceiptCommand(t, ctx, hub, execute)
	if executeResult.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || executeResult.Receipt.PrerequisiteStale == nil ||
		executeResult.Receipt.PrerequisiteStale.Domain != runtimecontract.RuntimeResourceWorkspaceTree {
		t.Fatalf("workspace-tree execution receipt = %+v", executeResult.Receipt)
	}
	assertGitIndexEmpty(t, workspace)
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
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"admissibility-git", ModuleID, []string{StepKindRead}, nil, nil,
		[]string{"repository-state-snapshot:" + snapshot.SnapshotID}, snapshot.ObservedRevision,
	)
	if err != nil {
		t.Fatal(err)
	}
	operationName := "status"
	command := gitevents.ProbeRuntimeOperationFeasibilityCommand{
		WorkspaceID: "ws-git", RunID: "run-git", WorkspaceDeliverableDirectory: workspace,
		Transaction:           runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-git"},
		Admissibility:         inventory,
		CandidateManifestHash: "manifest-git-status", OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, StepKindRead),
		Operation: runtimecontract.RuntimeOperation{ID: "status", Kind: StepKindRead, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &operationName}}},
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
