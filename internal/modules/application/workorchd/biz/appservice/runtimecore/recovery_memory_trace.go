package runtimecore

import (
	"fmt"
	"strings"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func BuildRecoveryMemoryRecallTrace(item *runmodel.Run, compressed map[string]any, existing []*memorymodel.MemoryRecallTrace, occurredAt string) *memorymodel.MemoryRecallTrace {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || len(compressed) == 0 {
		return nil
	}
	sourceRefs := StringSliceFromAny(compressed["sourceSectionRefs"])
	if !ContainsString(sourceRefs, "memory_recall") {
		return nil
	}
	memoryIDs := make([]common.MemoryID, 0)
	sourceTraceIDs := make([]string, 0)
	status := "inherited"
	for _, trace := range existing {
		if trace == nil || trace.Source == "recovery" {
			continue
		}
		sourceTraceIDs = append(sourceTraceIDs, string(trace.ID), trace.EvidenceRef)
		memoryIDs = append(memoryIDs, trace.RetrievedMemoryIDs...)
		if trace.Status == "degraded" {
			status = "degraded"
		}
	}
	memoryIDs = CompactMemoryIDs(memoryIDs)
	if len(memoryIDs) == 0 && status != "degraded" {
		return nil
	}
	traceID := common.MemoryRecallTraceID(fmt.Sprintf("%s/memory-recall/recovery", item.ID))
	trace := &memorymodel.MemoryRecallTrace{
		ID:                 traceID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.ID,
		Stage:              "recovery",
		Source:             "recovery",
		Policy:             "inherit_compressed_context",
		Status:             status,
		RetrievedMemoryIDs: memoryIDs,
		Reason:             "compressed context inherited memory recall evidence",
		OccurredAt:         occurredAt,
		EvidenceRef:        string(traceID),
	}
	if refs := CompactStringRefs(sourceTraceIDs); len(refs) > 0 {
		trace.Reason += ": " + strings.Join(refs, ", ")
	}
	return trace
}
