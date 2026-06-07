package ownercommandports

import (
	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
)

func memoryRecallSelectionScoreCommands(items []memorymodel.MemoryRecallSelectionScore) []memorycontracts.MemoryRecallSelectionScoreCommand {
	if items == nil {
		return nil
	}
	ret := make([]memorycontracts.MemoryRecallSelectionScoreCommand, 0, len(items))
	for _, item := range items {
		ret = append(ret, memorycontracts.MemoryRecallSelectionScoreCommand{
			ID:     item.ID,
			Score:  item.Score,
			Reason: item.Reason,
		})
	}
	return ret
}

func memoryRecallHitCommands(items []memorymodel.MemoryRecallHit) []memorycontracts.MemoryRecallHitCommand {
	if items == nil {
		return nil
	}
	ret := make([]memorycontracts.MemoryRecallHitCommand, 0, len(items))
	for _, item := range items {
		ret = append(ret, memorycontracts.MemoryRecallHitCommand{
			ID:              item.ID,
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}
