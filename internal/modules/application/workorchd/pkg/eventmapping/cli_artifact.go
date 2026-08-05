package eventmapping

import (
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func artifactDTOs(items []projectioncontract.Artifact) ([]workorchdevents.CLIArtifactInfo, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchdevents.CLIArtifactInfo, 0, len(items))
	for _, item := range items {
		metadata, err := workorcheventpayload.FromMap(item.Metadata)
		if err != nil {
			return nil, err
		}
		ret = append(ret, workorchdevents.CLIArtifactInfo{
			ID: common.ArtifactID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), RunID: common.RunID(item.RunID),
			ResultID: common.ResultID(item.ResultID), StepID: common.StepID(item.StepID), ToolCallID: common.ToolCallID(item.ToolCallID),
			PatchID: common.PatchID(item.PatchID), ValidationResultID: common.ValidationResultID(item.ValidationResultID),
			ArtifactType: item.ArtifactType, Category: item.Category, Name: item.Name, Summary: item.Summary, URI: item.URI,
			ContentType: item.ContentType, State: common.ArtifactState(item.State), EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
			Metadata: metadata, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	return ret, nil
}

func artifactProjections(items []workorchdevents.CLIArtifactInfo) []projectioncontract.Artifact {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.Artifact, 0, len(items))
	for _, item := range items {
		ret = append(ret, projectioncontract.Artifact{
			ID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), ResultID: string(item.ResultID),
			StepID: string(item.StepID), ToolCallID: string(item.ToolCallID), PatchID: string(item.PatchID), ValidationResultID: string(item.ValidationResultID),
			ArtifactType: item.ArtifactType, Category: item.Category, Name: item.Name, Summary: item.Summary, URI: item.URI,
			ContentType: item.ContentType, State: string(item.State), EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
			Metadata: item.Metadata.Map(), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	return ret
}

func artifactContinuationDTOs(items []projectioncontract.ArtifactContinuation) ([]workorchdevents.CLIArtifactContinuationInfo, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchdevents.CLIArtifactContinuationInfo, 0, len(items))
	for _, item := range items {
		progress, err := workorcheventpayload.FromMap(item.Progress)
		if err != nil {
			return nil, err
		}
		ret = append(ret, workorchdevents.CLIArtifactContinuationInfo{
			ID: common.ArtifactContinuationID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), RunID: common.RunID(item.RunID), RootStepID: common.StepID(item.RootStepID),
			ArtifactID: item.ArtifactID, TargetRef: item.TargetRef, Operation: item.Operation, State: common.ArtifactContinuationState(item.State),
			ExpectedSequence: item.ExpectedSequence, ReceivedSequences: append([]int(nil), item.ReceivedSequences...), FinalReceived: item.FinalReceived,
			BytesWritten: item.BytesWritten, RollingHash: item.RollingHash, LastChunkHash: item.LastChunkHash,
			CoverageTargets: append([]string(nil), item.CoverageTargets...), EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
			FailureReason: item.FailureReason, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Progress: progress,
		})
	}
	return ret, nil
}

func artifactContinuationProjections(items []workorchdevents.CLIArtifactContinuationInfo) []projectioncontract.ArtifactContinuation {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.ArtifactContinuation, 0, len(items))
	for _, item := range items {
		ret = append(ret, projectioncontract.ArtifactContinuation{
			ID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), RootStepID: string(item.RootStepID),
			ArtifactID: item.ArtifactID, TargetRef: item.TargetRef, Operation: item.Operation, State: string(item.State),
			ExpectedSequence: item.ExpectedSequence, ReceivedSequences: append([]int(nil), item.ReceivedSequences...), FinalReceived: item.FinalReceived,
			BytesWritten: item.BytesWritten, RollingHash: item.RollingHash, LastChunkHash: item.LastChunkHash,
			CoverageTargets: append([]string(nil), item.CoverageTargets...), EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
			FailureReason: item.FailureReason, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Progress: item.Progress.Map(),
		})
	}
	return ret
}

func patchDTOs(items []projectioncontract.Patch) ([]workorchdevents.CLIPatchInfo, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchdevents.CLIPatchInfo, 0, len(items))
	for _, item := range items {
		diagnostics, err := workorcheventpayload.FromMapSlice(item.Diagnostics)
		if err != nil {
			return nil, err
		}
		ret = append(ret, workorchdevents.CLIPatchInfo{
			ID: common.PatchID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), RunID: common.RunID(item.RunID),
			StepID: common.StepID(item.StepID), ToolCallID: common.ToolCallID(item.ToolCallID), ApprovalID: common.ApprovalID(item.ApprovalID), RevertApprovalID: common.ApprovalID(item.RevertApprovalID),
			PatchType: item.PatchType, TargetRef: item.TargetRef, Summary: item.Summary, State: common.PatchState(item.State),
			AddedLines: item.AddedLines, DeletedLines: item.DeletedLines, BeforeHash: item.BeforeHash, AfterHash: item.AfterHash,
			AffectedFiles: append([]string(nil), item.AffectedFiles...), RiskLevel: item.RiskLevel, RiskReasons: append([]string(nil), item.RiskReasons...),
			Diagnostics: diagnostics, CreatedAt: item.CreatedAt, AppliedAt: item.AppliedAt, RevertedAt: item.RevertedAt,
		})
	}
	return ret, nil
}

func patchProjections(items []workorchdevents.CLIPatchInfo) []projectioncontract.Patch {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.Patch, 0, len(items))
	for _, item := range items {
		ret = append(ret, projectioncontract.Patch{
			ID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), StepID: string(item.StepID),
			ToolCallID: string(item.ToolCallID), ApprovalID: string(item.ApprovalID), RevertApprovalID: string(item.RevertApprovalID),
			PatchType: item.PatchType, TargetRef: item.TargetRef, Summary: item.Summary, State: string(item.State),
			AddedLines: item.AddedLines, DeletedLines: item.DeletedLines, BeforeHash: item.BeforeHash, AfterHash: item.AfterHash,
			AffectedFiles: append([]string(nil), item.AffectedFiles...), RiskLevel: item.RiskLevel, RiskReasons: append([]string(nil), item.RiskReasons...),
			Diagnostics: workorcheventpayload.MapSlice(item.Diagnostics), CreatedAt: item.CreatedAt, AppliedAt: item.AppliedAt, RevertedAt: item.RevertedAt,
		})
	}
	return ret
}
