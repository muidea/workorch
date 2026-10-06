package readmodelprojection

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestProjectWorkspaceRuntimeViewRunStateRequiresActualWorkspaceRun(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		snapshot readmodelcore.WorkspaceSnapshotState
		want     string
	}{
		{name: "active_without_run", snapshot: readmodelcore.WorkspaceSnapshotState{
			Workspace:    appmodelcore.Workspace{ID: "ws", State: "active"},
			TaskProgress: projectioncontract.TaskProgress{State: "running"},
		}},
		{name: "terminal_progress_without_run", snapshot: readmodelcore.WorkspaceSnapshotState{
			Workspace:    appmodelcore.Workspace{ID: "ws", State: "completed"},
			TaskProgress: projectioncontract.TaskProgress{RunID: "run-old", State: "completed"},
		}},
		{name: "actual_owner_beats_progress", snapshot: readmodelcore.WorkspaceSnapshotState{
			Workspace:    appmodelcore.Workspace{ID: "ws", State: "active", ActiveRunID: "run"},
			TaskProgress: projectioncontract.TaskProgress{RunID: "run", State: "running"},
			Runs:         []appmodelcore.Run{{ID: "run", WorkspaceID: "ws", State: "completed"}},
		}, want: "completed"},
		{name: "cross_workspace_list", snapshot: readmodelcore.WorkspaceSnapshotState{
			Workspace:    appmodelcore.Workspace{ID: "ws", ActiveRunID: "run"},
			TaskProgress: projectioncontract.TaskProgress{RunID: "run", State: "running"},
			Runs:         []appmodelcore.Run{{ID: "run", WorkspaceID: "other", State: "failed"}},
		}},
		{name: "cross_workspace_pointer", snapshot: readmodelcore.WorkspaceSnapshotState{
			Workspace: appmodelcore.Workspace{ID: "ws", ActiveRunID: "run"},
			Run:       &appmodelcore.Run{ID: "run", WorkspaceID: "other", State: "failed"},
		}},
		{name: "actual_run_empty_state", snapshot: readmodelcore.WorkspaceSnapshotState{
			Workspace:    appmodelcore.Workspace{ID: "ws", ActiveRunID: "run"},
			TaskProgress: projectioncontract.TaskProgress{RunID: "run", State: "running"},
			Run:          &appmodelcore.Run{ID: "run", WorkspaceID: "ws"},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := projectWorkspaceRuntimeView(test.snapshot, 10, "")
			if view.RunState != test.want {
				t.Fatalf("RunState=%q want=%q", view.RunState, test.want)
			}
			encoded, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			if test.want == "" && strings.Contains(string(encoded), `"runState"`) {
				t.Fatalf("无 owner 状态仍暴露 runState: %s", encoded)
			}
		})
	}
}
