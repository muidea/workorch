package orchestrator

import (
	"context"
	"testing"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestPurgeRunEvidenceDeletesOnlyRequestedRunEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-purge-evidence")
	runID := common.RunID("run-purge-evidence")
	otherRunID := common.RunID("run-keep-evidence")
	steps := memory.NewStepRepository()
	toolCalls := memory.NewToolCallRepository()
	patches := memory.NewPatchRepository()
	validations := memory.NewValidationResultRepository()
	checkpoints := memory.NewRuntimeCheckpointRepository()
	continuations := memory.NewArtifactContinuationRepository()
	traces := memory.NewMemoryRecallTraceRepository()
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
	must(continuations.Create(ctx, &orchestratormodel.ArtifactContinuation{ID: "continuation-purge", WorkspaceID: workspaceID, RunID: runID}))
	must(continuations.Create(ctx, &orchestratormodel.ArtifactContinuation{ID: "continuation-keep", WorkspaceID: workspaceID, RunID: otherRunID}))
	must(traces.Create(ctx, &memorymodel.MemoryRecallTrace{ID: "trace-purge", WorkspaceID: workspaceID, RunID: runID}))
	must(traces.Create(ctx, &memorymodel.MemoryRecallTrace{ID: "trace-keep", WorkspaceID: workspaceID, RunID: otherRunID}))

	deleted, err := purgeRunEvidence(ctx, steps, orchestratorbiz.RuntimeEvidenceRepositories{
		ToolCalls:             toolCalls,
		Patches:               patches,
		ValidationResults:     validations,
		RuntimeCheckpoints:    checkpoints,
		ArtifactContinuations: continuations,
		MemoryRecallTraces:    traces,
	}, runID)
	if err != nil {
		t.Fatalf("purge run evidence: %v", err)
	}
	if deleted != 7 {
		t.Fatalf("deleted = %d, want 7", deleted)
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
