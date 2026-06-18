package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestrator_EventDrivenGraphBatchPublishesDispatchRequests(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-event-driven-graph")
	workspaceID := common.WorkspaceID("ws-event-driven-graph")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		hub,
	)
	orch.EnableEventDrivenDispatch()

	err := orch.dispatchGraphStepBatchWithWorkspace(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}, []stepDispatchSpec{
		{ID: common.StepID(runID + "/graph-001/001-a"), Kind: "llm/workspace_execution", Input: map[string]any{"node": "a"}},
		{ID: common.StepID(runID + "/graph-001/002-b"), Kind: "llm/workspace_execution", Input: map[string]any{"node": "b"}},
	}, &workspacemodel.Workspace{ID: workspaceID})
	if err != nil {
		t.Fatalf("dispatchGraphStepBatchWithWorkspace: %v", err)
	}
	published := hub.Published()
	var dispatches []events.Envelope
	for _, envelope := range published {
		if envelope.Topic == orchestratorevents.TopicOrchestratorDispatchStepRequested {
			dispatches = append(dispatches, envelope)
		}
	}
	if len(dispatches) != 2 {
		t.Fatalf("dispatch events = %d in %+v, want 2", len(dispatches), published)
	}
	if dispatches[0].RunID != string(runID) || dispatches[0].WorkspaceID != string(workspaceID) || dispatches[0].StepID == "" {
		t.Fatalf("dispatch envelope = %+v, want scoped workspace/run/step", dispatches[0])
	}
	if key, _ := dispatches[0].Payload[events.PayloadKeyIdempotencyKey].(string); key == "" {
		t.Fatalf("dispatch payload = %+v, want idempotency key", dispatches[0].Payload)
	}
	if _, err := stepRepo.Get(ctx, common.StepID(runID+"/graph-001/001-a")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("graph step after dispatch request err = %v, want not found before dispatch handler runs", err)
	}
}

func TestRunEvidenceSummaryPrioritizesActionEvidenceWhenLarge(t *testing.T) {
	runID := common.RunID("run-evidence-priority")
	steps := make([]*runmodel.Step, 0, maxLLMContextSliceItems+3)
	for idx := 0; idx < maxLLMContextSliceItems+2; idx++ {
		steps = append(steps, &runmodel.Step{
			ID:     common.StepID(fmt.Sprintf("%s/llm-%03d", runID, idx)),
			RunID:  runID,
			Kind:   llmWorkspaceExecutionStepKind,
			State:  common.StepStateCompleted,
			Output: map[string]any{"status": "completed"},
		})
	}
	critical := &runmodel.Step{
		ID:     common.StepID(runID + "/graph-001/002-verify"),
		RunID:  runID,
		Kind:   shellcommon.StepKindCommand,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stdout": "critical validation passed"},
	}
	steps = append([]*runmodel.Step{critical}, steps...)

	evidence := runEvidenceSummaryFromSteps(steps)

	if len(evidence) != maxLLMContextSliceItems {
		t.Fatalf("runEvidenceSummary length = %d, want %d", len(evidence), maxLLMContextSliceItems)
	}
	foundCritical := false
	for _, item := range evidence {
		if item["stepId"] == string(critical.ID) {
			foundCritical = true
			break
		}
	}
	if !foundCritical {
		t.Fatalf("runEvidenceSummary = %+v, want critical shell evidence retained", evidence)
	}
}

func TestRunEvidenceSummaryIncludesOutputRefAndTargetedLargeTextExcerpt(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-evidence-ref",
		WorkspaceID: "ws-evidence-ref",
	}
	step := &runmodel.Step{
		ID:    "run-evidence-ref/graph-001/001-verify",
		RunID: run.ID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stderr": strings.Repeat("head-line\n", 200) + "final actionable failure",
		},
	}

	evidence := runEvidenceSummaryFromRunSteps(run, []*runmodel.Step{step})

	if len(evidence) != 1 {
		t.Fatalf("runEvidenceSummary length = %d, want 1", len(evidence))
	}
	if evidence[0]["outputRef"] != "workorch://workspaces/ws-evidence-ref/runs/run-evidence-ref/steps/run-evidence-ref/graph-001/001-verify/output.json" {
		t.Fatalf("outputRef = %#v", evidence[0]["outputRef"])
	}
	output := mapFromValue(evidence[0]["output"])
	stderr := mapFromValue(output["stderr"])
	if stderr["truncated"] != true ||
		!strings.Contains(fmt.Sprint(stderr["headPreview"]), "head-line") ||
		!strings.Contains(fmt.Sprint(stderr["tailPreview"]), "final actionable failure") {
		t.Fatalf("stderr = %+v, want targeted head/tail excerpt", stderr)
	}
}

func TestRuntimeOrchestrator_ReadyGraphStepSpecsReusesLoadedStepsForStepID(t *testing.T) {
	ctx := context.Background()
	stepRepo := newCountingStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-graph-step-id-efficiency")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-step-id-efficiency"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify-file", Kind: "graph/mock"},
	}}
	root := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/zz-llm-after-graph-root"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	conflictingBaseStep := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/block/verify-file"),
		RunID:  runID,
		Kind:   "graph/mock",
		State:  common.StepStateCompleted,
		Input:  map[string]any{stepInputKeyGraphNodeID: "verify-file", stepInputKeyGraphRootStepID: string(runID) + "/previous-root"},
		Output: map[string]any{"stdout": "previous graph scope"},
	}
	for _, step := range []*runmodel.Step{root, conflictingBaseStep} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	specs, _ := orch.graphManager().readyGraphStepSpecsWithSteps(ctx, run, root)

	if got := stepRepo.listCount.Load(); got != 1 {
		t.Fatalf("step ListByRun count = %d, want one graph step scan", got)
	}
	if got := stepRepo.getCount.Load(); got != 0 {
		t.Fatalf("step Get count = %d, want step ID resolution to reuse loaded steps", got)
	}
	if len(specs) != 1 {
		t.Fatalf("ready specs = %+v, want one graph node spec", specs)
	}
	wantID := graphScopedStepID(runID, 2, 1, "verify-file")
	if specs[0].ID != wantID {
		t.Fatalf("spec ID = %s, want scoped ID %s", specs[0].ID, wantID)
	}
	if strings.Contains(string(specs[0].ID), "zz-llm-after-graph") {
		t.Fatalf("spec ID = %s, want readable scoped ID without embedded root lineage", specs[0].ID)
	}
}
