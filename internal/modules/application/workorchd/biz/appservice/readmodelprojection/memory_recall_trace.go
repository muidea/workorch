package readmodelprojection

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapMemoryRecallTrace(item *memorymodel.MemoryRecallTrace) projectioncontract.MemoryRecallTrace {
	return projectioncontract.MemoryRecallTrace{
		ID:                     string(item.ID),
		WorkspaceID:            string(item.WorkspaceID),
		RunID:                  string(item.RunID),
		StepID:                 string(item.StepID),
		GraphNodeID:            item.GraphNodeID,
		Stage:                  item.Stage,
		Source:                 item.Source,
		Policy:                 item.Policy,
		Status:                 item.Status,
		ProviderID:             item.ProviderID,
		ModelID:                item.ModelID,
		Operation:              item.Operation,
		QueryTextPreview:       item.QueryTextPreview,
		QueryTextSha256:        item.QueryTextSha256,
		MinScore:               item.MinScore,
		Limit:                  item.Limit,
		EmbeddingModel:         item.EmbeddingModel,
		VectorDimension:        item.VectorDimension,
		RefreshSource:          item.RefreshSource,
		SelectionPolicyVersion: item.SelectionPolicyVersion,
		CandidateMemoryIDs:     memoryIDsToReadModelStrings(item.CandidateMemoryIDs),
		SelectedMemoryIDs:      memoryIDsToReadModelStrings(item.SelectedMemoryIDs),
		ExcludedMemoryIDs:      memoryIDsToReadModelStrings(item.ExcludedMemoryIDs),
		TypeQuota:              cloneReadModelIntMap(item.TypeQuota),
		SelectionScores:        append([]workorchmemorycontract.RecallSelectionScore(nil), item.SelectionScores...),
		RetrievedMemoryIDs:     memoryIDsToReadModelStrings(item.RetrievedMemoryIDs),
		RetrievedMemoryHits:    append([]workorchmemorycontract.RecallHit(nil), item.RetrievedMemoryHits...),
		ContextBudget:          item.ContextBudget,
		SemanticRecall:         workorchmemorycontract.CloneRecallSelectionEvidence(item.SemanticRecall),
		FallbackFrom:           item.FallbackFrom,
		Reason:                 item.Reason,
		OccurredAt:             item.OccurredAt,
		EvidenceRef:            item.EvidenceRef,
	}
}
