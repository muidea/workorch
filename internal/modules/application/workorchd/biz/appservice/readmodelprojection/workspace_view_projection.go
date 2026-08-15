package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func mapWorkspaceWithRunProjection(workspace *workspacemodel.Workspace, run *runmodel.Run) appmodelcore.Workspace {
	ret := workspacecore.MapWorkspace(workspace)
	if run == nil {
		return ret
	}
	if run.State == common.RunStatePending {
		return ret
	}
	ret.State = string(workspaceProjectionState(run.State))
	if isTerminalProjectionRunState(run.State) {
		ret.ActiveRunID = ""
		ret.BlockingTicketID = ""
		ret.BlockingApprovalID = ""
	} else {
		ret.ActiveRunID = string(run.ID)
		ret.BlockingTicketID = string(run.BlockingTicketID)
		ret.BlockingApprovalID = string(run.BlockingApprovalID)
	}
	return ret
}
