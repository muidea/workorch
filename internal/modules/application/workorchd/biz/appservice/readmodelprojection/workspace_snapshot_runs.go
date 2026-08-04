package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func mapWorkspaceSnapshotRuns(workspace *workspacemodel.Workspace, runs []*runmodel.Run) []appmodelcore.Run {
	ret := make([]appmodelcore.Run, 0, len(runs))
	for _, item := range runs {
		if item == nil {
			continue
		}
		if workspace != nil && item.WorkspaceID != workspace.ID {
			continue
		}
		ret = append(ret, runtimecore.MapRun(item))
	}
	return ret
}
