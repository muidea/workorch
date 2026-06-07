package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunSnapshotFromModelsProjectsBlockingDecisionStepAndCoverage(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-run-snapshot-projection")
	runID := common.RunID("run-snapshot-projection")
	blockingStepID := common.StepID("step-blocking")
	ticket := &decisionmodel.Ticket{
		ID:          "ticket-blocking",
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      blockingStepID,
		State:       common.DecisionTicketStateOpen,
	}
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "projection workspace", State: common.WorkspaceStateRunning},
		Run: &runmodel.Run{
			ID:               runID,
			WorkspaceID:      workspaceID,
			State:            common.RunStateBlockedOnDecision,
			BlockingTicketID: ticket.ID,
			ExecutionContext: map[string]any{
				"coverageWorkItems": []any{map[string]any{"id": "cwi-001"}},
				"coverageWorkItemContext": map[string]any{
					"summary": map[string]any{"open": float64(1)},
				},
			},
		},
		Steps: []*runmodel.Step{
			{ID: "step-stale", RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateCompleted},
			{ID: blockingStepID, RunID: runID, Kind: "llm/approval", State: common.StepStatePending},
		},
		Checkpoints: []*runmodel.RuntimeCheckpoint{
			{ID: "checkpoint-stale", WorkspaceID: workspaceID, RunID: runID, CurrentStepID: "step-stale"},
		},
		SelectedTicket: ticket,
		Decisions: []readmodelcore.Decision{{
			ID:          string(ticket.ID),
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			StepID:      string(blockingStepID),
			State:       string(common.DecisionTicketStateOpen),
		}},
	})

	if got.Step == nil || got.Step.ID != string(blockingStepID) {
		t.Fatalf("Step = %+v, want blocking decision step %s", got.Step, blockingStepID)
	}
	if got.TaskProgress.BlockingTicketID != string(ticket.ID) {
		t.Fatalf("TaskProgress.BlockingTicketID = %q, want %s", got.TaskProgress.BlockingTicketID, ticket.ID)
	}
	if len(got.CoverageWorkItems) != 1 || got.CoverageWorkItems[0]["id"] != "cwi-001" {
		t.Fatalf("CoverageWorkItems = %+v, want copied coverage item", got.CoverageWorkItems)
	}
	if got.CoverageWorkItemContext["summary"] == nil {
		t.Fatalf("CoverageWorkItemContext = %+v, want copied coverage context", got.CoverageWorkItemContext)
	}
}

func TestRunRecoverySnapshotFromModelsUsesCheckpointStep(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-run-recovery-projection")
	runID := common.RunID("run-recovery-projection")
	currentStepID := common.StepID("step-current")
	got := RunRecoverySnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "recovery workspace", State: common.WorkspaceStateRunning},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		Steps: []*runmodel.Step{
			{ID: "step-old", RunID: runID, Kind: "llm/planning", State: common.StepStateCompleted},
			{ID: currentStepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateDispatched},
		},
		Checkpoints: []*runmodel.RuntimeCheckpoint{
			{ID: "checkpoint-current", WorkspaceID: workspaceID, RunID: runID, CurrentStepID: currentStepID},
		},
	})

	if got.Step == nil || got.Step.ID != string(currentStepID) {
		t.Fatalf("Step = %+v, want checkpoint current step %s", got.Step, currentStepID)
	}
	if got.TaskProgress.CurrentStepID != string(currentStepID) {
		t.Fatalf("TaskProgress.CurrentStepID = %q, want %s", got.TaskProgress.CurrentStepID, currentStepID)
	}
	if len(got.TaskDecompositions) != 0 {
		t.Fatalf("TaskDecompositions = %+v, want recovery snapshot without full task decomposition projection", got.TaskDecompositions)
	}
}
