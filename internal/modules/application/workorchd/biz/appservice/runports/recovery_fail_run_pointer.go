package runports

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func FailRunAndRecoverActiveWorkspacePointer(ctx context.Context, hub events.Hub, item *runmodel.Run, reason string) (*runmodel.Run, error) {
	if hub == nil || item == nil {
		return nil, common.ErrInvalidArgument
	}
	runCtx := common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	if err := FailRun(runCtx, hub, item, reason); err != nil {
		return nil, err
	}
	failed, err := ReadRun(runCtx, hub, item.ID)
	if err != nil {
		return nil, err
	}
	workspace, err := workspaceports.ReadWorkspaceRecord(ctx, hub, failed.WorkspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return failed, nil
		}
		return nil, err
	}
	if workspace == nil || workspace.ActiveRunID != failed.ID {
		return failed, nil
	}
	workspace.ActiveRunID = ""
	workspace.State = runtimecore.RecoveredWorkspaceStateForTerminalRun(failed.State)
	return failed, workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace)
}
