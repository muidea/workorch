package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectWorkspaceSnapshotSummary(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	ret := ProjectWorkspaceSnapshotDetail(snapshot)
	ret.Run = nil
	ret.Step = nil
	ret.Result = nil
	ret.RuntimeExecution = nil
	ret.ContextBundle = nil
	ret.GapBaseline = nil
	ret.AuthorityDiagnostics = nil
	ret.Acceptance = readmodelcore.AcceptanceProjection{}
	ret.TaskProgress.Milestones = nil
	ret.TaskProgress.Items = nil
	ret.TaskProgress.EvidenceRefs = nil
	ret.TaskDecompositions = nil
	ret.TaskPlans = nil
	ret.ExecutionSections = nil
	ret.MemoryRecallTraces = nil
	ret.Goals = nil
	ret.DoDs = nil
	ret.Runs = nil
	ret.Results = nil
	ret.Artifacts = nil
	ret.ArtifactContinuations = nil
	ret.Patches = nil
	ret.WebSources = nil
	ret.LLMContextBudgets = nil
	ret.LLMUsageTraces = nil
	ret.EventRecords = nil
	return ret
}
