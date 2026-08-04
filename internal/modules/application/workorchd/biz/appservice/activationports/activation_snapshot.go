package activationports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func WorkspaceExecutionActivationSnapshot(workspace *workspacemodel.Workspace, run *runmodel.Run, goal *workspacemodel.Goal, dod *workspacemodel.DoD) readmodelcore.WorkspaceExecutionActivationState {
	snapshot := readmodelprojection.ActivationSnapshot(workspace, run, goal, dod)
	return readmodelcore.WorkspaceExecutionActivationState{
		Workspace: snapshot.Workspace,
		Run:       runtimecore.MapRun(run),
		Snapshot:  snapshot,
	}
}
