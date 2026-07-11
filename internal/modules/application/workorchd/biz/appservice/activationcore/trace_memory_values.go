package activationcore

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TraceMemoryIDs(value any) []common.MemoryID {
	switch typed := value.(type) {
	case []common.MemoryID:
		return append([]common.MemoryID(nil), typed...)
	case []string:
		ret := make([]common.MemoryID, 0, len(typed))
		for _, item := range typed {
			if item != "" {
				ret = append(ret, common.MemoryID(item))
			}
		}
		return ret
	case []any:
		ret := make([]common.MemoryID, 0, len(typed))
		for _, item := range typed {
			if text := TraceStringValue(item); text != "" {
				ret = append(ret, common.MemoryID(text))
			}
		}
		return ret
	default:
		return nil
	}
}

func TraceMemoryHits(value any) []memorymodel.MemoryRecallHit {
	items := TraceMapSlice(value)
	ret := make([]memorymodel.MemoryRecallHit, 0, len(items))
	for _, item := range items {
		id := common.MemoryID(TraceStringValue(item["id"]))
		if id == "" {
			continue
		}
		ret = append(ret, memorymodel.MemoryRecallHit{
			ID:              id,
			Summary:         TraceStringValue(item["summary"]),
			VectorScore:     TraceFloatValue(item["vectorScore"]),
			VectorDimension: TraceIntValue(item["vectorDimension"]),
		})
	}
	return ret
}
