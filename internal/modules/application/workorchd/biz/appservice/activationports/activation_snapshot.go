package activationports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func WorkspaceExecutionActivationSnapshot(workspace *workspacemodel.Workspace, run *runmodel.Run, goal *workspacemodel.Goal, dod *workspacemodel.DoD) readmodelcore.WorkspaceExecutionActivation {
	snapshot := readmodelprojection.ActivationSnapshot(workspace, run, goal, dod)
	return readmodelcore.WorkspaceExecutionActivation{
		Workspace: snapshot.Workspace,
		Run:       runtimecore.MapRun(run),
		Snapshot:  snapshot,
	}
}
