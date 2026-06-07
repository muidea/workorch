package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func TestWorkspaceSnapshotTaskProgressUsesExistingProgress(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-progress"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-progress",
			State:       "custom",
		},
	}

	got := WorkspaceSnapshotTaskProgress(snapshot)
	if got.State != "custom" || got.WorkspaceID != "ws-progress" {
		t.Fatalf("progress = %+v, want existing workspace progress", got)
	}
}

func TestWorkspaceContextBundleFromSnapshotsPrefersRunSnapshot(t *testing.T) {
	t.Parallel()

	workspaceSnapshot := readmodelcore.WorkspaceSnapshot{
		Workspace:    readmodelcore.Workspace{ID: "ws-context"},
		TaskProgress: readmodelcore.TaskProgress{WorkspaceID: "ws-context", RunID: "run-context"},
	}
	runSnapshot := &readmodelcore.RunSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-context"},
		Run:       readmodelcore.Run{ID: "run-context", WorkspaceID: "ws-context"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-context",
			RunID:       "run-context",
			State:       "running",
		},
	}

	got := WorkspaceContextBundleFromSnapshots(workspaceSnapshot, runSnapshot)
	if got.Mode != "run_recovery" || got.RunID != "run-context" {
		t.Fatalf("context bundle = %+v, want run recovery bundle", got)
	}
}

func TestWorkspaceTaskDecompositionsFromSnapshots(t *testing.T) {
	t.Parallel()

	existing := []readmodelcore.TaskDecomposition{{ID: "workspace-decomposition"}}
	if got := WorkspaceTaskDecompositionsFromSnapshots(readmodelcore.WorkspaceSnapshot{
		TaskDecompositions: existing,
		TaskProgress:       readmodelcore.TaskProgress{RunID: "run-ignored"},
	}, &readmodelcore.RunSnapshot{TaskDecompositions: []readmodelcore.TaskDecomposition{{ID: "run-decomposition"}}}); len(got) != 1 || got[0].ID != "workspace-decomposition" {
		t.Fatalf("existing decompositions = %+v, want workspace decomposition", got)
	}

	runSnapshot := &readmodelcore.RunSnapshot{
		TaskDecompositions: []readmodelcore.TaskDecomposition{{ID: "run-decomposition"}},
	}
	if got := WorkspaceTaskDecompositionsFromSnapshots(readmodelcore.WorkspaceSnapshot{
		TaskProgress: readmodelcore.TaskProgress{RunID: "run-decomposition"},
	}, runSnapshot); len(got) != 1 || got[0].ID != "run-decomposition" {
		t.Fatalf("run decompositions = %+v, want run decomposition", got)
	}

	if got := WorkspaceTaskDecompositionsFromSnapshots(readmodelcore.WorkspaceSnapshot{}, nil); got != nil {
		t.Fatalf("decompositions = %+v, want nil without run progress", got)
	}
}
