package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func mapWorkspaceSnapshotRuns(workspace *workspacemodel.Workspace, runs []*runmodel.Run) []readmodelcore.Run {
	ret := make([]readmodelcore.Run, 0, len(runs))
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
