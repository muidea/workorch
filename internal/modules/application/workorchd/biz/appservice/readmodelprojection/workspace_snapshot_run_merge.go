package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func WorkspaceSnapshotWithRunSnapshot(snapshot readmodelcore.WorkspaceSnapshot, runSnapshot *readmodelcore.RunSnapshot) readmodelcore.WorkspaceSnapshot {
	ret := snapshot
	if runSnapshot == nil {
		ret.Outcome = MapWorkspaceOutcome(ret)
		ret.RunHeartbeat = MapRunHeartbeat(ret)
		return WorkspaceSnapshotWithContextBundle(ret)
	}

	run := runSnapshot.Run
	ret.Run = &run
	ret.Step = runSnapshot.Step
	if result := PrimaryTaskProgressResult(RunScopedResults(*runSnapshot)); result != nil {
		selected := *result
		ret.Result = &selected
	}
	ret.TaskProgress = runSnapshot.TaskProgress
	ret.RuntimeExecution = runSnapshot.RuntimeExecution
	ret.CoverageSummary = CoverageSummaryFromRunSnapshot(*runSnapshot)
	ret.TaskDecompositions = runSnapshot.TaskDecompositions
	ret.ExecutionSections = runSnapshot.ExecutionSections
	ret.MemoryRecallTraces = runSnapshot.MemoryRecallTraces
	ret.Patches = MergeWorkspaceSnapshotPatches(ret.Patches, runSnapshot.Patches)
	ret.ArtifactContinuations = MergeWorkspaceSnapshotArtifactContinuations(ret.ArtifactContinuations, runSnapshot.ArtifactContinuations)
	ret.WebSources = runSnapshot.WebSources
	ret.LLMContextBudgets = runSnapshot.LLMContextBudgets
	ret.LLMUsageTraces = MergeWorkspaceLLMUsageTraces(ret.LLMUsageTraces, runSnapshot.LLMUsageTraces)
	ret.LLMUsageSummary = SummarizeLLMUsage(ret.LLMUsageTraces)
	ret.Outcome = MapWorkspaceOutcome(ret)
	ret.RunHeartbeat = MapRunHeartbeat(ret)
	bundle := MapRunContextBundle(*runSnapshot)
	ret.ContextBundle = &bundle
	return ret
}

func WorkspaceSnapshotWithContextBundle(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	bundle := MapWorkspaceContextBundle(snapshot)
	snapshot.ContextBundle = &bundle
	return snapshot
}
