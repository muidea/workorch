package readmodelprojection

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapMemoryRecallSelectionScores(items []memorymodel.MemoryRecallSelectionScore) []projectioncontract.MemoryRecallSelectionScore {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.MemoryRecallSelectionScore, 0, len(items))
	for _, item := range items {
		ret = append(ret, projectioncontract.MemoryRecallSelectionScore{
			ID:     string(item.ID),
			Score:  item.Score,
			Reason: item.Reason,
		})
	}
	return ret
}

func mapMemoryRecallHits(items []memorymodel.MemoryRecallHit) []projectioncontract.MemoryRecallHit {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.MemoryRecallHit, 0, len(items))
	for _, item := range items {
		ret = append(ret, projectioncontract.MemoryRecallHit{
			ID:              string(item.ID),
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}
