package memory

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPreparationMemoryInventoryRawCloneAndClosedIdentity(t *testing.T) {
	v := PreparationInventory{Version: PreparationInventoryVersion, WorkspaceID: "ws", OwnerEpoch: "memory-preparation-epoch:" + strings.Repeat("A", 26), ObservedAt: time.Now().UTC(), Items: []*MemoryItem{{ID: "memory", WorkspaceID: "ws", SourceType: "user", MemoryType: "project_context", Summary: "原摘要\r\n", Content: "原文\x00", Tags: []string{}, Vector: []float64{0.125}, Importance: 12345, Confidence: -1}}}
	if v.ValidateFor("ws") != nil {
		t.Fatal("raw score/content changed or invented selection policy")
	}
	c := v.Clone()
	if !reflect.DeepEqual(c, v) || c.Items[0].Tags == nil {
		t.Fatal("clone normalized nil/empty or original values")
	}
	c.Items[0].Vector[0] = 99
	c.Items[0].Content = "caller"
	if v.Items[0].Vector[0] != 0.125 || v.Items[0].Content != "原文\x00" {
		t.Fatal("clone aliases raw material")
	}
	for _, change := range []func(*PreparationInventory){
		func(v *PreparationInventory) { v.Version = "other" },
		func(v *PreparationInventory) { v.WorkspaceID = "other" },
		func(v *PreparationInventory) { v.OwnerEpoch = "memory-preparation-epoch:" },
		func(v *PreparationInventory) { v.ObservedAt = time.Time{} },
		func(v *PreparationInventory) { v.Items = nil },
		func(v *PreparationInventory) { v.Items[0].Vector[0] = math.Inf(1) },
		func(v *PreparationInventory) { v.Items[0].Content = string([]byte{255}) },
		func(v *PreparationInventory) { v.Items = append(v.Items, v.Items[0]) },
	} {
		bad := v.Clone()
		change(&bad)
		if bad.ValidateFor("ws") == nil {
			t.Fatal("malformed inventory accepted")
		}
	}
	v.Items = []*MemoryItem{}
	if v.ValidateFor("ws") != nil {
		t.Fatal("complete actual empty inventory rejected")
	}
}
