package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func selectWorkspaceOutcomeRun(snapshot readmodelcore.WorkspaceSnapshot) *appmodelcore.Run {
	if snapshot.Workspace.ActiveRunID != "" {
		if activeRun := workspaceRunByID(snapshot, snapshot.Workspace.ActiveRunID); activeRun != nil && !IsTerminalRunState(activeRun.State) {
			return activeRun
		}
	}
	if snapshot.Run != nil && !IsTerminalRunState(snapshot.Run.State) {
		return snapshot.Run
	}
	selectedRun := latestWorkspaceRunForOutcome(snapshot)
	if selectedRun == nil && snapshot.TaskProgress.RunID != "" {
		selectedRun = workspaceRunByID(snapshot, snapshot.TaskProgress.RunID)
	}
	if selectedRun == nil && snapshot.Workspace.ActiveRunID != "" {
		selectedRun = workspaceRunByID(snapshot, snapshot.Workspace.ActiveRunID)
	}
	if selectedRun == nil && len(snapshot.Runs) > 0 {
		selectedRun = &snapshot.Runs[len(snapshot.Runs)-1]
	}
	return selectedRun
}

func workspaceRunByID(snapshot readmodelcore.WorkspaceSnapshot, runID string) *appmodelcore.Run {
	for idx := range snapshot.Runs {
		if snapshot.Runs[idx].ID == runID {
			return &snapshot.Runs[idx]
		}
	}
	return nil
}
