package runports

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func RecoverTerminalActiveWorkspacePointers(ctx context.Context, hub events.Hub) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	items, err := workspaceports.ListWorkspaceRecords(ctx, hub)
	if err != nil {
		return err
	}
	for _, workspace := range items {
		if workspace == nil || workspace.ActiveRunID == "" {
			continue
		}
		runCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspace.ID)
		activeRun, err := ReadRun(runCtx, hub, workspace.ActiveRunID)
		if err != nil {
			if !errors.Is(err, common.ErrNotFound) {
				return err
			}
			workspace.ActiveRunID = ""
			if runtimecore.IsActiveWorkspaceStateForRecovery(workspace.State) {
				workspace.State = common.WorkspaceStateActive
			}
			if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
				return err
			}
			continue
		}
		if activeRun.WorkspaceID != workspace.ID || !runtimecore.IsTerminalRunState(activeRun.State) {
			continue
		}
		workspace.ActiveRunID = ""
		workspace.State = runtimecore.RecoveredWorkspaceStateForTerminalRun(activeRun.State)
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return err
		}
	}
	return nil
}
