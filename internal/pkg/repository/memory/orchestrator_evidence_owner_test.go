package memory

import (
	"context"
	"errors"
	"reflect"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMemoryOrchestratorPurgeReceiptCountsOrphansAndPreservesSameRunOtherWorkspace(t *testing.T) {
	ctx := context.Background()
	owner, rawContinuations := newOrchestratorOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-memory-orchestrator-purge")
	otherWorkspaceID := common.WorkspaceID("ws-memory-orchestrator-keep")
	runID := common.RunID("run-001")
	targetCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	otherCtx := common.ContextWithRuntimeWorkspaceID(ctx, otherWorkspaceID)

	if err := owner.ToolCalls().Create(targetCtx, &orchestratormodel.ToolCall{ID: "tool-target", WorkspaceID: workspaceID, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Patches().Create(targetCtx, &orchestratormodel.Patch{ID: "patch-target", WorkspaceID: workspaceID, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := owner.ValidationResults().Create(targetCtx, &orchestratormodel.ValidationResult{ID: "validation-target", WorkspaceID: workspaceID, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := owner.EvidenceRecords().Create(targetCtx, evidenceRecordFixture("evidence-target", workspaceID, runID)); err != nil {
		t.Fatal(err)
	}
	if err := owner.CoverageWorkItems().Create(targetCtx, coverageFixture("coverage-target", workspaceID, runID)); err != nil {
		t.Fatal(err)
	}
	continuation := &runtimecontract.ArtifactContinuation{ID: "continuation-target", WorkspaceID: workspaceID, RunID: runID}
	if err := owner.ArtifactContinuations().Create(targetCtx, continuation); err != nil {
		t.Fatal(err)
	}
	if err := owner.ArtifactContinuations().SaveChunk(targetCtx, &runtimecontract.ArtifactContinuationChunk{ContinuationID: continuation.ID, Sequence: 1}, "chunk"); err != nil {
		t.Fatal(err)
	}
	if err := owner.ArtifactContinuations().SaveAssembled(targetCtx, &runtimecontract.ArtifactContinuationAssembledMetadata{ContinuationID: continuation.ID}, "assembled"); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash cut that lost the parent row after child resources were
	// durable. Workspace scope remains owner-private metadata and must drive the
	// exact purge independently of any Run/parent inventory.
	rawContinuations.mu.Lock()
	delete(rawContinuations.items, continuation.ID)
	rawContinuations.mu.Unlock()

	if err := owner.CoverageWorkItems().Create(otherCtx, coverageFixture("coverage-keep", otherWorkspaceID, runID)); err != nil {
		t.Fatal(err)
	}
	if err := owner.ArtifactContinuations().Create(otherCtx, &runtimecontract.ArtifactContinuation{ID: "continuation-keep", WorkspaceID: otherWorkspaceID, RunID: runID}); err != nil {
		t.Fatal(err)
	}

	scope := persistMemoryOrchestratorFence(t, owner, workspaceID, deletioncontract.ModePurge)
	receipt, err := owner.WorkspacePurger().PurgeWorkspaceEvidence(targetCtx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.ValidateFor(scope); err != nil {
		t.Fatalf("receipt invalid: %v", err)
	}
	observed := make(map[string]repository.OrchestratorWorkspaceEvidencePurgeObservation, len(receipt.Observations))
	for _, observation := range receipt.Observations {
		observed[observation.Surface] = observation
		if observation.Remaining != 0 || observation.Deleted != observation.ObservedBefore {
			t.Fatalf("incomplete purge observation: %+v", observation)
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
	if items, err := owner.CoverageWorkItems().ListByRun(otherCtx, runID); err != nil || len(items) != 1 || items[0].WorkspaceID != otherWorkspaceID {
		t.Fatalf("same RunID other-workspace coverage = %+v err=%v", items, err)
	}
	if items, err := owner.ArtifactContinuations().ListByRun(otherCtx, runID); err != nil || len(items) != 1 || items[0].WorkspaceID != otherWorkspaceID {
		t.Fatalf("same RunID other-workspace continuation = %+v err=%v", items, err)
	}
	if err := owner.ToolCalls().Create(targetCtx, &orchestratormodel.ToolCall{ID: "late", WorkspaceID: workspaceID, RunID: runID}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("late write after purge = %v, want not found", err)
	}
	// Simulate response loss between physical purge and the Orchestrator
	// aggregate's PurgeCompleted CAS. The backend set survives the Biz reopen,
	// so retry must replay the first non-empty owner receipt exactly.
	replayed, err := owner.WorkspacePurger().PurgeWorkspaceEvidence(targetCtx, scope)
	if err != nil || !reflect.DeepEqual(replayed, receipt) {
		t.Fatalf("purge response-loss replay invented a receipt: first=%+v replay=%+v err=%v", receipt, replayed, err)
	}
}

func TestMemoryOrchestratorRetainFenceHidesQueriesAndRejectsScopeSpoof(t *testing.T) {
	ctx := context.Background()
	owner := NewOrchestratorOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-memory-orchestrator-retain")
	runID := common.RunID("run-retain")
	bound := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)

	tool := &orchestratormodel.ToolCall{ID: "tool-retain", WorkspaceID: workspaceID, RunID: runID}
	patch := &orchestratormodel.Patch{ID: "patch-retain", WorkspaceID: workspaceID, RunID: runID}
	validation := &orchestratormodel.ValidationResult{ID: "validation-retain", WorkspaceID: workspaceID, RunID: runID}
	evidence := evidenceRecordFixture("evidence-retain", workspaceID, runID)
	coverage := coverageFixture("coverage-retain", workspaceID, runID)
	continuation := &runtimecontract.ArtifactContinuation{ID: "continuation-retain", WorkspaceID: workspaceID, RunID: runID}
	for _, create := range []func() error{
		func() error { return owner.ToolCalls().Create(bound, tool) },
		func() error { return owner.Patches().Create(bound, patch) },
		func() error { return owner.ValidationResults().Create(bound, validation) },
		func() error { return owner.EvidenceRecords().Create(bound, evidence) },
		func() error { return owner.CoverageWorkItems().Create(bound, coverage) },
		func() error { return owner.ArtifactContinuations().Create(bound, continuation) },
	} {
		if err := create(); err != nil {
			t.Fatal(err)
		}
	}
	spoofWorkspace := common.WorkspaceID("ws-spoof")
	for name, create := range map[string]func() error{
		"tool": func() error {
			return owner.ToolCalls().Create(bound, &orchestratormodel.ToolCall{ID: "tool-create-spoof", WorkspaceID: spoofWorkspace, RunID: runID})
		},
		"patch": func() error {
			return owner.Patches().Create(bound, &orchestratormodel.Patch{ID: "patch-create-spoof", WorkspaceID: spoofWorkspace, RunID: runID})
		},
		"validation": func() error {
			return owner.ValidationResults().Create(bound, &orchestratormodel.ValidationResult{ID: "validation-create-spoof", WorkspaceID: spoofWorkspace, RunID: runID})
		},
		"evidence": func() error {
			return owner.EvidenceRecords().Create(bound, evidenceRecordFixture("evidence-create-spoof", spoofWorkspace, runID))
		},
		"coverage": func() error {
			return owner.CoverageWorkItems().Create(bound, coverageFixture("coverage-create-spoof", spoofWorkspace, runID))
		},
		"continuation": func() error {
			return owner.ArtifactContinuations().Create(bound, &runtimecontract.ArtifactContinuation{ID: "continuation-create-spoof", WorkspaceID: spoofWorkspace, RunID: runID})
		},
	} {
		if err := create(); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("%s create scope spoof = %v, want not found", name, err)
		}
	}
	for name, mutate := range map[string]func() error{
		"tool": func() error {
			value := *tool
			value.RunID = "run-spoof"
			return owner.ToolCalls().Update(bound, &value)
		},
		"patch": func() error { value := *patch; value.RunID = "run-spoof"; return owner.Patches().Update(bound, &value) },
		"validation": func() error {
			value := *validation
			value.RunID = "run-spoof"
			return owner.ValidationResults().Update(bound, &value)
		},
		"evidence": func() error {
			value := *evidence
			value.RunID = "run-spoof"
			return owner.EvidenceRecords().Update(bound, &value)
		},
		"coverage": func() error {
			value := *coverage
			value.RunID = "run-spoof"
			return owner.CoverageWorkItems().Update(bound, &value)
		},
		"continuation": func() error {
			value := *continuation
			value.RunID = "run-spoof"
			return owner.ArtifactContinuations().Update(bound, &value)
		},
	} {
		if err := mutate(); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("%s immutable Run scope spoof = %v, want conflict", name, err)
		}
	}
	persistMemoryOrchestratorFence(t, owner, workspaceID, deletioncontract.ModeRetain)

	if _, err := owner.ToolCalls().Get(bound, tool.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("retained tool query = %v, want not found", err)
	}
	if _, err := owner.CoverageWorkItems().ListByRun(bound, runID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("retained coverage query = %v, want not found", err)
	}
	for name, mutate := range map[string]func() error{
		"tool": func() error {
			value := *tool
			value.WorkspaceID = spoofWorkspace
			return owner.ToolCalls().Update(bound, &value)
		},
		"patch": func() error {
			value := *patch
			value.WorkspaceID = spoofWorkspace
			return owner.Patches().Update(bound, &value)
		},
		"validation": func() error {
			value := *validation
			value.WorkspaceID = spoofWorkspace
			return owner.ValidationResults().Update(bound, &value)
		},
		"evidence": func() error {
			value := *evidence
			value.WorkspaceID = spoofWorkspace
			return owner.EvidenceRecords().Update(bound, &value)
		},
		"coverage": func() error {
			value := *coverage
			value.WorkspaceID = spoofWorkspace
			return owner.CoverageWorkItems().Update(bound, &value)
		},
		"continuation": func() error {
			value := *continuation
			value.WorkspaceID = spoofWorkspace
			return owner.ArtifactContinuations().Update(bound, &value)
		},
	} {
		if err := mutate(); !errors.Is(err, common.ErrNotFound) && !errors.Is(err, common.ErrConflict) {
			t.Fatalf("%s scope spoof = %v, want fail closed", name, err)
		}
	}
}

func persistMemoryOrchestratorFence(t *testing.T, owner repository.OrchestratorEvidenceOwnerRepositories, workspaceID common.WorkspaceID, mode deletioncontract.Mode) deletioncontract.Scope {
	t.Helper()
	intent := deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: workspaceID, DeletionID: "delete-" + string(workspaceID), DeletionGeneration: 1,
		DataDispositionIntent: mode, WorkspaceIntentHash: "workspace-intent-hash", RunDrainingRevision: 1,
	}
	now := "2026-08-26T00:00:00Z"
	quiescing := &orchestratormodel.WorkspaceDeletion{
		WorkspaceID: workspaceID, DeletionID: intent.DeletionID, DeletionGeneration: 1, Mode: mode,
		WorkspaceIntentHash: intent.WorkspaceIntentHash, RunDrainingRevision: 1,
		QuiescingGateID: orchestratorevents.StableWorkspaceRuntimeQuiescingGateID(intent),
		Phase:           orchestratormodel.WorkspaceDeletionPhaseQuiescing, Revision: 1, RequestedAt: now, UpdatedAt: now,
	}
	if err := owner.WorkspaceDeletions().Create(context.Background(), quiescing); err != nil {
		t.Fatal(err)
	}
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

func evidenceRecordFixture(id common.EvidenceRecordID, workspaceID common.WorkspaceID, runID common.RunID) *runtimecontract.EvidenceRecord {
	return &runtimecontract.EvidenceRecord{ID: id, WorkspaceID: workspaceID, RunID: runID, Kind: runtimecontract.EvidenceKindValidation, Status: runtimecontract.EvidenceStatusAccepted, SourceRef: "source"}
}

func coverageFixture(id common.CoverageWorkItemID, workspaceID common.WorkspaceID, runID common.RunID) *runtimecontract.CoverageWorkItem {
	return &runtimecontract.CoverageWorkItem{ID: id, WorkspaceID: workspaceID, RunID: runID, DefinitionRevision: "definition", NormalizedSourceKey: "source-" + string(id), Status: "open"}
}
