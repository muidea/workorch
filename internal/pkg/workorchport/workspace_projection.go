package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func WorkspaceViewModelInfoFromProjection(item projectioncontract.WorkspaceViewModel) WorkspaceViewModelInfo {
	return WorkspaceViewModelInfo{
		WorkspaceID: common.WorkspaceID(item.WorkspaceID), State: item.State, PromptMode: item.PromptMode,
		Revision: item.Revision, StreamGeneration: item.StreamGeneration, GlobalRevision: item.GlobalRevision,
		WorkspaceRevision: item.WorkspaceRevision, SourceEventRevision: item.SourceEventRevision,
		ProjectedSourceRevision: item.ProjectedSourceRevision, UpdatedAt: item.UpdatedAt, RebuiltAt: item.RebuiltAt,
		Degraded: item.Degraded, StaleReason: item.StaleReason, Error: item.Error,
		FailedWorkspaceIds: cloneProjectionStrings(item.FailedWorkspaceIds), DegradedSince: item.DegradedSince,
		Sections: cloneWorkspaceViewModelSections(item.Sections),
	}
}

func WorkspaceRuntimeViewInfoFromProjection(item projectioncontract.WorkspaceRuntimeView) WorkspaceRuntimeViewInfo {
	return WorkspaceRuntimeViewInfo{
		WorkspaceID: common.WorkspaceID(item.WorkspaceID), ActiveRunID: common.RunID(item.ActiveRunID),
		DisplayRunID: common.RunID(item.DisplayRunID), Projection: ProjectionStatusFromWorkspaceViewModel(item.Projection),
		Revision: item.Revision, PromptMode: item.PromptMode, RunState: common.RunState(item.RunState),
		TaskProgress: TaskProgressInfoFromProjection(item.TaskProgress), RunHeartbeat: RunHeartbeatInfoFromProjection(item.RunHeartbeat),
		ConvergenceStatus: ConvergenceStatusInfoFromProjection(item.ConvergenceStatus),
		Acceptance:        AcceptanceProjectionInfoFromProjection(item.Acceptance), RunCommand: RunCommandStatusInfoFromProjection(item.RunCommand),
		BlockingDecision: DecisionInfoPtrFromProjection(item.BlockingDecision), BlockingApproval: ApprovalInfoPtrFromProjection(item.BlockingApproval),
		Outcome: WorkspaceOutcomeInfoFromProjection(item.Outcome), Activity: WorkspaceActivityFeedInfoFromProjection(item.Activity),
	}
}

func ProjectionStatusFromWorkspaceViewModel(item projectioncontract.WorkspaceViewModel) projectioncontract.ProjectionStatus {
	return projectioncontract.ProjectionStatus{
		State: item.State, StreamGeneration: item.StreamGeneration, GlobalRevision: item.GlobalRevision,
		WorkspaceRevision: item.WorkspaceRevision, SourceEventRevision: item.SourceEventRevision,
		ProjectedSourceRevision: item.ProjectedSourceRevision, UpdatedAt: item.UpdatedAt, RebuiltAt: item.RebuiltAt,
		Degraded: item.Degraded, StaleReason: item.StaleReason, Error: item.Error,
		FailedWorkspaceIds: cloneProjectionStrings(item.FailedWorkspaceIds), DegradedSince: item.DegradedSince,
	}
}

func CloneProjectionStatus(item projectioncontract.ProjectionStatus) projectioncontract.ProjectionStatus {
	item.FailedWorkspaceIds = cloneProjectionStrings(item.FailedWorkspaceIds)
	return item
}

