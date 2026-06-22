package orchestrator

import (
	"context"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	memorymodule "github.com/muidea/workorch/internal/modules/kernel/memory"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPurgeRunEvidenceDeletesOnlyRequestedRunEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-purge-evidence")
	runID := common.RunID("run-purge-evidence")
	otherRunID := common.RunID("run-keep-evidence")
	steps := memoryrepo.NewStepRepository()
	toolCalls := memoryrepo.NewToolCallRepository()
	patches := memoryrepo.NewPatchRepository()
	validations := memoryrepo.NewValidationResultRepository()
	checkpoints := memoryrepo.NewRuntimeCheckpointRepository()
	coverageWorkItems := memoryrepo.NewCoverageWorkItemRepository()
	continuations := memoryrepo.NewArtifactContinuationRepository()
	traces := memoryrepo.NewMemoryRecallTraceRepository()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(steps.Create(ctx, &runmodel.Step{ID: "step-purge", RunID: runID}))
	must(steps.Create(ctx, &runmodel.Step{ID: "step-keep", RunID: otherRunID}))
	must(toolCalls.Create(ctx, &runmodel.ToolCall{ID: "tool-purge", RunID: runID}))
	must(toolCalls.Create(ctx, &runmodel.ToolCall{ID: "tool-keep", RunID: otherRunID}))
	must(patches.Create(ctx, &runmodel.Patch{ID: "patch-purge", RunID: runID}))
	must(patches.Create(ctx, &runmodel.Patch{ID: "patch-keep", RunID: otherRunID}))
	must(validations.Create(ctx, &runmodel.ValidationResult{ID: "validation-purge", RunID: runID}))
	must(validations.Create(ctx, &runmodel.ValidationResult{ID: "validation-keep", RunID: otherRunID}))
	must(checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-purge", RunID: runID}))
	must(checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-keep", RunID: otherRunID}))
	must(coverageWorkItems.Create(ctx, &orchestratormodel.CoverageWorkItem{ID: "cwi-purge", WorkspaceID: workspaceID, RunID: runID, DefinitionRevision: "rev-purge", NormalizedSourceKey: "source-purge", Status: "open"}))
	must(coverageWorkItems.Create(ctx, &orchestratormodel.CoverageWorkItem{ID: "cwi-keep", WorkspaceID: workspaceID, RunID: otherRunID, DefinitionRevision: "rev-keep", NormalizedSourceKey: "source-keep", Status: "open"}))
	must(continuations.Create(ctx, &orchestratormodel.ArtifactContinuation{ID: "continuation-purge", WorkspaceID: workspaceID, RunID: runID}))
	must(continuations.Create(ctx, &orchestratormodel.ArtifactContinuation{ID: "continuation-keep", WorkspaceID: workspaceID, RunID: otherRunID}))
	must(traces.Create(ctx, &memorymodel.MemoryRecallTrace{ID: "trace-purge", WorkspaceID: workspaceID, RunID: runID}))
	must(traces.Create(ctx, &memorymodel.MemoryRecallTrace{ID: "trace-keep", WorkspaceID: workspaceID, RunID: otherRunID}))

	deleted, err := purgeRunEvidence(ctx, steps, orchestratorbiz.RuntimeEvidenceRepositories{
		ToolCalls:             toolCalls,
		Patches:               patches,
		ValidationResults:     validations,
		RuntimeCheckpoints:    checkpoints,
		CoverageWorkItems:     coverageWorkItems,
		ArtifactContinuations: continuations,
		MemoryRecallTraces:    traces,
	}, runID)
	if err != nil {
		t.Fatalf("purge run evidence: %v", err)
	}
	if deleted != 8 {
		t.Fatalf("deleted = %d, want 8", deleted)
	}
	assertRunEvidenceCount(t, ctx, steps, runID, 0)
	assertRunEvidenceCount(t, ctx, steps, otherRunID, 1)
	assertRunEvidenceCount(t, ctx, toolCalls, runID, 0)
	assertRunEvidenceCount(t, ctx, toolCalls, otherRunID, 1)
	assertRunEvidenceCount(t, ctx, patches, runID, 0)
	assertRunEvidenceCount(t, ctx, patches, otherRunID, 1)
	assertRunEvidenceCount(t, ctx, validations, runID, 0)
	assertRunEvidenceCount(t, ctx, validations, otherRunID, 1)
	assertRunEvidenceCount(t, ctx, checkpoints, runID, 0)
	assertRunEvidenceCount(t, ctx, checkpoints, otherRunID, 1)
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
	hub := events.NewMemoryHub()
	steps := memoryrepo.NewStepRepository()
	runID := common.RunID("run-purge-contract")
	if err := steps.Create(ctx, &runmodel.Step{ID: "step-purge-contract", RunID: runID}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	memoryModule := memorymodule.New()
	if err := memoryModule.Configure(memorymodule.Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(),
		EventHub:           hub,
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
		Runtime:               NewRuntimeAdapterRegistry(),
		Steps:                 steps,
		ToolCalls:             memoryrepo.NewToolCallRepository(),
		Patches:               memoryrepo.NewPatchRepository(),
		ValidationResults:     memoryrepo.NewValidationResultRepository(),
		RuntimeCheckpoints:    memoryrepo.NewRuntimeCheckpointRepository(),
		CoverageWorkItems:     memoryrepo.NewCoverageWorkItemRepository(),
		ArtifactContinuations: memoryrepo.NewArtifactContinuationRepository(),
		EventHub:              hub,
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       orchestratorevents.TopicOrchestratorCommandPurgeRun,
		SubjectType: "run",
		SubjectID:   string(runID),
		Payload: map[string]any{
			events.PayloadKeyCommand: orchestratorcontracts.PurgeRunCommand{RunID: runID},
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("purge err = %v, want response", err)
	}
	items, err := steps.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("steps after purge = %d, want 0", len(items))
	}
}
