package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func activationSnapshotWithRun(projectedWorkspace appmodelcore.Workspace, run *runmodel.Run, goal *workspacemodel.Goal, dod *workspacemodel.DoD) readmodelcore.WorkspaceSnapshot {
	runView := runtimecore.MapRun(run)
	runSnapshot := readmodelcore.RunSnapshot{
		Workspace: projectedWorkspace,
		Run:       runView,
	}
	runSnapshot.TaskProgress = MapLocalizedTaskProgress(runSnapshot)
	applyRuntimeV2Progress(&runSnapshot.TaskProgress, run)
	runSnapshot.Outcome = MapRunOutcome(runSnapshot)

	ret := readmodelcore.WorkspaceSnapshot{
		Workspace:    projectedWorkspace,
		Run:          &runView,
		Outcome:      MapWorkspaceOutcome(readmodelcore.WorkspaceSnapshot{Workspace: projectedWorkspace, Runs: []appmodelcore.Run{runView}, TaskProgress: runSnapshot.TaskProgress}),
		TaskProgress: runSnapshot.TaskProgress,
		Runs:         []appmodelcore.Run{runView},
	}
	applyActivationDefinitions(&ret, goal, dod)
	return ret
}
