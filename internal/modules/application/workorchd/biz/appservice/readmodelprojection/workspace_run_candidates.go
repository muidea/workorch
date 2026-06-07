package readmodelprojection

import (
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceProjectionRunCandidatesResult struct {
	byID   map[common.RunID]*runmodel.Run
	active *runmodel.Run
	latest *runmodel.Run
}

func workspaceProjectionRunCandidates(workspace *workspacemodel.Workspace, runs []*runmodel.Run) workspaceProjectionRunCandidatesResult {
	ret := workspaceProjectionRunCandidatesResult{
		byID: make(map[common.RunID]*runmodel.Run, len(runs)),
	}
	for _, item := range runs {
		if item == nil || item.WorkspaceID != workspace.ID {
			continue
		}
		ret.byID[item.ID] = item
		if workspace.ActiveRunID != "" && item.ID == workspace.ActiveRunID {
			ret.active = item
		}
		if ret.latest == nil || string(item.ID) > string(ret.latest.ID) {
			ret.latest = item
		}
	}
	return ret
}
