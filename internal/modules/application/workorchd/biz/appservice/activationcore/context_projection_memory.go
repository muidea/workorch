package activationcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
)

func MemoryContexts(items []*memorymodel.MemoryItem) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, map[string]any{
			"id":          string(item.ID),
			"memoryType":  item.MemoryType,
			"summary":     item.Summary,
			"content":     item.Content,
			"tags":        contextregistrycore.CloneStrings(item.Tags),
			"importance":  item.Importance,
			"confidence":  item.Confidence,
			"visibility":  item.Visibility,
			"provenance":  MemoryProvenance(item),
			"vectorScore": item.VectorScore,
		})
	}
	return ret
}

func MemoryProvenance(item *memorymodel.MemoryItem) string {
	if item == nil || item.RunID == "" {
		return "workspace"
	}
	return "historical_run"
}
