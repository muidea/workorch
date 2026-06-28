package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func latestWorkspaceRunForOutcome(snapshot readmodelcore.WorkspaceSnapshot) *readmodelcore.Run {
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
