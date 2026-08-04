package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func workspaceOutcomeRunFromResultFallback(results []readmodelcore.Result, runsByID map[string]*appmodelcore.Run) *appmodelcore.Run {
	if result := latestTerminalWorkspaceResult(results, runsByID); result != nil {
		if run := runsByID[result.RunID]; run != nil {
			return run
		}
	}
	if result := primaryAcceptedResult(results, runsByID); result != nil {
		if run := runsByID[result.RunID]; run != nil {
			return run
		}
	}
	return nil
}
