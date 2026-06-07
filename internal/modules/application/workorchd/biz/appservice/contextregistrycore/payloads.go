package contextregistrycore

import (
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func MemoryItemsFromPayload(payload map[string]any, key string) ([]*memorymodel.MemoryItem, error) {
	items, ok := payload[key].([]*memorymodel.MemoryItem)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
