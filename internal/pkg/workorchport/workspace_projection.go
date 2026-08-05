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
