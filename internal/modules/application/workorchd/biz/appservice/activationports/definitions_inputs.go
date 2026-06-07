package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type WorkspaceExecutionContextInputs struct {
	Workspace *workspacemodel.Workspace
	Goal      *workspacemodel.Goal
	DoD       *workspacemodel.DoD
}

func ReadWorkspaceExecutionContextInputs(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID) (WorkspaceExecutionContextInputs, error) {
	if hub == nil || workspaceID == "" || goalID == "" || dodID == "" {
		return WorkspaceExecutionContextInputs{}, common.ErrInvalidArgument
	}
	var ret WorkspaceExecutionContextInputs
	if err := readmodelcore.RunTasks(
		func() error {
			var err error
			ret.Goal, err = definitionports.ReadGoal(ctx, hub, goalID)
			return err
		},
		func() error {
			var err error
			ret.DoD, err = definitionports.ReadDoD(ctx, hub, dodID)
			return err
		},
		func() error {
			var err error
			ret.Workspace, err = workspaceports.ReadWorkspaceRecord(ctx, hub, workspaceID)
			return err
		},
	); err != nil {
		return WorkspaceExecutionContextInputs{}, err
	}
	return ret, nil
}
