package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileRuntimeTargetSelectionReturnsCanonicalCompleteOwnerFacts(t *testing.T) {
	adapter := NewRuntimeAdapter()
	directory := t.TempDir()
	binding := runtimeTargetSelectionBinding{
		ownerRef: "profile", subjectRef: "implementation",
		prospectiveTargetAllOfConstraintRefs: runtimeFileProspectiveTargetAllOfConstraintRefs(),
	}
	acquire := func(selector string) runtimecontract.RuntimeTargetSelection {
		t.Helper()
		selection, err := adapter.AcquireRuntimeTargetSelection(directory, "work-scope", "implementation_work", selector, "", runtimecontract.RuntimeOwnerInventoryItemsMaxCount, binding)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(selection, selection.Normalized()) {
			t.Fatalf("owner returned a noncanonical selection preimage: %#v", selection)
		}
		if selection.SnapshotID != runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection) {
			t.Fatal("owner selection identity differs from its complete returned preimage")
		}
		bytes, err := json.Marshal(selection)
		if err != nil {
			t.Fatal(err)
		}
		var restored runtimecontract.RuntimeTargetSelection
		if err := json.Unmarshal(bytes, &restored); err != nil || !reflect.DeepEqual(restored, selection) {
			t.Fatalf("owner selection changed across current JSON reload: %v", err)
		}
		return selection
	}
	empty := acquire(runtimeFileTargetSelectorWorkspaceDeliverable)
	if empty.Disposition != runtimecontract.RuntimeTargetSelectionScope || empty.CandidateCount != 0 || empty.TargetRefs != nil || empty.CandidateTargetRefs != nil {
		t.Fatalf("empty scope must retain canonical absence without inventing a target: %#v", empty)
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
		t.Fatalf("scope observation changed the empty deliverable: entries=%v err=%v", entries, err)
	}
	for index, name := range []string{"resource-a", "resource-b"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		selection := acquire(runtimeFileTargetSelectorWorkspaceDeliverable)
		wanted := runtimecontract.RuntimeTargetSelectionSelected
		if index == 1 {
			wanted = runtimecontract.RuntimeTargetSelectionScope
		}
		if selection.Disposition != wanted || selection.CandidateCount != index+1 || len(selection.TargetRefs) != index+1 || len(selection.CandidateTargetRefs) != index+1 || selection.ObservedRevision == empty.ObservedRevision {
			t.Fatalf("owner lost its complete current inventory: %#v", selection)
		}
	}
	unsupported := acquire("unsupported-work-selector")
	if unsupported.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported || unsupported.TargetRefs != nil || unsupported.CandidateTargetRefs != nil {
		t.Fatalf("unsupported fact acquired selected targets: %#v", unsupported)
	}
}
