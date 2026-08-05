package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func WorkspaceRuntimeViewInfoFromProjection(item projectioncontract.WorkspaceRuntimeView) WorkspaceRuntimeViewInfo {
	return CloneWorkspaceRuntimeViewInfo(WorkspaceRuntimeViewInfo{
		WorkspaceID: common.WorkspaceID(item.WorkspaceID), ActiveRunID: common.RunID(item.ActiveRunID),
		DisplayRunID: common.RunID(item.DisplayRunID), Projection: ProjectionStatusFromWorkspaceViewModel(item.Projection),
		Revision: item.Revision, PromptMode: item.PromptMode, RunState: common.RunState(item.RunState),
		TaskProgress: projectioncontract.CloneTaskProgress(item.TaskProgress), RunHeartbeat: item.RunHeartbeat,
		ConvergenceStatus: projectioncontract.CloneConvergenceStatus(item.ConvergenceStatus),
		Acceptance:        projectioncontract.CloneAcceptanceProjection(item.Acceptance), RunCommand: projectioncontract.CloneRunCommandStatus(item.RunCommand),
		BlockingDecision: cloneDecisionPtr(item.BlockingDecision), BlockingApproval: cloneApprovalPtr(item.BlockingApproval),
		Outcome: projectioncontract.CloneWorkspaceOutcome(item.Outcome), Activity: projectioncontract.CloneWorkspaceActivityFeed(item.Activity),
	})
}

func cloneDecisionPtr(item *projectioncontract.Decision) *projectioncontract.Decision {
	if item == nil {
		return nil
	}
	ret := projectioncontract.CloneDecision(*item)
	return &ret
}

func cloneApprovalPtr(item *projectioncontract.Approval) *projectioncontract.Approval {
	if item == nil {
		return nil
	}
	ret := projectioncontract.CloneApproval(*item)
	return &ret
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
