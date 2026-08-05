package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// PublicRunSnapshot projects application-private read-model state to the
// protocol DTO. Private workspace and run state never crosses this boundary.
func PublicRunSnapshot(state readmodelcore.RunSnapshotState) projectioncontract.RunSnapshot {
	return projectioncontract.CloneRunSnapshot(projectioncontract.RunSnapshot{
		Workspace:               appmodelcore.ProjectWorkspace(state.Workspace),
		Run:                     appmodelcore.ProjectRun(state.Run),
		Outcome:                 state.Outcome,
		TaskProgress:            state.TaskProgress,
		RepairLoop:              state.RepairLoop,
		TaskDecompositions:      state.TaskDecompositions,
		TaskPlans:               state.TaskPlans,
		Step:                    state.Step,
		Steps:                   state.Steps,
		ToolCalls:               state.ToolCalls,
		Patches:                 state.Patches,
		GitOperations:           state.GitOperations,
		WebSources:              state.WebSources,
		ValidationResults:       state.ValidationResults,
		RuntimeCheckpoints:      state.RuntimeCheckpoints,
		ConvergenceStatus:       state.ConvergenceStatus,
		SchedulerDiagnostics:    state.SchedulerDiagnostics,
		Results:                 state.Results,
		ClosureTraces:           state.ClosureTraces,
		DoDEvidence:             state.DoDEvidence,
		ResultQuality:           state.ResultQuality,
		Artifacts:               state.Artifacts,
		ArtifactContinuations:   state.ArtifactContinuations,
		ExecutionSections:       state.ExecutionSections,
		LLMContextBudgets:       state.LLMContextBudgets,
		LLMContextBudgetSummary: state.LLMContextBudgetSummary,
		LLMUsageTraces:          state.LLMUsageTraces,
		LLMUsageSummary:         state.LLMUsageSummary,
		CoverageWorkItems:       state.CoverageWorkItems,
		CoverageWorkItemContext: state.CoverageWorkItemContext,
		GapBaseline:             state.GapBaseline,
		AuthorityDiagnostics:    state.AuthorityDiagnostics,
		Acceptance:              state.Acceptance,
		MemoryRecallTraces:      state.MemoryRecallTraces,
		SelectedCapabilities:    state.SelectedCapabilities,
		UsedCapabilities:        state.UsedCapabilities,
		EventRecords:            state.EventRecords,
		Decisions:               state.Decisions,
		Approvals:               state.Approvals,
		Observed:                state.Observed,
	})
}

// PublicWorkspaceSnapshot projects application-private read-model state to
// the protocol DTO while preserving every public observation.
func PublicWorkspaceSnapshot(state readmodelcore.WorkspaceSnapshotState) projectioncontract.WorkspaceSnapshot {
	ret := projectioncontract.WorkspaceSnapshot{
		Workspace:               appmodelcore.ProjectWorkspace(state.Workspace),
		ViewModel:               state.ViewModel,
		Step:                    state.Step,
		Result:                  state.Result,
		Outcome:                 state.Outcome,
		TaskProgress:            state.TaskProgress,
		RunHeartbeat:            state.RunHeartbeat,
		ConvergenceStatus:       state.ConvergenceStatus,
		RunCommand:              state.RunCommand,
		SchedulerDiagnostics:    state.SchedulerDiagnostics,
		ContextBundle:           state.ContextBundle,
		CoverageSummary:         state.CoverageSummary,
		GapBaseline:             state.GapBaseline,
		AuthorityDiagnostics:    state.AuthorityDiagnostics,
		Acceptance:              state.Acceptance,
		TaskDecompositions:      state.TaskDecompositions,
		TaskPlans:               state.TaskPlans,
		ExecutionSections:       state.ExecutionSections,
		MemoryRecallTraces:      state.MemoryRecallTraces,
		Goals:                   state.Goals,
		DoDs:                    state.DoDs,
		Runs:                    appmodelcore.ProjectRuns(state.Runs),
		Results:                 state.Results,
		Artifacts:               state.Artifacts,
		ArtifactContinuations:   state.ArtifactContinuations,
		Patches:                 state.Patches,
		WebSources:              state.WebSources,
		LLMContextBudgets:       state.LLMContextBudgets,
		LLMContextBudgetSummary: state.LLMContextBudgetSummary,
		LLMUsageTraces:          state.LLMUsageTraces,
		LLMUsageSummary:         state.LLMUsageSummary,
		EventRecords:            state.EventRecords,
		Decisions:               state.Decisions,
		Approvals:               state.Approvals,
	}
	if state.Run != nil {
		run := appmodelcore.ProjectRun(*state.Run)
		ret.Run = &run
	}
	return projectioncontract.CloneWorkspaceSnapshot(ret)
}

func PublicRunObservability(state readmodelcore.RunObservabilityState) projectioncontract.RunObservability {
	return projectioncontract.CloneRunObservability(projectioncontract.RunObservability{
		Run:                     appmodelcore.ProjectRun(state.Run),
		Outcome:                 state.Outcome,
		Progress:                state.Progress,
		TaskProgress:            state.TaskProgress,
		RepairLoop:              state.RepairLoop,
		Acceptance:              state.Acceptance,
		TaskDecompositions:      state.TaskDecompositions,
		TaskPlans:               state.TaskPlans,
		ExecutionSections:       state.ExecutionSections,
		LLMContextBudgets:       state.LLMContextBudgets,
		LLMContextBudgetSummary: state.LLMContextBudgetSummary,
		LLMUsageTraces:          state.LLMUsageTraces,
		LLMUsageSummary:         state.LLMUsageSummary,
		MemoryRecallTraces:      state.MemoryRecallTraces,
		GitOperations:           state.GitOperations,
		WebSources:              state.WebSources,
		ClosureTraces:           state.ClosureTraces,
		DoDEvidence:             state.DoDEvidence,
		ResultQuality:           state.ResultQuality,
		Evidence:                state.Evidence,
		LatestCheckpoint:        state.LatestCheckpoint,
		SelectedCapabilities:    state.SelectedCapabilities,
		UsedCapabilities:        state.UsedCapabilities,
		Timeline:                state.Timeline,
	})
}

func PublicWorkspaceExecutionActivation(state readmodelcore.WorkspaceExecutionActivationState) projectioncontract.WorkspaceExecutionActivation {
	return projectioncontract.WorkspaceExecutionActivation{
		Workspace: appmodelcore.ProjectWorkspace(state.Workspace),
		Run:       appmodelcore.ProjectRun(state.Run),
		Snapshot:  PublicWorkspaceSnapshot(state.Snapshot),
	}
}
