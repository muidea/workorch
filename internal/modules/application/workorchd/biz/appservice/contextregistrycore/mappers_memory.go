package contextregistrycore

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func MapMemoryItem(item *memorymodel.MemoryItem) workorchmanagementcontract.MemoryItem {
	if item == nil {
		return workorchmanagementcontract.MemoryItem{}
	}
	return workorchmanagementcontract.MemoryItem{
		ID:                  string(item.ID),
		WorkspaceID:         string(item.WorkspaceID),
		RunID:               string(item.RunID),
		SourceType:          item.SourceType,
		SourceRef:           item.SourceRef,
		MemoryType:          item.MemoryType,
		Content:             item.Content,
		Summary:             item.Summary,
		Tags:                CloneStrings(item.Tags),
		Importance:          item.Importance,
		Confidence:          item.Confidence,
		Visibility:          item.Visibility,
		EmbeddingProviderID: item.EmbeddingProviderID,
		EmbeddingModel:      item.EmbeddingModel,
		EmbeddingText:       item.EmbeddingText,
		VectorReady:         len(item.Vector) > 0,
		VectorDimension:     len(item.Vector),
		VectorScore:         item.VectorScore,
	}
}

func MapMemoryItems(items []*memorymodel.MemoryItem) []workorchmanagementcontract.MemoryItem {
	ret := make([]workorchmanagementcontract.MemoryItem, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapMemoryItem(item))
	}
	return ret
}
