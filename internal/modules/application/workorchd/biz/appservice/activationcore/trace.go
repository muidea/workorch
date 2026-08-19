package activationcore

import (
	"fmt"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func BuildRuntimeActivationMemoryRecallTrace(runID common.RunID, workspaceID common.WorkspaceID, bootstrap *runtimecontract.RuntimeBootstrap, occurredAt string) *memorymodel.MemoryRecallTrace {
	if runID == "" || workspaceID == "" || bootstrap == nil {
		return nil
	}
	activation := bootstrap.Activation.Normalized()
	if activation.MemorySelection == nil {
		return nil
	}
	selection := activation.MemorySelection.Normalized()
	traceID := common.MemoryRecallTraceID(fmt.Sprintf("%s/memory-recall/activation", runID))
	return &memorymodel.MemoryRecallTrace{
		ID: traceID, WorkspaceID: workspaceID, RunID: runID, Stage: "planning", Source: "activation",
		Policy: selection.Policy, Status: selection.Status, ProviderID: selection.ProviderID, ModelID: selection.ModelID,
		Operation: selection.Operation, QueryTextPreview: selection.QueryTextPreview, QueryTextSha256: selection.QueryTextSHA256,
		MinScore: selection.MinScore, Limit: selection.Limit, EmbeddingModel: selection.EmbeddingModel, VectorDimension: selection.VectorDimension,
		RefreshSource: selection.RefreshSource, SelectionPolicyVersion: selection.SelectionPolicyVersion,
		CandidateMemoryIDs: append([]common.MemoryID(nil), selection.CandidateMemoryIDs...),
		SelectedMemoryIDs:  append([]common.MemoryID(nil), selection.SelectedMemoryIDs...), ExcludedMemoryIDs: append([]common.MemoryID(nil), selection.ExcludedMemoryIDs...),
		TypeQuota: recallTypeQuotaMap(selection.TypeQuota), SelectionScores: append([]workorchmemorycontract.RecallSelectionScore(nil), selection.SelectionScores...),
		RetrievedMemoryIDs: append([]common.MemoryID(nil), selection.RetrievedMemoryIDs...), RetrievedMemoryHits: append([]workorchmemorycontract.RecallHit(nil), selection.RetrievedMemoryHits...),
		ContextBudget: selection.ContextBudget, SemanticRecall: cloneRecallSelection(selection.SemanticRecall), FallbackFrom: selection.FallbackFrom,
		Reason:     selection.Reason,
		OccurredAt: occurredAt, EvidenceRef: string(traceID),
	}
}

func recallTypeQuotaMap(values []workorchmemorycontract.TypeQuotaEntry) map[string]int {
	if values == nil {
		return nil
	}
	ret := make(map[string]int, len(values))
	for _, value := range values {
		ret[value.Type] = value.Quota
	}
	return ret
}

func cloneRecallSelection(input *workorchmemorycontract.RecallSelectionEvidence) *workorchmemorycontract.RecallSelectionEvidence {
	if input == nil {
		return nil
	}
	value := input.Normalized()
	return &value
}
