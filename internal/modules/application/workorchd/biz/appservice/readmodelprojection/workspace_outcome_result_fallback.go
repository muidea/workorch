package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func workspaceOutcomeRunFromResultFallback(results []projectioncontract.Result, runsByID map[string]*appmodelcore.Run) *appmodelcore.Run {
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
