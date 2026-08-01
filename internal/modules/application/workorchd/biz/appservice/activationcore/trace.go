package activationcore

import (
	"fmt"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func BuildRuntimeActivationMemoryRecallTrace(runID common.RunID, workspaceID common.WorkspaceID, bootstrap *runtimecontract.RuntimeBootstrap, occurredAt string) *memorymodel.MemoryRecallTrace {
	if runID == "" || workspaceID == "" || bootstrap == nil {
		return nil
	}
	activation := bootstrap.Activation.Normalized()
	if len(activation.Memories) == 0 {
		return nil
	}
	traceID := common.MemoryRecallTraceID(fmt.Sprintf("%s/memory-recall/activation", runID))
	ids := make([]common.MemoryID, 0, len(activation.Memories))
	hits := make([]memorymodel.MemoryRecallHit, 0, len(activation.Memories))
	for _, item := range activation.Memories {
		ids = append(ids, item.ID)
		hits = append(hits, memorymodel.MemoryRecallHit{ID: item.ID, Summary: item.Summary, VectorScore: item.VectorScore})
	}
	return &memorymodel.MemoryRecallTrace{
		ID: traceID, WorkspaceID: workspaceID, RunID: runID, Stage: "planning", Source: "activation",
		Policy: "runtime_typed_activation", Status: "selected", Operation: "typed_activation_snapshot", RefreshSource: "activation",
		SelectedMemoryIDs: append([]common.MemoryID(nil), ids...), RetrievedMemoryIDs: ids, RetrievedMemoryHits: hits,
		OccurredAt: occurredAt, EvidenceRef: string(traceID),
	}
}
