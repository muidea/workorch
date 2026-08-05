package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func PortWorkspaceSnapshotInfo(snapshot readmodelcore.WorkspaceSnapshotState) port.WorkspaceSnapshotInfo {
	ret := port.WorkspaceSnapshotInfo{
		Workspace:               WorkspaceProjection(snapshot.Workspace),
		ViewModel:               projectioncontract.CloneWorkspaceViewModel(snapshot.ViewModel),
		Projection:              port.ProjectionStatusFromWorkspaceViewModel(snapshot.ViewModel),
		Step:                    projectioncontract.CloneStepView(snapshot.Step),
		Outcome:                 projectioncontract.CloneWorkspaceOutcome(snapshot.Outcome),
		TaskProgress:            projectioncontract.CloneTaskProgress(snapshot.TaskProgress),
		RunHeartbeat:            snapshot.RunHeartbeat,
		ConvergenceStatus:       projectioncontract.CloneConvergenceStatus(snapshot.ConvergenceStatus),
		RunCommand:              projectioncontract.CloneRunCommandStatus(snapshot.RunCommand),
		SchedulerDiagnostics:    projectioncontract.CloneSchedulerDiagnostics(snapshot.SchedulerDiagnostics),
		ContextBundle:           projectioncontract.CloneContextBundle(snapshot.ContextBundle),
		CoverageSummary:         portPayload(snapshot.CoverageSummary),
		GapBaseline:             projectioncontract.CloneGapBaseline(snapshot.GapBaseline),
		AuthorityDiagnostics:    projectioncontract.CloneGapDiagnostics(snapshot.AuthorityDiagnostics),
		Acceptance:              projectioncontract.CloneAcceptanceProjection(snapshot.Acceptance),
		TaskDecompositions:      projectioncontract.CloneTaskDecompositions(snapshot.TaskDecompositions),
		TaskPlans:               projectioncontract.CloneTaskPlans(snapshot.TaskPlans),
		ExecutionSections:       port.ExecutionSectionsFromProjection(snapshot.ExecutionSections),
		Goals:                   workorchmanagementcontract.CloneGoals(snapshot.Goals),
		DoDs:                    workorchmanagementcontract.CloneDoDs(snapshot.DoDs),
		Runs:                    RunProjections(snapshot.Runs),
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
		run := RunProjection(*snapshot.Run)
		ret.Run = &run
	}
	if snapshot.Result != nil {
		result := projectioncontract.CloneResult(*snapshot.Result)
		ret.Result = &result
	}
	return ret
}

func WorkspaceProjection(item appmodelcore.Workspace) projectioncontract.Workspace {
	return projectioncontract.CloneWorkspace(projectioncontract.Workspace{
		ID:                   item.ID,
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		State:                item.State,
		ActiveRunID:          item.ActiveRunID,
		BlockingTicketID:     item.BlockingTicketID,
		BlockingApprovalID:   item.BlockingApprovalID,
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultLLMTarget:     runtimecontract.CloneProviderModelTarget(item.DefaultLLMTarget),
		DefaultSoulID:        item.DefaultSoulID,
		SoulBindings:         item.SoulBindings,
		ContextHints:         item.ContextHints,
		EvidencePolicies:     item.EvidencePolicies,
		Projection:           port.ProjectionStatusFromWorkspaceViewModel(item.ViewModel),
		ViewModel:            projectioncontract.CloneWorkspaceViewModel(item.ViewModel),
	})
}

func WorkspaceProjections(items []appmodelcore.Workspace) []projectioncontract.Workspace {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.Workspace, 0, len(items))
	for _, item := range items {
		ret = append(ret, WorkspaceProjection(item))
	}
	return ret
}

func RunProjection(item appmodelcore.Run) projectioncontract.Run {
	return projectioncontract.CloneRun(projectioncontract.Run{
		ID: item.ID, WorkspaceID: item.WorkspaceID, GoalID: item.GoalID, DoDID: item.DoDID,
		Lineage: item.Lineage, DefinitionRevision: item.DefinitionRevision, InitialStepKind: item.InitialStepKind,
		State: item.State, CurrentStepID: item.CurrentStepID, CurrentStepKind: item.CurrentStepKind, CurrentStepState: item.CurrentStepState,
		Priority: item.Priority, BlockingTicketID: item.BlockingTicketID, BlockingApprovalID: item.BlockingApprovalID,
		FailureReason: item.FailureReason, SealedLLMTarget: item.SealedLLMTarget, RuntimeActivation: item.RuntimeActivation,
		CreatedAt: item.CreatedAt, StartedAt: item.StartedAt, EndedAt: item.EndedAt,
	})
}

func RunProjections(items []appmodelcore.Run) []projectioncontract.Run {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.Run, 0, len(items))
	for _, item := range items {
		ret = append(ret, RunProjection(item))
	}
	return ret
}

func portPayload(input map[string]any) workorcheventpayload.Payload {
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		return workorcheventpayload.Payload{}
	}
	return payload
}
