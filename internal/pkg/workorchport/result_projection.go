package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// ResultInfosFromProjection converts the immutable product projection into the
// client port shape without changing Result owner's already-judged outcome.
func ResultInfosFromProjection(items []projectioncontract.Result) []ResultInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ResultInfo, 0, len(items))
	for _, item := range items {
		var summary = item.SummaryDetail
		if summary != nil {
			cloned := summary.Clone()
			summary = &cloned
		}
		ret = append(ret, ResultInfo{
			ID: common.ResultID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID),
			RunID: common.RunID(item.RunID), GoalID: common.GoalID(item.GoalID), DoDID: common.DoDID(item.DoDID),
			State: common.ResultState(item.State), Title: item.Title, Summary: item.Summary,
			SourceType: item.SourceType, SourceRef: item.SourceRef,
			EvidenceRefs: cloneProjectionStrings(item.EvidenceRefs), ArtifactRefs: cloneProjectionStrings(item.ArtifactRefs),
			SummaryDetail: summary,
		})
	}
	return ret
}

func ArtifactInfosFromProjection(items []projectioncontract.Artifact) []ArtifactInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ArtifactInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, ArtifactInfo{
			ID: common.ArtifactID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID),
			RunID: common.RunID(item.RunID), ResultID: common.ResultID(item.ResultID),
			ArtifactType: item.ArtifactType, Category: item.Category, Name: item.Name, Summary: item.Summary,
			URI: item.URI, ContentType: item.ContentType, State: common.ArtifactState(item.State),
			EvidenceRefs: cloneProjectionStrings(item.EvidenceRefs),
		})
	}
	return ret
}

func ArtifactContinuationInfosFromProjection(items []projectioncontract.ArtifactContinuation) []ArtifactContinuationInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ArtifactContinuationInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, ArtifactContinuationInfo{
			ID: common.ArtifactContinuationID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID),
			RunID: common.RunID(item.RunID), RootStepID: common.StepID(item.RootStepID), ArtifactID: item.ArtifactID,
			TargetRef: item.TargetRef, Operation: item.Operation, State: common.ArtifactContinuationState(item.State),
			ExpectedSequence: item.ExpectedSequence, ReceivedSequences: append([]int(nil), item.ReceivedSequences...),
			FinalReceived: item.FinalReceived, BytesWritten: item.BytesWritten, RollingHash: item.RollingHash,
			LastChunkHash: item.LastChunkHash, CoverageTargets: cloneProjectionStrings(item.CoverageTargets),
			EvidenceRefs: cloneProjectionStrings(item.EvidenceRefs), FailureReason: item.FailureReason,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Progress: projectionPayload(item.Progress),
		})
	}
	return ret
}

func PatchInfosFromProjection(items []projectioncontract.Patch) []PatchInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]PatchInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PatchInfo{
			ID: common.PatchID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), RunID: common.RunID(item.RunID),
			StepID: common.StepID(item.StepID), ToolCallID: common.ToolCallID(item.ToolCallID),
			ApprovalID: common.ApprovalID(item.ApprovalID), RevertApprovalID: common.ApprovalID(item.RevertApprovalID),
			PatchType: item.PatchType, TargetRef: item.TargetRef, Summary: item.Summary, State: common.PatchState(item.State),
			AddedLines: item.AddedLines, DeletedLines: item.DeletedLines, BeforeHash: item.BeforeHash, AfterHash: item.AfterHash,
			AffectedFiles: cloneProjectionStrings(item.AffectedFiles), RiskLevel: item.RiskLevel,
			RiskReasons: cloneProjectionStrings(item.RiskReasons), Diagnostics: projectionPayloads(item.Diagnostics),
			CreatedAt: item.CreatedAt, AppliedAt: item.AppliedAt, RevertedAt: item.RevertedAt,
		})
	}
	return ret
}

func projectionPayload(input map[string]any) workorcheventpayload.Payload {
	payload, _ := workorcheventpayload.FromMap(input)
	return payload
}

func projectionPayloads(input []map[string]any) []workorcheventpayload.Payload {
	payloads, _ := workorcheventpayload.FromMapSlice(input)
	return payloads
}

func cloneProjectionStrings(input []string) []string {
	return append([]string(nil), input...)
}
