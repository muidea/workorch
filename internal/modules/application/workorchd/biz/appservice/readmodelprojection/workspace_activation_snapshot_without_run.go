package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func activationSnapshotWithoutRun(projectedWorkspace appmodelcore.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD) readmodelcore.WorkspaceSnapshotState {
	ret := readmodelcore.WorkspaceSnapshotState{
		Workspace: projectedWorkspace,
	}
	applyActivationDefinitions(&ret, goal, dod)
	ret.DisplayOutcome = mapWorkspaceOutcome(ret)
	return ret
}

func applyActivationDefinitions(snapshot *readmodelcore.WorkspaceSnapshotState, goal *workspacemodel.Goal, dod *workspacemodel.DoD) {
	if goal != nil {
		addActivationDefinitions(snapshot, goal, dod)
	}
	if goal == nil && dod != nil {
		addActivationDefinitions(snapshot, goal, dod)
	}
}
