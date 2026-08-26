package workorchrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileOrchestratorPurgeCountsOrphanChildrenAndPreservesSameRunOtherWorkspace(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	repositories := NewWorkspaceScopedFileRepositorySet(dataRoot)
	workspaceID := common.WorkspaceID("ws-file-orchestrator-purge")
	otherWorkspaceID := common.WorkspaceID("ws-file-orchestrator-keep")
	runID := common.RunID("run-001")
	targetCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	otherCtx := common.ContextWithRuntimeWorkspaceID(ctx, otherWorkspaceID)
	for _, currentWorkspaceID := range []common.WorkspaceID{workspaceID, otherWorkspaceID} {
		if err := repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: currentWorkspaceID, State: common.RunStateCompleted}); err != nil {
			t.Fatal(err)
		}
	}

	if err := repositories.ToolCalls.Create(targetCtx, &orchestratormodel.ToolCall{ID: "tool-target", WorkspaceID: workspaceID, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := repositories.Patches.Create(targetCtx, &orchestratormodel.Patch{ID: "patch-target", WorkspaceID: workspaceID, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := repositories.ValidationResults.Create(targetCtx, &orchestratormodel.ValidationResult{ID: "validation-target", WorkspaceID: workspaceID, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := repositories.EvidenceRecords.Create(targetCtx, fileOrchestratorEvidenceFixture("evidence-target", workspaceID, runID)); err != nil {
		t.Fatal(err)
	}
	if err := repositories.CoverageWorkItems.Create(targetCtx, fileOrchestratorCoverageFixture("coverage-target", workspaceID, runID)); err != nil {
		t.Fatal(err)
	}
	continuation := &runtimecontract.ArtifactContinuation{ID: "continuation-target", WorkspaceID: workspaceID, RunID: runID}
	if err := repositories.ArtifactContinuations.Create(targetCtx, continuation); err != nil {
		t.Fatal(err)
	}
	if err := repositories.ArtifactContinuations.SaveChunk(targetCtx, &runtimecontract.ArtifactContinuationChunk{ContinuationID: continuation.ID, Sequence: 1}, "chunk"); err != nil {
		t.Fatal(err)
	}
	if err := repositories.ArtifactContinuations.SaveAssembled(targetCtx, &runtimecontract.ArtifactContinuationAssembledMetadata{ContinuationID: continuation.ID}, "assembled"); err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(dataRoot, "internal", "workspaces", string(workspaceID), "runs", string(runID), workspaceScopedArtifactContinuationDir, string(continuation.ID), "metadata.json")
	if err := os.Remove(metadataPath); err != nil {
		t.Fatal(err)
	}

	if err := repositories.CoverageWorkItems.Create(otherCtx, fileOrchestratorCoverageFixture("coverage-keep", otherWorkspaceID, runID)); err != nil {
		t.Fatal(err)
	}
	if err := repositories.ArtifactContinuations.Create(otherCtx, &runtimecontract.ArtifactContinuation{ID: "continuation-keep", WorkspaceID: otherWorkspaceID, RunID: runID}); err != nil {
		t.Fatal(err)
	}

	scope := persistFileOrchestratorFence(t, repositories.OrchestratorEvidence, workspaceID, deletioncontract.ModePurge)
	receipt, err := repositories.OrchestratorPurger.PurgeWorkspaceEvidence(targetCtx, scope)
	if err != nil {
		t.Fatal(err)
	}
	observed := make(map[string]repository.OrchestratorWorkspaceEvidencePurgeObservation, len(receipt.Observations))
	for _, observation := range receipt.Observations {
		observed[observation.Surface] = observation
		if observation.Remaining != 0 || observation.Deleted != observation.ObservedBefore {
			t.Fatalf("incomplete observation: %+v", observation)
		}
	}
	for _, surface := range []string{
		repository.OrchestratorPurgeSurfaceToolCalls,
		repository.OrchestratorPurgeSurfacePatches,
		repository.OrchestratorPurgeSurfaceValidationResults,
		repository.OrchestratorPurgeSurfaceEvidenceRecords,
		repository.OrchestratorPurgeSurfaceCoverageWorkItems,
		repository.OrchestratorPurgeSurfaceArtifactChunkMetadata,
		repository.OrchestratorPurgeSurfaceArtifactChunkContent,
		repository.OrchestratorPurgeSurfaceArtifactAssembledMetadata,
		repository.OrchestratorPurgeSurfaceArtifactAssembledContent,
	} {
		if observed[surface].ObservedBefore != 1 {
			t.Fatalf("surface %s observed before = %d, want 1", surface, observed[surface].ObservedBefore)
		}
	}
	if observed[repository.OrchestratorPurgeSurfaceArtifactContinuations].ObservedBefore != 0 {
		t.Fatalf("orphan parent count = %+v", observed[repository.OrchestratorPurgeSurfaceArtifactContinuations])
	}
	if items, err := repositories.CoverageWorkItems.ListByRun(otherCtx, runID); err != nil || len(items) != 1 || items[0].WorkspaceID != otherWorkspaceID {
		t.Fatalf("other workspace coverage = %+v err=%v", items, err)
	}
	if items, err := repositories.ArtifactContinuations.ListByRun(otherCtx, runID); err != nil || len(items) != 1 || items[0].WorkspaceID != otherWorkspaceID {
		t.Fatalf("other workspace continuation = %+v err=%v", items, err)
	}
	reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
	if _, err := reopened.OrchestratorDeletions.Get(ctx, workspaceID); err != nil {
		t.Fatalf("reopen tombstone: %v", err)
	}
	if err := reopened.ToolCalls.Create(targetCtx, &orchestratormodel.ToolCall{ID: "late", WorkspaceID: workspaceID, RunID: runID}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("late write after reopen = %v, want not found", err)
	}
	replayed, err := reopened.OrchestratorPurger.PurgeWorkspaceEvidence(targetCtx, scope)
	if err != nil || !reflect.DeepEqual(replayed, receipt) {
		t.Fatalf("reopened purge replay invented a receipt: first=%+v replay=%+v err=%v", receipt, replayed, err)
	}
}

func TestFileOrchestratorPendingPurgeJournalSurvivesPhysicalDeleteCrash(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	first := NewWorkspaceScopedFileRepositorySet(dataRoot)
	workspaceID := common.WorkspaceID("ws-file-orchestrator-purge-crash")
	runID := common.RunID("run-001")
	bound := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	if err := first.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatal(err)
	}
	if err := first.ToolCalls.Create(bound, &orchestratormodel.ToolCall{ID: "tool-before-crash", WorkspaceID: workspaceID, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	scope := persistFileOrchestratorFence(t, first.OrchestratorEvidence, workspaceID, deletioncontract.ModePurge)
	injected := errors.New("injected crash after physical purge")
	purger, ok := first.OrchestratorPurger.(*workspaceScopedOrchestratorEvidencePurger)
	if !ok {
		t.Fatalf("unexpected purger type %T", first.OrchestratorPurger)
	}
	purger.afterPhysicalPurge = func() error { return injected }
	if _, err := first.OrchestratorPurger.PurgeWorkspaceEvidence(bound, scope); !errors.Is(err, injected) {
		t.Fatalf("physical-delete crash error=%v, want injected", err)
	}
	if item, err := first.OrchestratorDeletions.Get(ctx, workspaceID); err != nil || item.Phase != orchestratormodel.WorkspaceDeletionPhaseFenced {
		t.Fatalf("crash advanced deletion aggregate: item=%+v err=%v", item, err)
	}

	// A new process sees the pending journal written before removal. It must
	// finish the idempotent sweep and return the original non-empty proof.
	reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
	receipt, err := reopened.OrchestratorPurger.PurgeWorkspaceEvidence(bound, scope)
	if err != nil || receipt.ValidateFor(scope) != nil {
		t.Fatalf("reopen pending purge receipt=%+v err=%v", receipt, err)
	}
	if receipt.Observations[0].Surface != repository.OrchestratorPurgeSurfaceToolCalls || receipt.Observations[0].ObservedBefore != 1 {
		t.Fatalf("reopen lost pre-delete observation: %+v", receipt.Observations)
	}
	third := NewWorkspaceScopedFileRepositorySet(dataRoot)
	replayed, err := third.OrchestratorPurger.PurgeWorkspaceEvidence(bound, scope)
	if err != nil || !reflect.DeepEqual(replayed, receipt) {
		t.Fatalf("completed journal replay invented a receipt: first=%+v replay=%+v err=%v", receipt, replayed, err)
	}
}

func TestFileOrchestratorFenceSerializesIndependentRepositorySets(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	first := NewWorkspaceScopedFileRepositorySet(dataRoot)
	second := NewWorkspaceScopedFileRepositorySet(dataRoot)
	workspaceID := common.WorkspaceID("ws-file-orchestrator-race")
	bound := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	quiescing := fileOrchestratorQuiescingFixture(workspaceID, deletioncontract.ModeRetain)

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		results <- first.ToolCalls.Create(bound, &orchestratormodel.ToolCall{ID: "tool-race", WorkspaceID: workspaceID, RunID: "run-race"})
	}()
	go func() {
		<-start
		results <- second.OrchestratorDeletions.Create(ctx, quiescing)
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil && !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("race result = %v", err)
		}
	}
	reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
	if _, err := reopened.ToolCalls.ListByRun(bound, "run-race"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("post-fence query = %v, want not found", err)
	}
	if err := reopened.ToolCalls.Create(bound, &orchestratormodel.ToolCall{ID: "tool-late", WorkspaceID: workspaceID, RunID: "run-race"}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("post-fence write = %v, want not found", err)
	}
}

func persistFileOrchestratorFence(t *testing.T, owner repository.OrchestratorEvidenceOwnerRepositories, workspaceID common.WorkspaceID, mode deletioncontract.Mode) deletioncontract.Scope {
	t.Helper()
	quiescing := fileOrchestratorQuiescingFixture(workspaceID, mode)
	if err := owner.WorkspaceDeletions().Create(context.Background(), quiescing); err != nil {
		t.Fatal(err)
	}
	intent := quiescing.QuiescenceIntent()
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: workspaceID, DeletionID: intent.DeletionID, DeletionGeneration: 1, DataDispositionIntent: mode,
		WorkspaceIntentHash: intent.WorkspaceIntentHash, RunDrainingRevision: 1, OrchestratorQuiescenceRevision: 2,
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	quiesced := orchestratormodel.CloneWorkspaceDeletion(quiescing)
	quiesced.Phase, quiesced.Revision = orchestratormodel.WorkspaceDeletionPhaseQuiesced, 2
	quiesced.QuiescenceToken, quiesced.QuiescenceID = &token, token.QuiescenceID
	quiesced.RequestHash = deletioncontract.StableRequestHash(quiesced.Scope())
	if err := owner.WorkspaceDeletions().CompareAndSwap(context.Background(), quiesced, 1); err != nil {
		t.Fatal(err)
	}
	fenced := orchestratormodel.CloneWorkspaceDeletion(quiesced)
	fenced.Phase, fenced.Revision = orchestratormodel.WorkspaceDeletionPhaseFenced, 3
	fenced.FenceID = orchestratorevents.StableOrchestratorDeletionFenceID(fenced.Scope())
	fenced.SealAckHash = orchestratorevents.StableOrchestratorDeletionSealAckHash(fenced.Scope())
	if err := owner.WorkspaceDeletions().CompareAndSwap(context.Background(), fenced, 2); err != nil {
		t.Fatal(err)
	}
	return fenced.Scope()
}

func fileOrchestratorQuiescingFixture(workspaceID common.WorkspaceID, mode deletioncontract.Mode) *orchestratormodel.WorkspaceDeletion {
	intent := deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: workspaceID, DeletionID: "delete-" + string(workspaceID), DeletionGeneration: 1,
		DataDispositionIntent: mode, WorkspaceIntentHash: "workspace-intent-hash", RunDrainingRevision: 1,
	}
	return &orchestratormodel.WorkspaceDeletion{
		WorkspaceID: workspaceID, DeletionID: intent.DeletionID, DeletionGeneration: 1, Mode: mode,
		WorkspaceIntentHash: intent.WorkspaceIntentHash, RunDrainingRevision: 1,
		QuiescingGateID: orchestratorevents.StableWorkspaceRuntimeQuiescingGateID(intent),
		Phase:           orchestratormodel.WorkspaceDeletionPhaseQuiescing, Revision: 1,
		RequestedAt: "2026-08-26T00:00:00Z", UpdatedAt: "2026-08-26T00:00:00Z",
	}
}

func fileOrchestratorEvidenceFixture(id common.EvidenceRecordID, workspaceID common.WorkspaceID, runID common.RunID) *runtimecontract.EvidenceRecord {
	return &runtimecontract.EvidenceRecord{ID: id, WorkspaceID: workspaceID, RunID: runID, Kind: runtimecontract.EvidenceKindValidation, Status: runtimecontract.EvidenceStatusAccepted, SourceRef: "source"}
}

func fileOrchestratorCoverageFixture(id common.CoverageWorkItemID, workspaceID common.WorkspaceID, runID common.RunID) *runtimecontract.CoverageWorkItem {
	return &runtimecontract.CoverageWorkItem{ID: id, WorkspaceID: workspaceID, RunID: runID, DefinitionRevision: "definition", NormalizedSourceKey: "source-" + string(id), Status: "open"}
}
