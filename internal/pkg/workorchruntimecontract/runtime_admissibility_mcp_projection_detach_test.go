package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeAdmissibilityMCPProjectionDetachesOriginalNestedMetadata(t *testing.T) {
	input := RuntimeOperationAdmissibility{MCPCapabilities: []RuntimeMCPCapabilityContext{{Name: "raw capability", Tags: []string{" tag-b ", "tag-a"}, StageHints: []string{" raw-stage-b ", "stage-a"}}}}
	before, _ := json.Marshal(input)
	got := input.Normalized()
	// The former projection sealed only effect fields. The fix must retain
	// that complete value, including original metadata order and raw spaces.
	want := SealMCPCapabilityEffect(input.MCPCapabilities[0])
	if len(got.MCPCapabilities) != 1 || !reflect.DeepEqual(got.MCPCapabilities[0], want) {
		t.Fatal("detachment changed original capability projection")
	}
	got.MCPCapabilities[0].Tags[0] = "changed-copy"
	got.MCPCapabilities[0].StageHints[0] = "changed-copy"
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("normalization shares mutable original metadata")
	}
	var reload RuntimeOperationAdmissibility
	if json.Unmarshal(before, &reload) != nil || !reflect.DeepEqual(input.Normalized(), reload.Normalized()) {
		t.Fatal("reload changed projection")
	}
	input.MCPCapabilities[0].StageHints[0] = "later-original"
	if input.Normalized().MCPCapabilities[0].StageHints[0] != "later-original" {
		t.Fatal("subsequent invocation reused old metadata")
	}
}
