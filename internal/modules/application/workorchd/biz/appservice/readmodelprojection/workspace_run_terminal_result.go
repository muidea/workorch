package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func selectTerminalWorkspaceResultRun(workspace *workspacemodel.Workspace, candidates map[common.RunID]*runmodel.Run, results []*resultmodel.Result, latestEvent *eventstreammodel.EventRecord) *runmodel.Run {
	if workspace == nil {
		return nil
	}
	result, resultRun := selectTerminalResultRun(workspace.ID, candidates, results)
	if resultRun != nil && resultModelNotBeforeEvent(result, latestEvent) {
		return resultRun
	}
	return nil
}
