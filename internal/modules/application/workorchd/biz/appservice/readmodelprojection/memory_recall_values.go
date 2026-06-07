package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
)

func mapMemoryRecallSelectionScores(items []memorymodel.MemoryRecallSelectionScore) []readmodelcore.MemoryRecallSelectionScore {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.MemoryRecallSelectionScore, 0, len(items))
	for _, item := range items {
		ret = append(ret, readmodelcore.MemoryRecallSelectionScore{
			ID:     string(item.ID),
			Score:  item.Score,
			Reason: item.Reason,
		})
	}
	return ret
}

func mapMemoryRecallHits(items []memorymodel.MemoryRecallHit) []readmodelcore.MemoryRecallHit {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.MemoryRecallHit, 0, len(items))
	for _, item := range items {
		ret = append(ret, readmodelcore.MemoryRecallHit{
			ID:              string(item.ID),
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}
