package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func WorkspaceSnapshotProjection(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.WorkspaceSnapshot {
	ret := projectioncontract.WorkspaceSnapshot{
		Workspace:               appmodelcore.ProjectWorkspace(snapshot.Workspace),
		ViewModel:               projectioncontract.CloneWorkspaceViewModel(snapshot.ViewModel),
		Step:                    projectioncontract.CloneStepView(snapshot.Step),
		DisplayOutcome:          projectioncontract.CloneWorkspaceDisplayOutcome(snapshot.DisplayOutcome),
		GovernanceDisplay:       snapshot.GovernanceDisplay,
		ConsistencyDiagnostic:   projectioncontract.CloneConsistencyDiagnostic(snapshot.ConsistencyDiagnostic),
		TaskProgress:            projectioncontract.CloneTaskProgress(snapshot.TaskProgress),
		RunHeartbeat:            snapshot.RunHeartbeat,
		ConvergenceStatus:       projectioncontract.CloneConvergenceStatus(snapshot.ConvergenceStatus),
		RunCommand:              projectioncontract.CloneRunCommandStatus(snapshot.RunCommand),
		ContextBundle:           projectioncontract.CloneContextBundle(snapshot.ContextBundle),
		CoverageSummary:         snapshot.CoverageSummary,
		GapBaseline:             projectioncontract.CloneGapBaseline(snapshot.GapBaseline),
		AuthorityDiagnostics:    projectioncontract.CloneGapDiagnostics(snapshot.AuthorityDiagnostics),
		Acceptance:              projectioncontract.CloneAcceptanceProjection(snapshot.Acceptance),
		TaskDecompositions:      projectioncontract.CloneTaskDecompositions(snapshot.TaskDecompositions),
		TaskPlans:               projectioncontract.CloneTaskPlans(snapshot.TaskPlans),
		ExecutionSections:       projectioncontract.CloneExecutionSections(snapshot.ExecutionSections),
		Goals:                   workorchmanagementcontract.CloneGoals(snapshot.Goals),
		DoDs:                    workorchmanagementcontract.CloneDoDs(snapshot.DoDs),
		Runs:                    appmodelcore.ProjectRuns(snapshot.Runs),
		Results:                 projectioncontract.CloneResults(snapshot.Results),
		Artifacts:               projectioncontract.CloneArtifacts(snapshot.Artifacts),
		ArtifactContinuations:   projectioncontract.CloneArtifactContinuations(snapshot.ArtifactContinuations),
		Patches:                 projectioncontract.ClonePatches(snapshot.Patches),
		MemoryRecallTraces:      projectioncontract.CloneMemoryRecallTraces(snapshot.MemoryRecallTraces),
		WebSources:              projectioncontract.CloneWebSourceTraces(snapshot.WebSources),
		LLMContextBudgets:       projectioncontract.CloneLLMContextBudgetTraces(snapshot.LLMContextBudgets),
		LLMContextBudgetSummary: snapshot.LLMContextBudgetSummary,
		LLMUsageTraces:          projectioncontract.CloneLLMUsageTraces(snapshot.LLMUsageTraces),
		LLMUsageSummary:         projectioncontract.CloneLLMUsageSummary(snapshot.LLMUsageSummary),
		EventRecords:            projectioncontract.CloneEventRecords(snapshot.EventRecords),
		Decisions:               projectioncontract.CloneDecisions(snapshot.Decisions),
		Approvals:               projectioncontract.CloneApprovals(snapshot.Approvals),
	}
	if snapshot.Run != nil {
		run := appmodelcore.ProjectRun(*snapshot.Run)
		ret.Run = &run
	}
	if snapshot.Result != nil {
		result := projectioncontract.CloneResult(*snapshot.Result)
		ret.Result = &result
	}
	return projectioncontract.CloneWorkspaceSnapshot(ret)
}
