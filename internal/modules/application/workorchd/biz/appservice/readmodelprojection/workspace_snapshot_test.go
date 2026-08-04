package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"

	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceSnapshotFromModelsProjectsBaseWorkspaceView(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-workspace-snapshot-projection")
	activeRunID := common.RunID("run-active-workspace-projection")
	got := WorkspaceSnapshotFromModels(WorkspaceSnapshotModelInput{
		Workspace:     &workspacemodel.Workspace{ID: workspaceID, Title: "workspace projection", State: common.WorkspaceStateActive, ActiveRunID: activeRunID},
		ProjectionRun: &runmodel.Run{ID: activeRunID, WorkspaceID: workspaceID, State: common.RunStateActive},
		Runs: []*runmodel.Run{
			{ID: activeRunID, WorkspaceID: workspaceID, State: common.RunStateActive},
			{ID: "run-other-workspace", WorkspaceID: "ws-other", State: common.RunStateActive},
		},
		Goals: []*workspacemodel.Goal{{ID: "goal-workspace-projection", WorkspaceID: workspaceID, Text: "goal"}},
		DoDs:  []*workspacemodel.DoD{{ID: "dod-workspace-projection", WorkspaceID: workspaceID, Items: []string{"dod"}, State: common.DoDStateReady}},
		Results: []*resultmodel.Result{{
			ID:          "result-workspace-projection",
			WorkspaceID: workspaceID,
			RunID:       activeRunID,
			State:       common.ResultStateAccepted,
		}},
		Decisions: []projectioncontract.Decision{{ID: "ticket-workspace-projection", WorkspaceID: string(workspaceID), RunID: string(activeRunID)}},
		Approvals: []projectioncontract.Approval{{ID: "approval-workspace-projection", WorkspaceID: string(workspaceID), RunID: string(activeRunID)}},
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
	if got.Acceptance.Summary.AcceptedSlices != 0 || got.Acceptance.Summary.ActiveSlices != 0 || got.Acceptance.CurrentFocus != nil {
		t.Fatalf("Acceptance = %+v, want empty projection for run without slices", got.Acceptance)
	}
}

func TestSelectTerminalResultRunIgnoresAcceptedSlice(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-terminal-result-slice")
	runID := common.RunID("run-terminal-result-slice")
	result, run := SelectTerminalResultRun(workspaceID, map[common.RunID]*runmodel.Run{
		runID: {ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted},
	}, []*resultmodel.Result{
		{
			ID:          "run-terminal-result-slice/result/acceptance-slice/slice-001",
			WorkspaceID: workspaceID,
			RunID:       runID,
			State:       common.ResultStateAccepted,
			SourceType:  resultcommon.SourceTypeAcceptanceSlice,
			SummaryDetail: resultmodel.ResultSummary{
				Diagnostics: workorcheventpayload.New(
					workorcheventpayload.StringField("resultScope", resultcommon.SourceTypeAcceptanceSlice),
					workorcheventpayload.StringField("finalCompletionGateState", "not_final_result"),
				),
			},
			CreatedAt: "2026-06-28T05:55:05Z",
			UpdatedAt: "2026-06-28T05:55:05Z",
		},
	})
	if result != nil || run != nil {
		t.Fatalf("SelectTerminalResultRun = %+v/%+v, want accepted slice ignored", result, run)
	}
}

func TestWorkspaceSnapshotWithRunSnapshotMergesRunProjection(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-workspace-run-merge")
	runID := common.RunID("run-workspace-run-merge")
	base := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: string(workspaceID), State: string(common.WorkspaceStateActive)},
		Results: []projectioncontract.Result{
			{ID: "result-run", WorkspaceID: string(workspaceID), RunID: string(runID), State: string(common.ResultStateRejected)},
			{ID: "result-workspace-only", WorkspaceID: string(workspaceID), RunID: "run-previous", State: string(common.ResultStateAccepted)},
		},
		Artifacts: []projectioncontract.Artifact{
			{ID: "artifact-result", WorkspaceID: string(workspaceID), RunID: string(runID), ResultID: "result-run", State: string(common.ArtifactStateCreated)},
			{ID: "artifact-workspace-only", WorkspaceID: string(workspaceID), RunID: "run-previous", State: string(common.ArtifactStateAvailable)},
		},
		Patches: []projectioncontract.Patch{
			{ID: "patch-shared", State: string(common.PatchStateApplied)},
			{ID: "patch-workspace-only", State: string(common.PatchStateApplied)},
		},
		ArtifactContinuations: []projectioncontract.ArtifactContinuation{
			{ID: "continuation-shared", RunID: string(runID)},
		},
		LLMUsageTraces: []projectioncontract.LLMUsageTrace{{ID: "usage-workspace", StepID: "step-workspace", OccurredAt: "2026-06-05T00:00:00Z"}},
		Decisions:      []projectioncontract.Decision{{ID: "decision-shared", State: "open"}, {ID: "decision-workspace-only", State: "open"}},
		Approvals:      []projectioncontract.Approval{{ID: "approval-shared", State: "pending"}, {ID: "approval-workspace-only", State: "pending"}},
	}
	runSnapshot := &readmodelcore.RunSnapshotState{
		Run: appmodelcore.Run{ID: string(runID), WorkspaceID: string(workspaceID), State: string(common.RunStateCompleted)},
		Step: &projectioncontract.StepView{
			ID:    "step-current",
			State: string(common.StepStateSucceeded),
		},
		TaskProgress:       projectioncontract.TaskProgress{WorkspaceID: string(workspaceID), RunID: string(runID), State: string(common.RunStateCompleted), CurrentStepID: "step-current"},
		Acceptance:         projectioncontract.AcceptanceProjection{ActiveSliceID: "slice-run", Summary: projectioncontract.AcceptanceSummary{ActiveSlices: 1}},
		GapBaseline:        &projectioncontract.GapBaseline{WorkspaceID: string(workspaceID), RunID: string(runID), BaselineRevision: "gap-run"},
		TaskDecompositions: []projectioncontract.TaskDecomposition{{ID: "decomposition-run"}},
		TaskPlans:          []projectioncontract.TaskPlan{{ID: "plan-run", AnchorStepID: "step-current", Revision: 1, State: "accepted"}},
		ExecutionSections:  []projectioncontract.ExecutionSection{{ID: "section-run"}},
		MemoryRecallTraces: []projectioncontract.MemoryRecallTrace{{ID: "memory-run", RunID: string(runID)}},
		Patches: []projectioncontract.Patch{
			{ID: "patch-shared", State: string(common.PatchStateFailed)},
			{ID: "patch-run-only", State: string(common.PatchStateApplied)},
		},
		ArtifactContinuations: []projectioncontract.ArtifactContinuation{
			{ID: "continuation-shared", RunID: string(runID), State: string(common.ArtifactContinuationStateApplied), FinalReceived: true},
			{ID: "continuation-run-only", RunID: string(runID), State: string(common.ArtifactContinuationStateChunking)},
		},
		WebSources: []projectioncontract.WebSourceTrace{{Query: "web-run"}},
		LLMContextBudgets: []projectioncontract.LLMContextBudgetTrace{{
			StepID:               "budget-run",
			EstimatedInputTokens: 2048,
			Truncated:            true,
			ContextUtilization:   0.75,
		}},
		LLMContextBudgetSummary: projectioncontract.LLMContextBudgetSummary{
			TraceTotal:            1,
			TruncatedCount:        1,
			MaxInputTokens:        2048,
			MaxContextUtilization: 0.75,
		},
		LLMUsageTraces: []projectioncontract.LLMUsageTrace{{ID: "usage-run", StepID: "step-run", OccurredAt: "2026-06-05T00:01:00Z"}},
		Results: []projectioncontract.Result{{
			ID:          "result-run",
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			State:       string(common.ResultStateAccepted),
			ArtifactRefs: []string{
				"artifact-result",
			},
			EvidenceRefs: []string{
				"step-current",
				"validation-current",
			},
		}},
		Artifacts: []projectioncontract.Artifact{{
			ID:           "artifact-result",
			WorkspaceID:  string(workspaceID),
			RunID:        string(runID),
			ResultID:     "result-run",
			State:        string(common.ArtifactStateAvailable),
			Category:     "runtime_evidence",
			EvidenceRefs: []string{"validation-current"},
		}},
		Decisions: []projectioncontract.Decision{
			{ID: "decision-shared", State: "resolved", RunID: string(runID)},
			{ID: "decision-run-only", State: "open", RunID: string(runID)},
		},
		Approvals: []projectioncontract.Approval{
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
	if len(got.Results) != 2 || got.Results[0].ID != "result-run" || got.Results[0].State != string(common.ResultStateAccepted) || len(got.Results[0].ArtifactRefs) != 1 {
		t.Fatalf("Results = %+v, want run result projection overriding workspace base", got.Results)
	}
	if len(got.Artifacts) != 2 ||
		got.Artifacts[0].ID != "artifact-result" ||
		got.Artifacts[0].State != string(common.ArtifactStateAvailable) ||
		got.Artifacts[0].ResultID != "result-run" ||
		len(got.Artifacts[0].EvidenceRefs) != 1 {
		t.Fatalf("Artifacts = %+v, want accepted result-linked artifact projection", got.Artifacts)
	}
	if got.Acceptance.ActiveSliceID != "slice-run" || got.Acceptance.Summary.ActiveSlices != 1 {
		t.Fatalf("Acceptance = %+v, want merged run acceptance", got.Acceptance)
	}
	if got.GapBaseline == nil || got.GapBaseline.BaselineRevision != "gap-run" {
		t.Fatalf("GapBaseline = %+v, want merged run gap baseline", got.GapBaseline)
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
	if got.LLMContextBudgetSummary.TraceTotal != 1 ||
		got.LLMContextBudgetSummary.TruncatedCount != 1 ||
		got.LLMContextBudgetSummary.MaxInputTokens != 2048 {
		t.Fatalf("LLM context summary = %+v, want merged run context summary", got.LLMContextBudgetSummary)
	}
	if len(got.TaskDecompositions) != 1 || got.TaskDecompositions[0].ID != "decomposition-run" {
		t.Fatalf("TaskDecompositions = %+v, want run decompositions", got.TaskDecompositions)
	}
	if len(got.TaskPlans) != 1 || got.TaskPlans[0].ID != "plan-run" || got.TaskPlans[0].State != "accepted" {
		t.Fatalf("TaskPlans = %+v, want run task plans", got.TaskPlans)
	}
	if got.TaskProgress.CurrentStepID != "step-current" {
		t.Fatalf("TaskProgress = %+v, want run task progress", got.TaskProgress)
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
		Workspace: &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive},
		ArtifactContinuations: []*orchestratormodel.ArtifactContinuation{
			{ID: "continuation-workspace", WorkspaceID: workspaceID, RunID: "run-continuation"},
		},
	})
	if len(got.ArtifactContinuations) != 1 || got.ArtifactContinuations[0].ID != "continuation-workspace" {
		t.Fatalf("ArtifactContinuations = %+v, want projected continuation", got.ArtifactContinuations)
	}
}
