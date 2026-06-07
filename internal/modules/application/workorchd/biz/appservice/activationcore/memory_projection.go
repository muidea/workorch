package activationcore

import memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"

func MemoryItemIDs(items []*memorymodel.MemoryItem) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, string(item.ID))
	}
	return ret
}

func MemoryRetrievalHits(items []*memorymodel.MemoryItem) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, map[string]any{
			"id":              string(item.ID),
			"summary":         item.Summary,
			"vectorScore":     item.VectorScore,
			"vectorDimension": len(item.Vector),
		})
	}
	return ret
}
