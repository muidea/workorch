package events

import (
	"testing"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
)

func TestGetMemoryResultCloneDetachesOwnerState(t *testing.T) {
	item := &memorymodel.MemoryItem{Tags: []string{"owner"}, Vector: []float64{1}}
	cloned := (GetMemoryResult{Item: item}).Clone()
	item.Tags[0], item.Vector[0] = "mutated", 2
	if cloned.Item.Tags[0] != "owner" || cloned.Item.Vector[0] != 1 {
		t.Fatal("cloned memory item shares mutable fields")
	}
}
