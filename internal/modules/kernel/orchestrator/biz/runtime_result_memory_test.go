package orchestrator

import (
	"context"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRecordRunSummaryMemoryIncludesDoDGateVersionTag(t *testing.T) {
	ctx := context.Background()
	memoryRepo := memory.NewMemoryItemRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems: memoryRepo,
			Results:     memory.NewResultRepository(),
			Artifacts:   memory.NewArtifactRepository(),
		},
	}
	run := &runmodel.Run{
		ID:              common.RunID("run-memory-gate-tag"),
		WorkspaceID:     common.WorkspaceID("ws-memory-gate-tag"),
		DoDID:           common.DoDID("dod-memory-gate-tag"),
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:    common.StepID("run-memory-gate-tag/step-1"),
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
	}
	assessment := map[string]any{
		"satisfied": true,
		"summary":   "DoD satisfied",
		"items": []map[string]any{
			{
				"item":         "evidence exists",
				"state":        "satisfied",
				"evidenceRefs": []string{string(step.ID)},
			},
		},
	}

	if err := orch.recordRunSummaryMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied"); err != nil {
		t.Fatalf("recordRunSummaryMemoryFromValidatedDoDAssessment: %v", err)
	}
	item, err := memoryRepo.Get(ctx, common.MemoryID("run-memory-gate-tag/memory/run-summary"))
	if err != nil {
		t.Fatalf("get memory item: %v", err)
	}
	found := false
	for _, tag := range item.Tags {
		if tag == runSummaryDoDGateVersionTag {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("memory tags = %+v, want %q", item.Tags, runSummaryDoDGateVersionTag)
	}
}
