package cli

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestPatchesForRunFiltersWorkspaceSnapshotPatches(t *testing.T) {
	runID := common.RunID("run-1")
	got := patchesForRun(&backend.WorkspaceSnapshotInfo{
		Patches: []backend.PatchInfo{
			{ID: "patch-1", RunID: runID, TargetRef: "main.go"},
			{ID: "patch-2", RunID: "run-2", TargetRef: "other.go"},
		},
	}, runID)
	if len(got) != 1 || got[0].ID != "patch-1" {
		t.Fatalf("patches = %+v, want only run-1 patch", got)
	}
}

func TestNonEmptyApprovalIDPrefersApplyThenRevertApproval(t *testing.T) {
	if got := nonEmptyApprovalID("apply-approval", "revert-approval"); got != "apply-approval" {
		t.Fatalf("approval id = %q, want apply approval", got)
	}
	if got := nonEmptyApprovalID("", "revert-approval"); got != "revert-approval" {
		t.Fatalf("approval id = %q, want revert approval", got)
	}
}
