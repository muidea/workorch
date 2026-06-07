package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func selectTerminalWorkspaceResultRun(workspace *workspacemodel.Workspace, candidates map[common.RunID]*runmodel.Run, results []*resultmodel.Result, latestEvent *eventstreammodel.EventRecord) *runmodel.Run {
	if workspace == nil || !IsTerminalProjectionState(string(workspace.State)) {
		return nil
	}
	result, resultRun := SelectTerminalResultRun(workspace.ID, candidates, results)
	if resultRun != nil && ResultModelNotBeforeEvent(result, latestEvent) {
		return resultRun
	}
	return nil
}
