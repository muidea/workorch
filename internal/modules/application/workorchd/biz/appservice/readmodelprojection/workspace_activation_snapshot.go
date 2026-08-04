package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func ActivationSnapshot(workspace *workspacemodel.Workspace, run *runmodel.Run, goal *workspacemodel.Goal, dod *workspacemodel.DoD) readmodelcore.WorkspaceSnapshotState {
	projectedWorkspace := MapWorkspaceWithRunProjection(workspace, run)
	if run == nil {
		return activationSnapshotWithoutRun(projectedWorkspace, goal, dod)
	}
	return activationSnapshotWithRun(projectedWorkspace, run, goal, dod)
}
