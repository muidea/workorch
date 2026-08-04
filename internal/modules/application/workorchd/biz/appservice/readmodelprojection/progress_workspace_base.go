package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func newWorkspaceTaskProgressBase(snapshot readmodelcore.WorkspaceSnapshot, labels WorkspaceTaskProgressLabels) readmodelcore.TaskProgress {
	ret := readmodelcore.TaskProgress{
		WorkspaceID: snapshot.Workspace.ID,
		State:       snapshot.Workspace.State,
		Recovery: projectioncontract.RecoveryPlan{
			Mode:       "idle",
			NextAction: "start_workspace_execution",
			Reason:     labels.NoActiveRun,
		},
		RepairLoop: readmodelcore.RepairLoop{
			State: "inactive",
		},
	}
	if snapshot.Workspace.ActiveRunID != "" {
		ret.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.Mode = "resume"
		ret.Recovery.NextAction = "inspect_active_run"
		ret.Recovery.Reason = labels.ActiveRun
	}
	return ret
}
