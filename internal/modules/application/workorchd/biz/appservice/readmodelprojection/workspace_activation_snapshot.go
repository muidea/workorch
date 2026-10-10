package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func activationSnapshot(workspace *workspacemodel.Workspace, run *runmodel.Run, goal *workspacemodel.Goal, dod *workspacemodel.DoD) readmodelcore.WorkspaceSnapshotState {
	projectedWorkspace := mapWorkspaceWithRunProjection(workspace, run)
	if run == nil {
		return activationSnapshotWithoutRun(projectedWorkspace, goal, dod)
	}
	return activationSnapshotWithRun(projectedWorkspace, run, goal, dod)
}
