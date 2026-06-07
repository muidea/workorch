package ownercommandports

import (
	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func memoryRecallTraceCommandFromModel(trace *memorymodel.MemoryRecallTrace) memorycontracts.UpsertMemoryRecallTraceCommand {
	if trace == nil {
		return memorycontracts.UpsertMemoryRecallTraceCommand{}
	}
	return memorycontracts.UpsertMemoryRecallTraceCommand{
		TraceID:                trace.ID,
		WorkspaceID:            trace.WorkspaceID,
		RunID:                  trace.RunID,
		StepID:                 trace.StepID,
		GraphNodeID:            trace.GraphNodeID,
		Stage:                  trace.Stage,
		Source:                 trace.Source,
		Policy:                 trace.Policy,
		Status:                 trace.Status,
		ProviderID:             trace.ProviderID,
		Operation:              trace.Operation,
		QueryTextPreview:       trace.QueryTextPreview,
		QueryTextSha256:        trace.QueryTextSha256,
		MinScore:               trace.MinScore,
		Limit:                  trace.Limit,
		EmbeddingModel:         trace.EmbeddingModel,
		VectorDimension:        trace.VectorDimension,
		RefreshSource:          trace.RefreshSource,
		SelectionPolicyVersion: trace.SelectionPolicyVersion,
		CandidateMemoryIDs:     append([]common.MemoryID(nil), trace.CandidateMemoryIDs...),
		SelectedMemoryIDs:      append([]common.MemoryID(nil), trace.SelectedMemoryIDs...),
		ExcludedMemoryIDs:      append([]common.MemoryID(nil), trace.ExcludedMemoryIDs...),
		TypeQuota:              cloneStringIntMap(trace.TypeQuota),
		SelectionScores:        memoryRecallSelectionScoreCommands(trace.SelectionScores),
		RetrievedMemoryIDs:     append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...),
		RetrievedMemoryHits:    memoryRecallHitCommands(trace.RetrievedMemoryHits),
		Reason:                 trace.Reason,
		OccurredAt:             trace.OccurredAt,
		EvidenceRef:            trace.EvidenceRef,
	}
}

func cloneStringIntMap(input map[string]int) map[string]int {
	if input == nil {
		return nil
	}
	ret := make(map[string]int, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
