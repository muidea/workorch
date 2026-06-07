package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ProjectArtifactContinuation(item *orchestratormodel.ArtifactContinuation) readmodelcore.ArtifactContinuation {
	if item == nil {
		return readmodelcore.ArtifactContinuation{}
	}
	return readmodelcore.ArtifactContinuation{
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
