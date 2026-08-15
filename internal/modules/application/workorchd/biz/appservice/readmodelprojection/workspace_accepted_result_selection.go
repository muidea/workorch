package readmodelprojection

import (
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func selectAcceptedResultRun(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, results []*resultmodel.Result) *runmodel.Run {
	var selected *resultmodel.Result
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID || item.State != common.ResultStateAccepted || item.RunID == "" {
			continue
		}
		run := runs[item.RunID]
		if run == nil || run.State != common.RunStateCompleted {
			continue
		}
		if selected == nil || resultProjectionTime(item) >= resultProjectionTime(selected) {
			selected = item
		}
	}
	if selected == nil {
		return nil
	}
	return runs[selected.RunID]
}
