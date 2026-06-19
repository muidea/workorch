package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceSnapshotFromModelsProjectsBaseWorkspaceView(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-workspace-snapshot-projection")
	activeRunID := common.RunID("run-active-workspace-projection")
	got := WorkspaceSnapshotFromModels(WorkspaceSnapshotModelInput{
		Workspace:     &workspacemodel.Workspace{ID: workspaceID, Title: "workspace projection", State: common.WorkspaceStateRunning, ActiveRunID: activeRunID},
		ProjectionRun: &runmodel.Run{ID: activeRunID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		Runs: []*runmodel.Run{
			{ID: activeRunID, WorkspaceID: workspaceID, State: common.RunStateRunning},
			{ID: "run-other-workspace", WorkspaceID: "ws-other", State: common.RunStateRunning},
		},
		Goals: []*workspacemodel.Goal{{ID: "goal-workspace-projection", WorkspaceID: workspaceID, Text: "goal"}},
		DoDs:  []*workspacemodel.DoD{{ID: "dod-workspace-projection", WorkspaceID: workspaceID, Items: []string{"dod"}, State: common.DoDStateReady}},
		Results: []*resultmodel.Result{{
			ID:          "result-workspace-projection",
			WorkspaceID: workspaceID,
			RunID:       activeRunID,
			State:       common.ResultStateAccepted,
		}},
		Decisions: []readmodelcore.Decision{{ID: "ticket-workspace-projection", WorkspaceID: string(workspaceID), RunID: string(activeRunID)}},
		Approvals: []readmodelcore.Approval{{ID: "approval-workspace-projection", WorkspaceID: string(workspaceID), RunID: string(activeRunID)}},
	})

	if got.Workspace.ID != string(workspaceID) || got.Workspace.ActiveRunID != string(activeRunID) {
		t.Fatalf("Workspace = %+v, want projected active run", got.Workspace)
	}
	if len(got.Runs) != 1 || got.Runs[0].ID != string(activeRunID) {
		t.Fatalf("Runs = %+v, want only workspace run", got.Runs)
	}
	if len(got.Goals) != 1 || got.Goals[0].ID != "goal-workspace-projection" {
		t.Fatalf("Goals = %+v, want projected goal", got.Goals)
	}
	if len(got.DoDs) != 1 || got.DoDs[0].ID != "dod-workspace-projection" {
		t.Fatalf("DoDs = %+v, want projected DoD", got.DoDs)
	}
	if got.TaskProgress.WorkspaceID != string(workspaceID) || got.TaskProgress.RunID != string(activeRunID) {
		t.Fatalf("TaskProgress = %+v, want workspace and active run ids", got.TaskProgress)
	}
	if got.Outcome.State == "" {
		t.Fatalf("Outcome = %+v, want populated outcome", got.Outcome)
	}
}

func TestWorkspaceSnapshotWithRunSnapshotMergesRunProjection(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-workspace-run-merge")
	runID := common.RunID("run-workspace-run-merge")
	base := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: string(workspaceID), State: string(common.WorkspaceStateRunning)},
		Patches: []readmodelcore.Patch{
			{ID: "patch-shared", State: string(common.PatchStateApplied)},
			{ID: "patch-workspace-only", State: string(common.PatchStateApplied)},
		},
		ArtifactContinuations: []readmodelcore.ArtifactContinuation{
			{ID: "continuation-shared", RunID: string(runID)},
		},
		LLMUsageTraces: []readmodelcore.LLMUsageTrace{{ID: "usage-workspace", StepID: "step-workspace", OccurredAt: "2026-06-05T00:00:00Z"}},
		Decisions:      []readmodelcore.Decision{{ID: "decision-shared", State: "open"}, {ID: "decision-workspace-only", State: "open"}},
		Approvals:      []readmodelcore.Approval{{ID: "approval-shared", State: "pending"}, {ID: "approval-workspace-only", State: "pending"}},
	}
	runSnapshot := &readmodelcore.RunSnapshot{
		Run: readmodelcore.Run{ID: string(runID), WorkspaceID: string(workspaceID), State: string(common.RunStateCompleted)},
		Step: &presenter.StepView{
			ID:    "step-current",
			State: string(common.StepStateCompleted),
		},
		TaskProgress:       readmodelcore.TaskProgress{WorkspaceID: string(workspaceID), RunID: string(runID), State: string(common.RunStateCompleted), CurrentStepID: "step-current"},
		RuntimeExecution:   &readmodelcore.RuntimeExecutionState{StepID: "step-current", RequestID: "request-current", Pending: true, Gap: "runtime_terminal_not_finalized"},
		TaskDecompositions: []readmodelcore.TaskDecomposition{{ID: "decomposition-run"}},
		ExecutionSections:  []readmodelcore.ExecutionSection{{ID: "section-run"}},
		MemoryRecallTraces: []readmodelcore.MemoryRecallTrace{{ID: "memory-run", RunID: string(runID)}},
		Patches: []readmodelcore.Patch{
			{ID: "patch-shared", State: string(common.PatchStateFailed)},
			{ID: "patch-run-only", State: string(common.PatchStateApplied)},
		},
		ArtifactContinuations: []readmodelcore.ArtifactContinuation{
			{ID: "continuation-shared", RunID: string(runID), State: string(common.ArtifactContinuationStateApplied), FinalReceived: true},
			{ID: "continuation-run-only", RunID: string(runID), State: string(common.ArtifactContinuationStateChunking)},
		},
		WebSources:        []readmodelcore.WebSourceTrace{{Query: "web-run"}},
		LLMContextBudgets: []readmodelcore.LLMContextBudgetTrace{{StepID: "budget-run"}},
		LLMUsageTraces:    []readmodelcore.LLMUsageTrace{{ID: "usage-run", StepID: "step-run", OccurredAt: "2026-06-05T00:01:00Z"}},
		Results: []readmodelcore.Result{{
			ID:          "result-run",
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			State:       string(common.ResultStateAccepted),
		}},
		Decisions: []readmodelcore.Decision{
			{ID: "decision-shared", State: "resolved", RunID: string(runID)},
			{ID: "decision-run-only", State: "open", RunID: string(runID)},
		},
		Approvals: []readmodelcore.Approval{
			{ID: "approval-shared", State: "approved", RunID: string(runID)},
			{ID: "approval-run-only", State: "pending", RunID: string(runID)},
		},
	}
	got := WorkspaceSnapshotWithRunSnapshot(base, runSnapshot)

	if got.Run == nil || got.Run.ID != string(runID) {
		t.Fatalf("Run = %+v, want merged run", got.Run)
	}
	if got.Step == nil || got.Step.ID != "step-current" {
		t.Fatalf("Step = %+v, want merged step", got.Step)
	}
	if got.Result == nil || got.Result.ID != "result-run" {
		t.Fatalf("Result = %+v, want primary run result", got.Result)
	}
	if len(got.Patches) != 3 || got.Patches[0].State != string(common.PatchStateFailed) {
		t.Fatalf("Patches = %+v, want merged run patch override", got.Patches)
	}
	if len(got.ArtifactContinuations) != 2 || got.ArtifactContinuations[0].State != string(common.ArtifactContinuationStateApplied) || !got.ArtifactContinuations[0].FinalReceived {
		t.Fatalf("ArtifactContinuations = %+v, want merged run continuation override", got.ArtifactContinuations)
	}
	if len(got.LLMUsageTraces) != 2 || got.LLMUsageSummary.EntryTotal != 2 {
		t.Fatalf("LLM usage = %+v summary=%+v, want merged traces and summary", got.LLMUsageTraces, got.LLMUsageSummary)
	}
	if len(got.TaskDecompositions) != 1 || got.TaskDecompositions[0].ID != "decomposition-run" {
		t.Fatalf("TaskDecompositions = %+v, want run decompositions", got.TaskDecompositions)
	}
	if got.TaskProgress.CurrentStepID != "step-current" {
		t.Fatalf("TaskProgress = %+v, want run task progress", got.TaskProgress)
	}
	if got.RuntimeExecution == nil || got.RuntimeExecution.RequestID != "request-current" || got.RuntimeExecution.Gap != "runtime_terminal_not_finalized" {
		t.Fatalf("RuntimeExecution = %+v, want run runtime projection", got.RuntimeExecution)
	}
	if len(got.Decisions) != 3 || got.Decisions[0].State != "resolved" {
		t.Fatalf("Decisions = %+v, want run decision override merged into workspace snapshot", got.Decisions)
	}
	if len(got.Approvals) != 3 || got.Approvals[0].State != "approved" {
		t.Fatalf("Approvals = %+v, want run approval override merged into workspace snapshot", got.Approvals)
	}
}

func TestWorkspaceSnapshotFromModelsProjectsArtifactContinuations(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-workspace-continuation-projection")
	got := WorkspaceSnapshotFromModels(WorkspaceSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning},
		ArtifactContinuations: []*orchestratormodel.ArtifactContinuation{
			{ID: "continuation-workspace", WorkspaceID: workspaceID, RunID: "run-continuation"},
		},
	})
	if len(got.ArtifactContinuations) != 1 || got.ArtifactContinuations[0].ID != "continuation-workspace" {
		t.Fatalf("ArtifactContinuations = %+v, want projected continuation", got.ArtifactContinuations)
	}
}
