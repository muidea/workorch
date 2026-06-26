package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func WorkspaceSnapshotFromModels(input WorkspaceSnapshotModelInput) readmodelcore.WorkspaceSnapshot {
	ret := readmodelcore.WorkspaceSnapshot{
		Workspace:             MapWorkspaceWithRunProjection(input.Workspace, input.ProjectionRun),
		Goals:                 definitioncore.MapGoals(input.Goals),
		DoDs:                  definitioncore.MapDoDs(input.DoDs),
		Runs:                  mapWorkspaceSnapshotRuns(input.Workspace, input.Runs),
		Results:               MapResults(input.Results),
		Artifacts:             MapArtifacts(input.Artifacts),
		ArtifactContinuations: MapArtifactContinuations(input.ArtifactContinuations),
		Patches:               MapPatches(input.Patches),
		EventRecords:          MapProjectedEventRecords(input.EventRecords),
		Decisions:             input.Decisions,
		Approvals:             input.Approvals,
	}
	ret.LLMUsageTraces = MapLLMUsageTracesFromEventRecords(MapEventRecords(input.LLMUsageEventRecords), "")
	ret.LLMContextBudgetSummary = SummarizeLLMContextBudgets(ret.LLMContextBudgets)
	ret.LLMUsageSummary = SummarizeLLMUsage(ret.LLMUsageTraces)
	ret.Acceptance = MapWorkspaceAcceptanceProjection(ret)
	ret.TaskProgress = MapLocalizedWorkspaceTaskProgress(ret)
	ret.Outcome = MapWorkspaceOutcome(ret)
	ret.RunHeartbeat = MapRunHeartbeat(ret)
	ret.ConvergenceStatus = MapWorkspaceConvergenceStatus(ret)
	ret = WorkspaceSnapshotWithContextBundle(ret)
	return ret
}
