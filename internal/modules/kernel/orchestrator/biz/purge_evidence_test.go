package orchestrator

// Owner Biz purge tests.

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	memorymodule "github.com/muidea/workorch/internal/modules/blocks/memory"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPurgeRunEvidenceDeletesOnlyRequestedRunEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-purge-evidence")
	runID := common.RunID("run-purge-evidence")
	otherRunID := common.RunID("run-keep-evidence")
	toolCalls := memoryrepo.NewToolCallRepository()
	patches := memoryrepo.NewPatchRepository()
	validations := memoryrepo.NewValidationResultRepository()
	evidenceRecords := memoryrepo.NewEvidenceRecordRepository()
	coverageWorkItems := memoryrepo.NewCoverageWorkItemRepository()
	continuations := memoryrepo.NewArtifactContinuationRepository()
	traces := memoryrepo.NewMemoryRecallTraceRepository()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(toolCalls.Create(ctx, &runmodel.ToolCall{ID: "tool-purge", RunID: runID}))
	must(toolCalls.Create(ctx, &runmodel.ToolCall{ID: "tool-keep", RunID: otherRunID}))
	must(patches.Create(ctx, &runmodel.Patch{ID: "patch-purge", RunID: runID}))
	must(patches.Create(ctx, &runmodel.Patch{ID: "patch-keep", RunID: otherRunID}))
	must(validations.Create(ctx, &runmodel.ValidationResult{ID: "validation-purge", RunID: runID}))
	must(validations.Create(ctx, &runmodel.ValidationResult{ID: "validation-keep", RunID: otherRunID}))
	must(evidenceRecords.Create(ctx, &orchestratormodel.EvidenceRecord{ID: "evidence-purge", WorkspaceID: workspaceID, RunID: runID, Kind: orchestratormodel.EvidenceKindValidation, Status: orchestratormodel.EvidenceStatusAccepted, SourceRef: "validation-purge"}))
	must(evidenceRecords.Create(ctx, &orchestratormodel.EvidenceRecord{ID: "evidence-keep", WorkspaceID: workspaceID, RunID: otherRunID, Kind: orchestratormodel.EvidenceKindValidation, Status: orchestratormodel.EvidenceStatusAccepted, SourceRef: "validation-keep"}))
	must(coverageWorkItems.Create(ctx, &orchestratormodel.CoverageWorkItem{ID: "cwi-purge", WorkspaceID: workspaceID, RunID: runID, DefinitionRevision: "rev-purge", NormalizedSourceKey: "source-purge", Status: "open"}))
	must(coverageWorkItems.Create(ctx, &orchestratormodel.CoverageWorkItem{ID: "cwi-keep", WorkspaceID: workspaceID, RunID: otherRunID, DefinitionRevision: "rev-keep", NormalizedSourceKey: "source-keep", Status: "open"}))
	must(continuations.Create(ctx, &orchestratormodel.ArtifactContinuation{ID: "continuation-purge", WorkspaceID: workspaceID, RunID: runID}))
	must(continuations.Create(ctx, &orchestratormodel.ArtifactContinuation{ID: "continuation-keep", WorkspaceID: workspaceID, RunID: otherRunID}))
	must(traces.Create(ctx, &memorymodel.MemoryRecallTrace{ID: "trace-purge", WorkspaceID: workspaceID, RunID: runID}))
	must(traces.Create(ctx, &memorymodel.MemoryRecallTrace{ID: "trace-keep", WorkspaceID: workspaceID, RunID: otherRunID}))

	deleted, err := purgeRunEvidence(ctx, RuntimeEvidenceRepositories{
		ToolCalls:             toolCalls,
		Patches:               patches,
		ValidationResults:     validations,
		EvidenceRecords:       evidenceRecords,
		CoverageWorkItems:     coverageWorkItems,
		ArtifactContinuations: continuations,
		MemoryRecallTraces:    traces,
	}, runID)
	if err != nil {
		t.Fatalf("purge run evidence: %v", err)
	}
	if deleted != 7 {
		t.Fatalf("deleted = %d, want 7", deleted)
	}
	assertRunEvidenceCount(t, ctx, toolCalls, runID, 0)
	assertRunEvidenceCount(t, ctx, toolCalls, otherRunID, 1)
	assertRunEvidenceCount(t, ctx, patches, runID, 0)
	assertRunEvidenceCount(t, ctx, patches, otherRunID, 1)
	assertRunEvidenceCount(t, ctx, validations, runID, 0)
	assertRunEvidenceCount(t, ctx, validations, otherRunID, 1)
	assertRunEvidenceCount(t, ctx, evidenceRecords, runID, 0)
	assertRunEvidenceCount(t, ctx, evidenceRecords, otherRunID, 1)
	assertRunEvidenceCount(t, ctx, coverageWorkItems, runID, 0)
	assertRunEvidenceCount(t, ctx, coverageWorkItems, otherRunID, 1)
	assertRunEvidenceCount(t, ctx, continuations, runID, 0)
	assertRunEvidenceCount(t, ctx, continuations, otherRunID, 1)
	traceItems, err := traces.List(ctx, repository.MemoryRecallTraceQuery{RunID: runID})
	if err != nil {
		t.Fatalf("list purged traces: %v", err)
	}
	if len(traceItems) != 0 {
		t.Fatalf("purged trace count = %d, want 0", len(traceItems))
	}
	traceItems, err = traces.List(ctx, repository.MemoryRecallTraceQuery{RunID: otherRunID})
	if err != nil {
		t.Fatalf("list kept traces: %v", err)
	}
	if len(traceItems) != 1 {
		t.Fatalf("kept trace count = %d, want 1", len(traceItems))
	}
}

