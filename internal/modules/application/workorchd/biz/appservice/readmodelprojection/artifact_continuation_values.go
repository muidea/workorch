package readmodelprojection

import (
	"strings"

	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func artifactContinuationProgress(item *orchestratormodel.ArtifactContinuation) map[string]any {
	progress := map[string]any{}
	if item.ExpectedSequence > 0 {
		progress["expectedSequence"] = item.ExpectedSequence
	}
	if len(item.ReceivedSequences) > 0 {
		progress["receivedCount"] = len(item.ReceivedSequences)
		progress["lastReceivedSequence"] = item.ReceivedSequences[len(item.ReceivedSequences)-1]
	}
	if item.BytesWritten > 0 {
		progress["bytesWritten"] = item.BytesWritten
	}
	if len(progress) == 0 {
		return nil
	}
	return progress
}

func artifactContinuationFailureReason(item *orchestratormodel.ArtifactContinuation) string {
	if item == nil || len(item.Metadata.Fields) == 0 {
		return ""
	}
	if failure, _ := item.Metadata.String("failureError"); strings.TrimSpace(failure) != "" {
		return strings.TrimSpace(failure)
	}
	if failure, _ := item.Metadata.String("applyFailure"); strings.TrimSpace(failure) != "" {
		return strings.TrimSpace(failure)
	}
	return ""
}
