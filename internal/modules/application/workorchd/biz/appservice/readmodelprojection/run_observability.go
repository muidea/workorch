package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapRunObservability(snapshot readmodelcore.RunSnapshot) readmodelcore.RunObservability {
	timeline := RuntimeTimeline(snapshot)
	return readmodelcore.RunObservability{
		Run:                     snapshot.Run,
		Outcome:                 snapshot.Outcome,
		Progress:                MapRuntimeProgress(snapshot),
		TaskProgress:            snapshot.TaskProgress,
		RepairLoop:              snapshot.RepairLoop,
		Acceptance:              snapshot.Acceptance,
		TaskDecompositions:      snapshot.TaskDecompositions,
		TaskPlans:               snapshot.TaskPlans,
		ExecutionSections:       snapshot.ExecutionSections,
		LLMContextBudgets:       snapshot.LLMContextBudgets,
		LLMContextBudgetSummary: snapshot.LLMContextBudgetSummary,
		LLMUsageTraces:          snapshot.LLMUsageTraces,
		LLMUsageSummary:         snapshot.LLMUsageSummary,
		MemoryRecallTraces:      snapshot.MemoryRecallTraces,
		GitOperations:           snapshot.GitOperations,
		WebSources:              snapshot.WebSources,
		ClosureTraces:           snapshot.ClosureTraces,
		DoDEvidence:             snapshot.DoDEvidence,
		ResultQuality:           snapshot.ResultQuality,
		Evidence:                MapRuntimeEvidenceSummary(snapshot, len(TimelineEvents(timeline))),
		LatestCheckpoint:        LatestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		SelectedCapabilities:    snapshot.SelectedCapabilities,
		UsedCapabilities:        snapshot.UsedCapabilities,
		Timeline:                timeline,
	}
}
