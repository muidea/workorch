package contextregistrycore

import memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"

func MapMemoryItem(item *memorymodel.MemoryItem) MemoryItem {
	if item == nil {
		return MemoryItem{}
	}
	return MemoryItem{
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

func MapMemoryItems(items []*memorymodel.MemoryItem) []MemoryItem {
	ret := make([]MemoryItem, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapMemoryItem(item))
	}
	return ret
}
