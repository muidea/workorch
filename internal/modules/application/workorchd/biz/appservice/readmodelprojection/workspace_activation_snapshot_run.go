package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func activationSnapshotWithRun(projectedWorkspace appmodelcore.Workspace, run *runmodel.Run, goal *workspacemodel.Goal, dod *workspacemodel.DoD) readmodelcore.WorkspaceSnapshotState {
	runView := runtimecore.MapRun(run)
	runSnapshot := readmodelcore.RunSnapshotState{
		Workspace: projectedWorkspace,
		Run:       runView,
	}
	runSnapshot.TaskProgress = MapLocalizedTaskProgress(runSnapshot)
	applyRuntimeProgress(&runSnapshot.TaskProgress, run)
	runSnapshot.Outcome = MapRunOutcome(runSnapshot)

	ret := readmodelcore.WorkspaceSnapshotState{
		Workspace:    projectedWorkspace,
		Run:          &runView,
		Outcome:      MapWorkspaceOutcome(readmodelcore.WorkspaceSnapshotState{Workspace: projectedWorkspace, Runs: []appmodelcore.Run{runView}, TaskProgress: runSnapshot.TaskProgress}),
		TaskProgress: runSnapshot.TaskProgress,
		Runs:         []appmodelcore.Run{runView},
	}
	applyActivationDefinitions(&ret, goal, dod)
	return ret
}
