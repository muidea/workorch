package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func mapWorkspaceWithRunProjection(workspace *workspacemodel.Workspace, run *runmodel.Run) appmodelcore.Workspace {
	// Run is accepted only to keep the existing pure projector signature. The
	// Workspace owner model is the sole source of Workspace state/activeRun.
	// Governance correlation belongs to GovernanceDisplay and never rewrites it.
	_ = run
	return workspacecore.MapWorkspace(workspace)
}