type runEvidenceLister[T any] interface {
	ListByRun(context.Context, common.RunID) ([]*T, error)
}

func assertRunEvidenceCount[T any](t *testing.T, ctx context.Context, repo runEvidenceLister[T], runID common.RunID, want int) {
	t.Helper()
	items, err := repo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list %s evidence: %v", runID, err)
	}
	if len(items) != want {
		t.Fatalf("%s evidence count = %d, want %d", runID, len(items), want)
	}
}

func TestPurgeRunCommandAcceptsContractPayload(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	workspaceID := common.WorkspaceID("ws-purge-contract")
	steps := memoryrepo.NewStepRepository()
	checkpoints := memoryrepo.NewRuntimeCheckpointRepository()
	runs := memoryrepo.NewRunRepository()
	runID := common.RunID("run-purge-contract")
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	if err := runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := steps.Create(ctx, &runmodel.Step{ID: "step-purge-contract", RunID: runID}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-purge-contract", RunID: runID}); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, Steps: steps, Checkpoints: checkpoints, EventHub: rawHub}); err != nil {
		t.Fatalf("configure run owner: %v", err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatalf("start run owner: %v", err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(context.Background()) })
	memoryModule := memorymodule.New()
	if err := memoryModule.Configure(memorymodule.Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(),
		EventHub:           rawHub,
	}); err != nil {
		t.Fatalf("configure memory module: %v", err)
	}
	if err := memoryModule.Start(); err != nil {
		t.Fatalf("start memory module: %v", err)
	}
	t.Cleanup(func() {
		if err := memoryModule.Stop(context.Background()); err != nil {
			t.Errorf("stop memory module: %v", err)
		}
	})

	module := New()
	if err := module.Configure(Dependencies{
		ToolCalls:             memoryrepo.NewToolCallRepository(),
		Patches:               memoryrepo.NewPatchRepository(),
		ValidationResults:     memoryrepo.NewValidationResultRepository(),
		EvidenceRecords:       memoryrepo.NewEvidenceRecordRepository(),
		CoverageWorkItems:     memoryrepo.NewCoverageWorkItemRepository(),
		ArtifactContinuations: memoryrepo.NewArtifactContinuationRepository(),
		CommandHub:            rawHub,
	}); err != nil {
		t.Fatalf("configure orchestrator module: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("start orchestrator module: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("stop orchestrator module: %v", err)
		}
	})

	result := rawHub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandPurgeRun,
		"test/orchestrator",
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		orchestratorevents.PurgeRunCommand{WorkspaceID: workspaceID, RunID: runID},
	))
	if result == nil {
		t.Fatal("purge result is nil")
	}
	if result.Error() != nil {
		t.Fatalf("purge err = %v", result.Error())
	}
	items, err := steps.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("steps after purge = %d, want 0", len(items))
	}
	checkpointItems, err := checkpoints.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(checkpointItems) != 0 {
		t.Fatalf("checkpoints after purge = %d, want 0", len(checkpointItems))
	}
}

