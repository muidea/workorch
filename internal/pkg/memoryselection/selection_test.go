package memoryselection

import (
	"reflect"
	"strings"
	"testing"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSharedSelectionPreservesOriginalPolicyAndRawInputs(t *testing.T) {
	for _, size := range []int{0, 1, 12, 200, 201} {
		items := make([]*memorymodel.MemoryItem, 0, size)
		for i := 0; i < size; i++ {
			items = append(items, &memorymodel.MemoryItem{ID: common.MemoryID("id-" + strings.Repeat("x", i)), MemoryType: "project_context", Content: "原文\r\n", Tags: []string{}, Vector: []float64{0.25}})
		}
		if size >= 12 {
			items[0].MemoryType, items[0].Tags = "tool_feedback", []string{"provider_retry"}
			items[1].MemoryType, items[1].Tags = "tool_feedback", []string{"provider_retry", "accepted_input"}
			items[3].Content = strings.Repeat("long", 5000)
			items[4] = nil
			items[6] = items[5]
			items[8].MemoryType = "decision"
		}
		original := (memorymodel.PreparationInventory{Items: items}).Clone().Items
		got, err := SelectRuntimeMemoryContext(items)
		want, refErr := SelectReferenceMemoryContext(items)
		if (err == nil) != (refErr == nil) || !reflect.DeepEqual(got, RuntimeMemorySelection(want)) || !reflect.DeepEqual(items, original) {
			t.Fatal("shared compiler changed original policy, raw input or rejection", size, err, refErr)
		}
		if len(got.Candidates) > 0 {
			got.Candidates[0].Content = "changed"
			if !reflect.DeepEqual(items, original) {
				t.Fatal("return aliases original")
			}
		}
	}
}

func FuzzSharedSelectionReference(f *testing.F) {
	f.Add([]byte("project_context\r\n"))
	f.Add([]byte{0, 255, 3, 9, 7})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		items := make([]*memorymodel.MemoryItem, len(data))
		for i, b := range data {
			items[i] = &memorymodel.MemoryItem{ID: common.MemoryID("id-" + strings.Repeat("a", i%205)), MemoryType: "project_context", Importance: int(b), Content: string(data), Tags: []string{}}
		}
		got, err := SelectRuntimeMemoryContext(items)
		want, refErr := SelectReferenceMemoryContext(items)
		if (err == nil) != (refErr == nil) || !reflect.DeepEqual(got, RuntimeMemorySelection(want)) {
			t.Fatal("reference drift")
		}
	})
}
