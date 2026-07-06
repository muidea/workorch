package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func selectWorkspaceOutcomeRun(snapshot readmodelcore.WorkspaceSnapshot) *readmodelcore.Run {
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

func workspaceRunByID(snapshot readmodelcore.WorkspaceSnapshot, runID string) *readmodelcore.Run {
	for idx := range snapshot.Runs {
		if snapshot.Runs[idx].ID == runID {
			return &snapshot.Runs[idx]
		}
	}
	return nil
}
