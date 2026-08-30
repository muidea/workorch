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
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGitOwnerSemanticProofEventHubReportsFeasibleThenStale(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	workspace := newTestRepo(t)

	admissibilityRequest := gitevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-git-semantic", RunID: "run-git-semantic", AuthoritySnapshotID: "authority-git-semantic",
		PrerequisiteID: "admissibility-git-semantic", OwnerModuleID: ModuleID,
		WorkspaceDeliverableDirectory: workspace, RequiredKinds: []string{StepKindWrite}, MaxItems: 8,
	}
	admissibilityData, err := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, admissibilityRequest,
	)).Get()
	if err != nil {
		t.Fatal(err)
	}
	acquisition := admissibilityData.(gitevents.AcquireRuntimeOperationAdmissibilityResult).Acquisition
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-git-semantic", OwnerModuleID: ModuleID, OperationKind: StepKindWrite, OperationVariantID: "add",
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{gitWorkspaceRepositorySelector},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef: "dod/cwi-git-semantic",
	}
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-git-semantic", RunID: "run-git-semantic", DefinitionRevision: "definition-git-semantic", DefinitionSeedHash: "seed-git-semantic",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{
			ID: "cwi-git-semantic", SliceID: "slice-git-semantic", SourceScope: "dodItem", SourceID: "dod-git-semantic", TargetText: "inspect repository state", Order: 1,
		}},
		AllowedOwnerModuleIDs: []string{ModuleID}, AllowedOperationKinds: []string{StepKindWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID: "cwi-git-semantic", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		}},
		OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{acquisition.Admissibility},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	command := gitevents.ProbeRuntimeSemanticProofFeasibilityCommand(runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: manifest.WorkspaceID, RunID: manifest.RunID, FeasibilityBatchID: "batch-git-semantic",
		WorkspaceDeliverableDirectory: workspace, Manifest: manifest, Proposal: proposal,
		OwnerCatalog: acquisition.Admissibility,
	})
	send := func() gitevents.ProbeRuntimeSemanticProofFeasibilityResult {
		t.Helper()
		data, resultErr := hub.Send(magicevent.NewEventWithContext(
			gitevents.TopicProbeRuntimeSemanticProofFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
			magicevent.NewHeader(), ctx, command,
		)).Get()
		if resultErr != nil {
			t.Fatal(resultErr)
		}
		return data.(gitevents.ProbeRuntimeSemanticProofFeasibilityResult)
	}
	feasible := send()
	if feasible.Observation.Status != runtimecontract.RuntimeSemanticFeasibilityFeasible || feasible.Observation.OriginalDiagnostic != nil {
		t.Fatalf("current Git semantic feasibility = %+v", feasible)
	}
	if err := os.WriteFile(filepath.Join(workspace, "semantic-drift.txt"), []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := send()
	if stale.Observation.Status != runtimecontract.RuntimeSemanticFeasibilityStale || stale.Observation.OriginalDiagnostic == nil ||
		stale.Observation.OriginalDiagnostic.OriginalCode != "semantic_catalog_stale" || stale.Observation.ObservedRevision == acquisition.Admissibility.ObservedRevision {
		t.Fatalf("drifted Git semantic feasibility = %+v", stale)
	}
}

func TestGitOwnerReadReceiptReplaysAfterOwnerRecreation(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
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
		receipts[command.Receipt.ExecutionKey] = command.Receipt.Clone()
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	workspace := newTestRepo(t)
	operationName := gitcommon.OperationStatus
	operation := runtimecontract.RuntimeOperation{
		ID: "git-status", Kind: runtimecontract.StepKindGitRead,
		Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &operationName}},
	}
	sealed, err := gitevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	command := gitevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: gitTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-git/run-git/dispatch-git/git-status", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
			}),
			WorkspaceID: "ws-git", RunID: "run-git", DispatchID: "dispatch-git",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-git", RequiredOutputHash: "output-git"},
			OperationID: "git-status", EvidenceKind: runtimecontract.EvidenceKindDiagnostic,
			WorkspaceDeliverableDirectory: workspace,
		},
		SealedOperation: sealed,
	}
	send := func() gitevents.ExecuteOperationResult {
		t.Helper()
		data, sendErr := hub.Send(magicevent.NewEventWithContext(
			gitevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
			magicevent.NewHeader(), ctx, command,
		)).Get()
		if sendErr != nil {
			t.Fatal(sendErr)
		}
		return data.(gitevents.ExecuteOperationResult)
	}
	first := send()
	if first.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || first.Receipt.Changed || first.Receipt.ObservedEffectScope != runtimecontract.RuntimeEffectNone || first.Receipt.Evidence.ContentHash == "" {
		t.Fatalf("Git read receipt = %+v", first.Receipt)
	}
	// Make a second git status materially different. A recreated owner must query
	// the persisted receipt before touching the repository; otherwise the changed
	// status output would produce a different evidence hash.
	if err := os.WriteFile(filepath.Join(workspace, "receipt-replay-drift.txt"), []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := exec.Command("git", "-C", workspace, "status", "--porcelain").Output()
	if err != nil || len(status) == 0 {
		t.Fatalf("prepare observable git status drift: output=%q err=%v", status, err)
	}
	owner.Teardown(ctx)
	restarted := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := restarted.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { restarted.Teardown(ctx) })
	replayed := send()
	if !runtimecontract.SameRuntimeOperationReceipt(first.Receipt, replayed.Receipt) {
		t.Fatalf("fresh Git owner read replay drifted: first=%+v replayed=%+v", first.Receipt, replayed.Receipt)
	}
}
