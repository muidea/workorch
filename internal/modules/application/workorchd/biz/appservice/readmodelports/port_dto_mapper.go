package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func PortWorkspaceSnapshotInfo(snapshot readmodelcore.WorkspaceSnapshotState) port.WorkspaceSnapshotInfo {
	ret := port.WorkspaceSnapshotInfo{
		Workspace:               PortWorkspaceInfo(snapshot.Workspace),
		ViewModel:               port.WorkspaceViewModelInfoFromProjection(snapshot.ViewModel),
		Projection:              port.ProjectionStatusFromWorkspaceViewModel(snapshot.ViewModel),
		Step:                    port.StepViewInfoFromProjection(snapshot.Step),
		Outcome:                 port.WorkspaceOutcomeInfoFromProjection(snapshot.Outcome),
		TaskProgress:            port.TaskProgressInfoFromProjection(snapshot.TaskProgress),
		RunHeartbeat:            port.RunHeartbeatInfoFromProjection(snapshot.RunHeartbeat),
		ConvergenceStatus:       port.ConvergenceStatusInfoFromProjection(snapshot.ConvergenceStatus),
		RunCommand:              port.RunCommandStatusInfoFromProjection(snapshot.RunCommand),
		SchedulerDiagnostics:    projectioncontract.CloneSchedulerDiagnostics(snapshot.SchedulerDiagnostics),
		ContextBundle:           projectioncontract.CloneContextBundle(snapshot.ContextBundle),
		CoverageSummary:         portPayload(snapshot.CoverageSummary),
		GapBaseline:             port.GapBaselineInfoFromProjection(snapshot.GapBaseline),
		AuthorityDiagnostics:    port.GapDiagnosticInfosFromProjection(snapshot.AuthorityDiagnostics),
		Acceptance:              port.AcceptanceProjectionInfoFromProjection(snapshot.Acceptance),
		TaskDecompositions:      port.TaskDecompositionInfosFromProjection(snapshot.TaskDecompositions),
		TaskPlans:               port.TaskPlanInfosFromProjection(snapshot.TaskPlans),
		ExecutionSections:       port.ExecutionSectionsFromProjection(snapshot.ExecutionSections),
		Goals:                   port.GoalInfosFromManagement(snapshot.Goals),
		DoDs:                    port.DoDInfosFromManagement(snapshot.DoDs),
		Runs:                    PortRunInfos(snapshot.Runs),
		Results:                 port.ResultInfosFromProjection(snapshot.Results),
		Artifacts:               port.ArtifactInfosFromProjection(snapshot.Artifacts),
		ArtifactContinuations:   port.ArtifactContinuationInfosFromProjection(snapshot.ArtifactContinuations),
		Patches:                 port.PatchInfosFromProjection(snapshot.Patches),
		MemoryRecallTraces:      port.MemoryRecallTraceInfosFromProjection(snapshot.MemoryRecallTraces),
		WebSources:              port.WebSourceTraceInfosFromProjection(snapshot.WebSources),
		LLMContextBudgets:       port.LLMContextBudgetTraceInfosFromProjection(snapshot.LLMContextBudgets),
		LLMContextBudgetSummary: snapshot.LLMContextBudgetSummary,
		LLMUsageTraces:          port.LLMUsageTraceInfosFromProjection(snapshot.LLMUsageTraces),
		LLMUsageSummary:         port.LLMUsageSummaryInfoFromProjection(snapshot.LLMUsageSummary),
		EventRecords:            port.EventRecordInfosFromProjection(snapshot.EventRecords),
		Decisions:               port.DecisionInfosFromProjection(snapshot.Decisions),
		Approvals:               port.ApprovalInfosFromProjection(snapshot.Approvals),
	}
	if snapshot.Run != nil {
		run := PortRunInfo(*snapshot.Run)
		ret.Run = &run
	}
	if snapshot.Result != nil {
		results := port.ResultInfosFromProjection([]projectioncontract.Result{*snapshot.Result})
		if len(results) != 0 {
			ret.Result = &results[0]
		}
	}
	return ret
}

func PortWorkspaceInfo(item appmodelcore.Workspace) port.WorkspaceInfo {
	return port.WorkspaceInfo{
		ID:                   common.WorkspaceID(item.ID),
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		State:                common.WorkspaceState(item.State),
		ActiveRunID:          common.RunID(item.ActiveRunID),
		BlockingTicketID:     common.TicketID(item.BlockingTicketID),
		BlockingApprovalID:   common.ApprovalID(item.BlockingApprovalID),
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultLLMTarget:     runtimecontract.CloneProviderModelTarget(item.DefaultLLMTarget),
		DefaultSoulID:        common.SoulID(item.DefaultSoulID),
		SoulBindings:         portSoulBindings(item.SoulBindings),
		ContextHints:         portPayload(item.ContextHints),
		EvidencePolicies:     portPayloadSlice(item.EvidencePolicies),
		Projection:           port.ProjectionStatusFromWorkspaceViewModel(item.ViewModel),
		ViewModel:            port.WorkspaceViewModelInfoFromProjection(item.ViewModel),
	}
}

func PortWorkspaceInfos(items []appmodelcore.Workspace) []port.WorkspaceInfo {
	ret := make([]port.WorkspaceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortWorkspaceInfo(item))
	}
	return ret
}

func PortRunInfo(item appmodelcore.Run) port.RunInfo {
	return port.RunInfo{
		ID:                  common.RunID(item.ID),
		WorkspaceID:         common.WorkspaceID(item.WorkspaceID),
		GoalID:              common.GoalID(item.GoalID),
		DoDID:               common.DoDID(item.DoDID),
		DefinitionRevision:  item.DefinitionRevision,
		PredecessorRunID:    common.RunID(item.Lineage.PredecessorRunID),
		PredecessorResultID: common.ResultID(item.Lineage.PredecessorResultID),
		SuccessorRunID:      common.RunID(item.Lineage.SuccessorRunID),
		State:               common.RunState(item.State),
		Priority:            item.Priority,
		BlockingTicketID:    common.TicketID(item.BlockingTicketID),
		BlockingApprovalID:  common.ApprovalID(item.BlockingApprovalID),
		FailureReason:       item.FailureReason,
		SealedLLMTarget:     runtimecontract.CloneProviderModelTarget(item.SealedLLMTarget),
		RuntimeActivation:   port.CloneRuntimeActivationStatus(item.RuntimeActivation),
	}
}

func PortRunInfos(items []appmodelcore.Run) []port.RunInfo {
	ret := make([]port.RunInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortRunInfo(item))
	}
	return ret
}

func portSoulBindings(input map[string]string) map[string]common.SoulID {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		ret[key] = common.SoulID(value)
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

func portPayloadSlice(input []map[string]any) []workorcheventpayload.Payload {
	payloads, err := workorcheventpayload.FromMapSlice(input)
	if err != nil {
		return nil
	}
	return payloads
}
