package backend

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAssertTypedSnapshotInfoAcceptsTypedValue(t *testing.T) {
	input := WorkspaceSnapshotInfo{
		Workspace: WorkspaceInfo{ID: "ws-a", State: "running"},
		ViewModel: WorkspaceViewModelInfo{
			WorkspaceID: "ws-a",
			PromptMode:  "running",
			Revision:    7,
		},
		Runs: []RunInfo{{
			ID:          "run-a",
			WorkspaceID: "ws-a",
			State:       "running",
		}},
		GapBaseline: &GapBaselineInfo{
			AcceptedEvidenceRefs: []string{"evidence-accepted"},
			RejectedEvidenceRefs: []string{"evidence-rejected"},
		},
	}

	snapshot, err := assertTypedSnapshotInfo(input)
	if err != nil {
		t.Fatalf("assertTypedSnapshotInfo: %v", err)
	}
	if snapshot == nil || snapshot.Workspace.ID != "ws-a" || snapshot.ViewModel.Revision != 7 {
		t.Fatalf("snapshot = %+v, want typed DTO fields preserved", snapshot)
	}
	if snapshot.GapBaseline.AcceptedEvidenceRefs[0] != "evidence-accepted" {
		t.Fatalf("gapBaseline = %+v, want evidence refs preserved", snapshot.GapBaseline)
	}
}

func TestAssertTypedSnapshotInfoAcceptsTypedPointer(t *testing.T) {
	input := &WorkspaceSnapshotInfo{Workspace: WorkspaceInfo{ID: "ws-pointer"}}

	snapshot, err := assertTypedSnapshotInfo(input)
	if err != nil {
		t.Fatalf("assertTypedSnapshotInfo: %v", err)
	}
	if snapshot != input {
		t.Fatalf("snapshot pointer = %p, want original %p", snapshot, input)
	}
}

func TestAssertTypedSnapshotInfoRejectsUntypedMap(t *testing.T) {
	_, err := assertTypedSnapshotInfo(map[string]any{
		"workspace": map[string]any{"id": "ws-map"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument for untyped map", err)
	}
}
