package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceRuntimeActivationInputs struct {
	Workspace *workspacemodel.Workspace
	Goal      *workspacemodel.Goal
	DoD       *workspacemodel.DoD
}

func ReadWorkspaceRuntimeActivationInputs(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID) (WorkspaceRuntimeActivationInputs, error) {
	if hub == nil || workspaceID == "" || goalID == "" || dodID == "" {
		return WorkspaceRuntimeActivationInputs{}, common.ErrInvalidArgument
	}
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	var ret WorkspaceRuntimeActivationInputs
	if err := readmodelcore.RunTasks(
		func() error {
			var err error
			ret.Goal, err = definitionports.ReadGoal(readCtx, hub, goalID)
			return err
		},
		func() error {
			var err error
			ret.DoD, err = definitionports.ReadDoD(readCtx, hub, dodID)
			return err
		},
		func() error {
			var err error
			ret.Workspace, err = workspaceports.ReadWorkspaceRecord(ctx, hub, workspaceID)
			return err
		},
	); err != nil {
		return WorkspaceRuntimeActivationInputs{}, err
	}
	return ret, nil
}
