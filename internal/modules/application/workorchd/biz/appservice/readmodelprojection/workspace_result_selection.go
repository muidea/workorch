package readmodelprojection

import (
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func SelectTerminalResultRun(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, results []*resultmodel.Result) (*resultmodel.Result, *runmodel.Run) {
	var selected *resultmodel.Result
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID || item.RunID == "" {
			continue
		}
		switch item.State {
		case common.ResultStateAccepted, common.ResultStateRejected:
		default:
			continue
		}
		run := runs[item.RunID]
		if run == nil || !IsTerminalProjectionRunState(run.State) {
			continue
		}
		if selected == nil || ResultProjectionTime(item) >= ResultProjectionTime(selected) {
			selected = item
		}
	}
	if selected == nil {
		return nil, nil
	}
	return selected, runs[selected.RunID]
}
