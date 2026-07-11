package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func ActivationSnapshot(workspace *workspacemodel.Workspace, run *runmodel.Run, goal *workspacemodel.Goal, dod *workspacemodel.DoD) readmodelcore.WorkspaceSnapshot {
	projectedWorkspace := MapWorkspaceWithRunProjection(workspace, run)
	if run == nil {
		return activationSnapshotWithoutRun(projectedWorkspace, goal, dod)
	}
	return activationSnapshotWithRun(projectedWorkspace, run, goal, dod)
}
