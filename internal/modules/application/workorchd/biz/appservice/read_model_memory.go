package appservice

import (
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func mapMemoryRecallTraces(items []*memorymodel.MemoryRecallTrace) []MemoryRecallTrace {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, MemoryRecallTrace{
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
			SelectionScores:        mapMemoryRecallSelectionScores(item.SelectionScores),
			RetrievedMemoryIDs:     memoryIDsToReadModelStrings(item.RetrievedMemoryIDs),
			RetrievedMemoryHits:    mapMemoryRecallHits(item.RetrievedMemoryHits),
			Reason:                 item.Reason,
			OccurredAt:             item.OccurredAt,
			EvidenceRef:            item.EvidenceRef,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapMemoryRecallSelectionScores(items []memorymodel.MemoryRecallSelectionScore) []MemoryRecallSelectionScore {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MemoryRecallSelectionScore, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallSelectionScore{
			ID:     string(item.ID),
			Score:  item.Score,
			Reason: item.Reason,
		})
	}
	return ret
}

func cloneReadModelIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]int, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func mapMemoryRecallHits(items []memorymodel.MemoryRecallHit) []MemoryRecallHit {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MemoryRecallHit, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallHit{
			ID:              string(item.ID),
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}

func memoryIDsToReadModelStrings(input []common.MemoryID) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		ret = append(ret, string(item))
	}
	return ret
}
