package activationcore

import (
	"fmt"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func BuildActivationMemoryRecallTrace(runID common.RunID, workspaceID common.WorkspaceID, executionContext map[string]any, occurredAt string) *memorymodel.MemoryRecallTrace {
	if runID == "" || workspaceID == "" {
		return nil
	}
	retrieval, _ := executionContext["memoryRetrieval"].(map[string]any)
	if len(retrieval) == 0 {
		return nil
	}
	traceID := common.MemoryRecallTraceID(fmt.Sprintf("%s/memory-recall/activation", runID))
	trace := &memorymodel.MemoryRecallTrace{
		ID:                  traceID,
		WorkspaceID:         workspaceID,
		RunID:               runID,
		Stage:               "planning",
		Source:              "activation",
		Policy:              TraceStringValue(retrieval["policy"]),
		Status:              TraceStringValue(retrieval["status"]),
		ProviderID:          TraceStringValue(retrieval["providerId"]),
		Operation:           TraceStringValue(retrieval["operation"]),
		QueryTextPreview:    TraceStringValue(retrieval["queryTextPreview"]),
		QueryTextSha256:     TraceStringValue(retrieval["queryTextSha256"]),
		MinScore:            TraceFloatValue(retrieval["minScore"]),
		Limit:               TraceIntValue(retrieval["limit"]),
		EmbeddingModel:      TraceStringValue(retrieval["embeddingModel"]),
		VectorDimension:     TraceIntValue(retrieval["vectorDimension"]),
		RefreshSource:       "activation",
		RetrievedMemoryIDs:  TraceMemoryIDs(retrieval["retrievedMemoryIds"]),
		RetrievedMemoryHits: TraceMemoryHits(retrieval["retrievedMemoryHits"]),
		Reason:              TraceStringValue(retrieval["reason"]),
		OccurredAt:          occurredAt,
		EvidenceRef:         string(traceID),
	}
	if len(trace.RetrievedMemoryIDs) == 0 {
		trace.RetrievedMemoryIDs = TraceMemoryIDs(executionContext["memoryIds"])
	}
	trace.SelectedMemoryIDs = append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...)
	return trace
}
