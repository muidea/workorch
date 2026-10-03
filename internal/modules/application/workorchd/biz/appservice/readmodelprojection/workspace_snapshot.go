package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func workspaceSnapshotFromModels(input WorkspaceSnapshotModelInput) readmodelcore.WorkspaceSnapshotState {
	ret := readmodelcore.WorkspaceSnapshotState{
		Workspace:             mapWorkspaceWithRunProjection(input.Workspace, input.ProjectionRun),
		Goals:                 definitioncore.MapGoals(input.Goals),
		DoDs:                  definitioncore.MapDoDs(input.DoDs),
		Runs:                  mapWorkspaceSnapshotRuns(input.Workspace, input.Runs),
		Results:               mapResults(input.Results),
		Artifacts:             mapArtifacts(input.Artifacts),
		ArtifactContinuations: mapArtifactContinuations(input.ArtifactContinuations),
		Patches:               mapPatches(input.Patches),
		EventRecords:          mapProjectedEventRecords(input.EventRecords),
		Decisions:             input.Decisions,
		Approvals:             input.Approvals,
	}
	if input.ProjectionRun != nil {
		run := runtimecore.MapRun(input.ProjectionRun)
		ret.Run = &run
	}
	ret.LLMUsageTraces = mapLLMUsageTracesFromEventRecords(mapEventRecords(input.LLMUsageEventRecords), "")
	ret.LLMContextBudgetSummary = summarizeLLMContextBudgets(ret.LLMContextBudgets)
	ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	ret.Acceptance = mapAcceptanceProjection(input.ProjectionRun)
	ret.TaskProgress = mapLocalizedWorkspaceTaskProgress(ret)
	applyRuntimeProgress(&ret.TaskProgress, input.ProjectionRun)
	ret.TaskProgress = finalizeWorkspaceFormalProgress(ret.TaskProgress, ret)
	ret.DisplayOutcome = mapWorkspaceOutcome(ret)
	ret.Result = workspaceSnapshotResultByID(ret.Results, ret.DisplayOutcome.ResultID)
	ret.RunHeartbeat = mapRunHeartbeat(ret)
	ret.ConvergenceStatus = mapWorkspaceConvergenceStatus(ret)
	ret = normalizeWorkspaceGovernanceProjection(ret)
	ret = workspaceSnapshotWithContextBundle(ret)
	return ret
}

func workspaceSnapshotResultByID(items []projectioncontract.Result, resultID string) *projectioncontract.Result {
	if resultID == "" {
		return nil
	}
	for idx := range items {
		if items[idx].ID == resultID {
			ret := items[idx]
			return &ret
		}
	}
	return nil
}
