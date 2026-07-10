package runports

import (
	"context"
	"errors"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func RecoverTerminalActiveWorkspacePointers(ctx context.Context, hub magicevent.Hub) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	items, err := workspaceports.ListWorkspaceRecords(ctx, hub)
	if err != nil {
		return fmt.Errorf("list workspace records: %w", err)
	}
	for _, workspace := range items {
		if workspace == nil || workspace.ActiveRunID == "" {
			continue
		}
		runCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspace.ID)
		activeRun, err := ReadRun(runCtx, hub, workspace.ActiveRunID)
		if err != nil {
			if !errors.Is(err, common.ErrNotFound) {
				return fmt.Errorf("read active run %q for workspace %q: %w", workspace.ActiveRunID, workspace.ID, err)
			}
			workspace.ActiveRunID = ""
			if runtimecore.IsActiveWorkspaceStateForRecovery(workspace.State) {
				workspace.State = common.WorkspaceStateActive
			}
			if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
				return fmt.Errorf("clear missing active run pointer for workspace %q: %w", workspace.ID, err)
			}
			continue
		}
		if activeRun.WorkspaceID != workspace.ID || !runtimecore.IsTerminalRunState(activeRun.State) {
			continue
		}
		workspace.ActiveRunID = ""
		workspace.State = runtimecore.RecoveredWorkspaceStateForTerminalRun(activeRun.State)
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return fmt.Errorf("clear terminal active run pointer for workspace %q: %w", workspace.ID, err)
		}
	}
	return nil
}