func GapBaselineInfoFromProjection(item *projectioncontract.GapBaseline) *GapBaselineInfo {
	if item == nil {
		return nil
	}
	ret := &GapBaselineInfo{
		WorkspaceID: common.WorkspaceID(item.WorkspaceID), RunID: common.RunID(item.RunID),
		DefinitionRevision: item.DefinitionRevision, BaselineRevision: item.BaselineRevision,
		CurrentFocus: projectionPayload(item.CurrentFocus), OpenCoverageWorkItems: projectionPayloads(item.OpenCoverageWorkItems),
		AcceptedEvidenceRefs: cloneProjectionStrings(item.AcceptedEvidenceRefs),
		RejectedEvidenceRefs: cloneProjectionStrings(item.RejectedEvidenceRefs),
		NextAction:           projectioncontract.GapNextAction{Kind: item.NextAction.Kind, Reason: item.NextAction.Reason},
	}
	for _, todo := range item.TodoItems {
		ret.TodoItems = append(ret.TodoItems, GapTodoItemInfo{
			ID: todo.ID, Source: todo.Source, SourceRef: todo.SourceRef,
			CoverageWorkItemIDs:   cloneProjectionStrings(todo.CoverageWorkItemIDs),
			RequiredEvidenceKinds: cloneProjectionStrings(todo.RequiredEvidenceKinds),
			Status:                todo.Status, Priority: todo.Priority, Reason: todo.Reason, Metadata: projectionPayload(todo.Metadata),
		})
	}
	for _, diagnostic := range item.Diagnostics {
		ret.Diagnostics = append(ret.Diagnostics, GapDiagnosticInfo{
			Code: diagnostic.Code, Kind: diagnostic.Kind, Severity: diagnostic.Severity,
			Message: diagnostic.Message, Source: diagnostic.Source, Candidate: diagnostic.Candidate,
			CurrentAuthority: diagnostic.CurrentAuthority,
		})
	}
	return ret
}

func GapDiagnosticInfosFromProjection(items []projectioncontract.GapDiagnostic) []GapDiagnosticInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]GapDiagnosticInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, GapDiagnosticInfo{Code: item.Code, Kind: item.Kind, Severity: item.Severity, Message: item.Message, Source: item.Source, Candidate: item.Candidate, CurrentAuthority: item.CurrentAuthority})
	}
	return ret
}

func StepViewInfoFromProjection(item *projectioncontract.StepView) *StepViewInfo {
	if item == nil {
		return nil
	}
	return &StepViewInfo{ID: common.StepID(item.ID), Kind: item.Kind, State: item.State, Input: projectionPayload(item.Input), Output: projectionPayload(item.Output), ContractAudit: projectionPayload(item.ContractAudit)}
}

func ExecutionSectionsFromProjection(items []projectioncontract.ExecutionSection) []projectioncontract.ExecutionSection {
	ret := append([]projectioncontract.ExecutionSection(nil), items...)
	for idx := range ret {
		ret[idx].RelatedDoDItems = cloneProjectionStrings(items[idx].RelatedDoDItems)
		ret[idx].PlannedStepRefs = cloneProjectionStrings(items[idx].PlannedStepRefs)
		ret[idx].ActualEvidenceRefs = cloneProjectionStrings(items[idx].ActualEvidenceRefs)
		ret[idx].DecisionRefs = cloneProjectionStrings(items[idx].DecisionRefs)
		ret[idx].ApprovalRefs = cloneProjectionStrings(items[idx].ApprovalRefs)
		ret[idx].ToolCallRefs = cloneProjectionStrings(items[idx].ToolCallRefs)
		ret[idx].PatchRefs = cloneProjectionStrings(items[idx].PatchRefs)
		ret[idx].ValidationRefs = cloneProjectionStrings(items[idx].ValidationRefs)
		ret[idx].ResultRefs = cloneProjectionStrings(items[idx].ResultRefs)
		ret[idx].ArtifactRefs = cloneProjectionStrings(items[idx].ArtifactRefs)
		ret[idx].MemoryRecallRefs = cloneProjectionStrings(items[idx].MemoryRecallRefs)
		ret[idx].MemoryIDs = cloneProjectionStrings(items[idx].MemoryIDs)
	}
	return ret
}

func cloneWorkspaceViewModelSections(items map[string]projectioncontract.WorkspaceViewModelSection) map[string]projectioncontract.WorkspaceViewModelSection {
	if len(items) == 0 {
		return nil
	}
	ret := make(map[string]projectioncontract.WorkspaceViewModelSection, len(items))
	for key, item := range items {
		ret[key] = item
	}
	return ret
}
