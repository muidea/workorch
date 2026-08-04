package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func WorkspaceSnapshotWithRunSnapshot(snapshot readmodelcore.WorkspaceSnapshotState, runSnapshot *readmodelcore.RunSnapshotState) readmodelcore.WorkspaceSnapshotState {
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
	ret.Results = MergeWorkspaceSnapshotResults(ret.Results, runSnapshot.Results)
	ret.Artifacts = MergeWorkspaceSnapshotArtifacts(ret.Artifacts, runSnapshot.Artifacts)
	ret.TaskProgress = runSnapshot.TaskProgress
	ret.CoverageSummary = CoverageSummaryFromRunSnapshot(*runSnapshot)
	ret.GapBaseline = runSnapshot.GapBaseline
	ret.AuthorityDiagnostics = append([]projectioncontract.GapDiagnostic(nil), runSnapshot.AuthorityDiagnostics...)
	ret.Acceptance = runSnapshot.Acceptance
	ret.TaskDecompositions = runSnapshot.TaskDecompositions
	ret.TaskPlans = runSnapshot.TaskPlans
	ret.ExecutionSections = runSnapshot.ExecutionSections
	ret.MemoryRecallTraces = runSnapshot.MemoryRecallTraces
	ret.Patches = MergeWorkspaceSnapshotPatches(ret.Patches, runSnapshot.Patches)
	ret.ArtifactContinuations = MergeWorkspaceSnapshotArtifactContinuations(ret.ArtifactContinuations, runSnapshot.ArtifactContinuations)
	ret.Decisions = MergeWorkspaceSnapshotDecisions(ret.Decisions, runSnapshot.Decisions)
	ret.Approvals = MergeWorkspaceSnapshotApprovals(ret.Approvals, runSnapshot.Approvals)
	ret.WebSources = runSnapshot.WebSources
	ret.LLMContextBudgets = runSnapshot.LLMContextBudgets
	ret.LLMContextBudgetSummary = runSnapshot.LLMContextBudgetSummary
	ret.LLMUsageTraces = MergeWorkspaceLLMUsageTraces(ret.LLMUsageTraces, runSnapshot.LLMUsageTraces)
	ret.LLMUsageSummary = SummarizeLLMUsage(ret.LLMUsageTraces)
	ret.Outcome = MapWorkspaceOutcome(ret)
	if runSnapshot.TaskProgress.WorkspaceID != "" {
		ret.TaskProgress = runSnapshot.TaskProgress
	}
	ret.RunHeartbeat = MapRunHeartbeat(ret)
	ret.ConvergenceStatus = MapWorkspaceConvergenceStatus(ret)
	bundle := MapRunContextBundle(*runSnapshot)
	ret.ContextBundle = &bundle
	return ret
}

func MergeWorkspaceSnapshotDecisions(existing []projectioncontract.Decision, latest []projectioncontract.Decision) []projectioncontract.Decision {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.Decision, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func MergeWorkspaceSnapshotApprovals(existing []projectioncontract.Approval, latest []projectioncontract.Approval) []projectioncontract.Approval {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.Approval, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func WorkspaceSnapshotWithContextBundle(snapshot readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	bundle := MapWorkspaceContextBundle(snapshot)
	snapshot.ContextBundle = &bundle
	return snapshot
}
