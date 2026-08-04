package readmodelprojection

import (
	"maps"
	"slices"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// PublicRunSnapshot projects application-private read-model state to the
// protocol DTO. Private workspace and run state never crosses this boundary.
func PublicRunSnapshot(state readmodelcore.RunSnapshotState) projectioncontract.RunSnapshot {
	return projectioncontract.RunSnapshot{
		Workspace:               appmodelcore.ProjectWorkspace(state.Workspace),
		Run:                     appmodelcore.ProjectRun(state.Run),
		Outcome:                 state.Outcome,
		TaskProgress:            state.TaskProgress,
		RepairLoop:              state.RepairLoop,
		TaskDecompositions:      slices.Clone(state.TaskDecompositions),
		TaskPlans:               slices.Clone(state.TaskPlans),
		Step:                    clonePointer(state.Step),
		Steps:                   slices.Clone(state.Steps),
		ToolCalls:               slices.Clone(state.ToolCalls),
		Patches:                 slices.Clone(state.Patches),
		GitOperations:           slices.Clone(state.GitOperations),
		WebSources:              slices.Clone(state.WebSources),
		ValidationResults:       slices.Clone(state.ValidationResults),
		RuntimeCheckpoints:      slices.Clone(state.RuntimeCheckpoints),
		ConvergenceStatus:       clonePointer(state.ConvergenceStatus),
		SchedulerDiagnostics:    clonePointer(state.SchedulerDiagnostics),
		Results:                 slices.Clone(state.Results),
		ClosureTraces:           slices.Clone(state.ClosureTraces),
		DoDEvidence:             slices.Clone(state.DoDEvidence),
		ResultQuality:           slices.Clone(state.ResultQuality),
		Artifacts:               slices.Clone(state.Artifacts),
		ArtifactContinuations:   slices.Clone(state.ArtifactContinuations),
		ExecutionSections:       slices.Clone(state.ExecutionSections),
		LLMContextBudgets:       slices.Clone(state.LLMContextBudgets),
		LLMContextBudgetSummary: state.LLMContextBudgetSummary,
		LLMUsageTraces:          slices.Clone(state.LLMUsageTraces),
		LLMUsageSummary:         state.LLMUsageSummary,
		CoverageWorkItems:       cloneAnyMapSlice(state.CoverageWorkItems),
		CoverageWorkItemContext: maps.Clone(state.CoverageWorkItemContext),
		GapBaseline:             clonePointer(state.GapBaseline),
		AuthorityDiagnostics:    slices.Clone(state.AuthorityDiagnostics),
		Acceptance:              state.Acceptance,
		MemoryRecallTraces:      slices.Clone(state.MemoryRecallTraces),
		SelectedCapabilities:    slices.Clone(state.SelectedCapabilities),
		UsedCapabilities:        slices.Clone(state.UsedCapabilities),
		EventRecords:            slices.Clone(state.EventRecords),
		Decisions:               slices.Clone(state.Decisions),
		Approvals:               slices.Clone(state.Approvals),
		Observed:                slices.Clone(state.Observed),
	}
}

// PublicWorkspaceSnapshot projects application-private read-model state to
// the protocol DTO while preserving every public observation.
func PublicWorkspaceSnapshot(state readmodelcore.WorkspaceSnapshotState) projectioncontract.WorkspaceSnapshot {
	ret := projectioncontract.WorkspaceSnapshot{
		Workspace:               appmodelcore.ProjectWorkspace(state.Workspace),
		ViewModel:               state.ViewModel,
		Step:                    clonePointer(state.Step),
		Result:                  clonePointer(state.Result),
		Outcome:                 state.Outcome,
		TaskProgress:            state.TaskProgress,
		RunHeartbeat:            state.RunHeartbeat,
		ConvergenceStatus:       clonePointer(state.ConvergenceStatus),
		RunCommand:              clonePointer(state.RunCommand),
		SchedulerDiagnostics:    clonePointer(state.SchedulerDiagnostics),
		ContextBundle:           projectioncontract.CloneContextBundle(state.ContextBundle),
		CoverageSummary:         maps.Clone(state.CoverageSummary),
		GapBaseline:             clonePointer(state.GapBaseline),
		AuthorityDiagnostics:    slices.Clone(state.AuthorityDiagnostics),
		Acceptance:              state.Acceptance,
		TaskDecompositions:      slices.Clone(state.TaskDecompositions),
		TaskPlans:               slices.Clone(state.TaskPlans),
		ExecutionSections:       slices.Clone(state.ExecutionSections),
		MemoryRecallTraces:      slices.Clone(state.MemoryRecallTraces),
		Goals:                   slices.Clone(state.Goals),
		DoDs:                    slices.Clone(state.DoDs),
		Runs:                    appmodelcore.ProjectRuns(state.Runs),
		Results:                 slices.Clone(state.Results),
		Artifacts:               slices.Clone(state.Artifacts),
		ArtifactContinuations:   slices.Clone(state.ArtifactContinuations),
		Patches:                 slices.Clone(state.Patches),
		WebSources:              slices.Clone(state.WebSources),
		LLMContextBudgets:       slices.Clone(state.LLMContextBudgets),
		LLMContextBudgetSummary: state.LLMContextBudgetSummary,
		LLMUsageTraces:          slices.Clone(state.LLMUsageTraces),
		LLMUsageSummary:         state.LLMUsageSummary,
		EventRecords:            slices.Clone(state.EventRecords),
		Decisions:               slices.Clone(state.Decisions),
		Approvals:               slices.Clone(state.Approvals),
	}
	if state.Run != nil {
		run := appmodelcore.ProjectRun(*state.Run)
		ret.Run = &run
	}
	return ret
}

func PublicRunObservability(state readmodelcore.RunObservabilityState) projectioncontract.RunObservability {
	return projectioncontract.RunObservability{
		Run:                     appmodelcore.ProjectRun(state.Run),
		Outcome:                 state.Outcome,
		Progress:                state.Progress,
		TaskProgress:            state.TaskProgress,
		RepairLoop:              state.RepairLoop,
		Acceptance:              state.Acceptance,
		TaskDecompositions:      slices.Clone(state.TaskDecompositions),
		TaskPlans:               slices.Clone(state.TaskPlans),
		ExecutionSections:       slices.Clone(state.ExecutionSections),
		LLMContextBudgets:       slices.Clone(state.LLMContextBudgets),
		LLMContextBudgetSummary: state.LLMContextBudgetSummary,
		LLMUsageTraces:          slices.Clone(state.LLMUsageTraces),
		LLMUsageSummary:         state.LLMUsageSummary,
		MemoryRecallTraces:      slices.Clone(state.MemoryRecallTraces),
		GitOperations:           slices.Clone(state.GitOperations),
		WebSources:              slices.Clone(state.WebSources),
		ClosureTraces:           slices.Clone(state.ClosureTraces),
		DoDEvidence:             slices.Clone(state.DoDEvidence),
		ResultQuality:           slices.Clone(state.ResultQuality),
		Evidence:                state.Evidence,
		LatestCheckpoint:        clonePointer(state.LatestCheckpoint),
		SelectedCapabilities:    slices.Clone(state.SelectedCapabilities),
		UsedCapabilities:        slices.Clone(state.UsedCapabilities),
		Timeline:                slices.Clone(state.Timeline),
	}
}

func PublicWorkspaceExecutionActivation(state readmodelcore.WorkspaceExecutionActivationState) projectioncontract.WorkspaceExecutionActivation {
	return projectioncontract.WorkspaceExecutionActivation{
		Workspace: appmodelcore.ProjectWorkspace(state.Workspace),
		Run:       appmodelcore.ProjectRun(state.Run),
		Snapshot:  PublicWorkspaceSnapshot(state.Snapshot),
	}
}

func clonePointer[T any](input *T) *T {
	if input == nil {
		return nil
	}
	ret := *input
	return &ret
}

func cloneAnyMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, maps.Clone(item))
	}
	return ret
}
