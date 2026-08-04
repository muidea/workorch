package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ProjectArtifactContinuation(item *orchestratormodel.ArtifactContinuation) projectioncontract.ArtifactContinuation {
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
