package activationcore

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/memoryselection"
)

func VisibleMemoryItems(items []*memorymodel.MemoryItem, limit int) []*memorymodel.MemoryItem {
	return memoryselection.VisibleMemoryItems(items, limit)
}
func ExcludedMemoryItems(candidates, selected []*memorymodel.MemoryItem) []*memorymodel.MemoryItem {
	return memoryselection.ExcludedMemoryItems(candidates, selected)
}
func MemoryVisibleToLLM(item *memorymodel.MemoryItem) bool {
	return memoryselection.MemoryVisibleToLLM(item)
}
func MemoryHasTag(item *memorymodel.MemoryItem, tag string) bool {
	return memoryselection.MemoryHasTag(item, tag)
}
