package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: artifact_continuation.go

func projectArtifactContinuation(item *orchestratormodel.ArtifactContinuation) projectioncontract.ArtifactContinuation {
	if item == nil {
		return projectioncontract.ArtifactContinuation{}
	}
	return projectioncontract.ArtifactContinuation{
		ID:                string(item.ID),
		WorkspaceID:       string(item.WorkspaceID),
		RunID:             string(item.RunID),
		RootStepID:        string(item.RootStepID),
		ArtifactID:        item.ArtifactID,
		TargetRef:         item.TargetRef,
		Operation:         item.Operation,
		State:             string(item.State),
		ExpectedSequence:  item.ExpectedSequence,
		ReceivedSequences: append([]int(nil), item.ReceivedSequences...),
		FinalReceived:     item.FinalReceived,
		BytesWritten:      item.BytesWritten,
		RollingHash:       item.RollingHash,
		LastChunkHash:     item.LastChunkHash,
		CoverageTargets:   cloneReadModelStringSlice(item.CoverageTargets),
		EvidenceRefs:      cloneReadModelStringSlice(item.EvidenceRefs),
		FailureReason:     artifactContinuationFailureReason(item),
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
		Progress:          artifactContinuationProgress(item),
	}
}

// source: artifact_continuation_list.go

func mapArtifactContinuations(items []*orchestratormodel.ArtifactContinuation) []projectioncontract.ArtifactContinuation {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.ArtifactContinuation, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectArtifactContinuation(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
