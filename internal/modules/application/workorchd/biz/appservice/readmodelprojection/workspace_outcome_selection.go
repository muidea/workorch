package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: workspace_outcome_event_selection.go

func latestWorkspaceRunForOutcome(snapshot readmodelcore.WorkspaceSnapshotState) *appmodelcore.Run {
	if len(snapshot.Runs) == 0 {
		return nil
	}
	runsByID := workspaceOutcomeRunsByID(snapshot.Runs)
	selectedEvent := latestWorkspaceOutcomeRunEvent(snapshot, runsByID)
	if selectedEvent != nil {
		if result := latestTerminalWorkspaceResult(snapshot.Results, runsByID); result != nil && resultNotBeforeEvent(*result, *selectedEvent) {
			if run := runsByID[result.RunID]; run != nil {
				return run
			}
		}
		return runsByID[selectedEvent.RunID]
	}
	if run := workspaceOutcomeRunFromResultFallback(snapshot.Results, runsByID); run != nil {
		return run
	}
	return &snapshot.Runs[len(snapshot.Runs)-1]
}

// source: workspace_outcome_selection.go

func selectWorkspaceOutcomeRun(snapshot readmodelcore.WorkspaceSnapshotState) *appmodelcore.Run {
	if snapshot.Workspace.ActiveRunID != "" {
		if activeRun := workspaceRunByID(snapshot, snapshot.Workspace.ActiveRunID); activeRun != nil && !isTerminalRunState(activeRun.State) {
			return activeRun
		}
	}
	if snapshot.Run != nil && !isTerminalRunState(snapshot.Run.State) {
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

func workspaceRunByID(snapshot readmodelcore.WorkspaceSnapshotState, runID string) *appmodelcore.Run {
	for idx := range snapshot.Runs {
		if snapshot.Runs[idx].ID == runID {
			return &snapshot.Runs[idx]
		}
	}
	return nil
}