func TestPurgeRunCommandUsesWorkspaceScopeForDuplicateRunIDs(t *testing.T) {
	ctx := context.Background()
	steps := memoryrepo.NewStepRepository()
	checkpoints := memoryrepo.NewRuntimeCheckpointRepository()
	runs := memoryrepo.NewRunRepository()
	runID := common.RunID("run-duplicate-purge")
	workspaceA := common.WorkspaceID("ws-purge-a")
	workspaceB := common.WorkspaceID("ws-purge-b")
	stepA := common.StepID("step-duplicate-purge-a")
	stepB := common.StepID("step-duplicate-purge-b")
	if err := runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceA, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("seed run A: %v", err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceB, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("seed run B: %v", err)
	}
	if err := steps.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceA), &runmodel.Step{ID: stepA, RunID: runID}); err != nil {
		t.Fatalf("seed step A: %v", err)
	}
	if err := steps.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceB), &runmodel.Step{ID: stepB, RunID: runID}); err != nil {
		t.Fatalf("seed step B: %v", err)
	}
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, Steps: steps, Checkpoints: checkpoints, EventHub: rawHub}); err != nil {
		t.Fatalf("configure run owner: %v", err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatalf("start run owner: %v", err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(context.Background()) })
	memoryModule := memorymodule.New()
	if err := memoryModule.Configure(memorymodule.Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(),
		EventHub:           rawHub,
	}); err != nil {
		t.Fatalf("configure memory module: %v", err)
	}
	if err := memoryModule.Start(); err != nil {
		t.Fatalf("start memory module: %v", err)
	}
	t.Cleanup(func() {
		if err := memoryModule.Stop(context.Background()); err != nil {
			t.Errorf("stop memory module: %v", err)
		}
	})
	module := New()
	if err := module.Configure(Dependencies{
		ToolCalls:             memoryrepo.NewToolCallRepository(),
		Patches:               memoryrepo.NewPatchRepository(),
		ValidationResults:     memoryrepo.NewValidationResultRepository(),
		EvidenceRecords:       memoryrepo.NewEvidenceRecordRepository(),
		CoverageWorkItems:     memoryrepo.NewCoverageWorkItemRepository(),
		ArtifactContinuations: memoryrepo.NewArtifactContinuationRepository(),
		CommandHub:            rawHub,
	}); err != nil {
		t.Fatalf("configure orchestrator module: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("start orchestrator module: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("stop orchestrator module: %v", err)
		}
	})

	result := rawHub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandPurgeRun,
		"test/orchestrator",
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		common.ContextWithRuntimeWorkspaceID(ctx, workspaceA),
		orchestratorevents.PurgeRunCommand{WorkspaceID: workspaceA, RunID: runID},
	))
	if result == nil {
		t.Fatal("purge result is nil")
	}
	if result.Error() != nil {
		t.Fatalf("purge err = %v", result.Error())
	}
	itemsA, err := steps.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceA), runID)
	if err != nil {
		t.Fatalf("list steps A: %v", err)
	}
	if len(itemsA) != 0 {
		t.Fatalf("workspace A steps after purge = %d, want 0", len(itemsA))
	}
	itemsB, err := steps.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceB), runID)
	if err != nil {
		t.Fatalf("list steps B: %v", err)
	}
	if len(itemsB) != 1 || itemsB[0].ID != stepB {
		t.Fatalf("workspace B steps after purge = %+v, want untouched step %s", itemsB, stepB)
	}
}
