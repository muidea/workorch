package runports

import (
	"context"
	"errors"
	"fmt"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const StartupInactiveRunRecoveryReason = "startup inactive running run recovered: previous process has no active execution; workspace reset to active"

func RecoverStartupInactiveRunningWorkspaceViews(ctx context.Context, hub events.Hub) ([]appmodelcore.Run, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := RecoverTerminalActiveWorkspacePointers(ctx, hub); err != nil {
		return nil, fmt.Errorf("recover terminal active workspace pointers: %w", err)
	}
	workspaces, err := workspaceports.ListWorkspaceRecords(ctx, hub)
	if err != nil {
		return nil, fmt.Errorf("list workspace records: %w", err)
	}
	ret := make([]appmodelcore.Run, 0)
	for _, workspace := range workspaces {
		recovered, changed, err := recoverStartupInactiveWorkspace(ctx, hub, workspace)
		if err != nil {
			return nil, fmt.Errorf("recover startup inactive workspace %q: %w", workspaceIDForRecoveryError(workspace), err)
		}
		if changed && recovered != nil {
			ret = append(ret, RunViewWithFailureReason(recovered, StartupInactiveRunRecoveryReason))
		}
	}
	return ret, nil
}

func workspaceIDForRecoveryError(workspace *workspacemodel.Workspace) string {
	if workspace == nil {
		return ""
	}
	return string(workspace.ID)
}

func recoverStartupInactiveWorkspace(ctx context.Context, hub events.Hub, workspace *workspacemodel.Workspace) (*runmodel.Run, bool, error) {
	if workspace == nil || workspace.ActiveRunID == "" {
		return nil, false, nil
	}
	switch workspace.State {
	case common.WorkspaceStateRunning, common.WorkspaceStateValidating:
	default:
		return nil, false, nil
	}
	runCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspace.ID)
	run, err := ReadRun(runCtx, hub, workspace.ActiveRunID)
	if err != nil {
		if !errors.Is(err, common.ErrNotFound) {
			return nil, false, fmt.Errorf("read active run %q: %w", workspace.ActiveRunID, err)
		}
		workspace.ActiveRunID = ""
		workspace.State = common.WorkspaceStateActive
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return nil, false, fmt.Errorf("clear missing active run pointer: %w", err)
		}
		return nil, true, nil
	}
	if run.WorkspaceID != workspace.ID {
		return nil, false, nil
	}
	if runtimecore.IsTerminalRunState(run.State) {
		return nil, false, nil
	}
	switch run.State {
	case common.RunStateBlockedOnDecision:
		workspace.State = common.WorkspaceStateWaitingDecision
		workspace.ActiveRunID = run.ID
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return nil, false, fmt.Errorf("sync decision-blocked workspace state: %w", err)
		}
		return nil, true, nil
	case common.RunStateBlockedOnApproval:
		workspace.State = common.WorkspaceStateBlocked
		workspace.ActiveRunID = run.ID
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return nil, false, fmt.Errorf("sync approval-blocked workspace state: %w", err)
		}
		return nil, true, nil
	case common.RunStateCreated:
		workspace.ActiveRunID = ""
		workspace.State = common.WorkspaceStateActive
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return nil, false, fmt.Errorf("clear created run pointer: %w", err)
		}
		return nil, true, nil
	case common.RunStateRunning, common.RunStateValidating:
	default:
		return nil, false, nil
	}
	if err := CancelRun(runCtx, hub, run); err != nil {
		return nil, false, fmt.Errorf("cancel inactive run %q: %w", run.ID, err)
	}
	cancelled, err := ReadRun(runCtx, hub, run.ID)
	if err != nil {
		return nil, false, fmt.Errorf("read cancelled inactive run %q: %w", run.ID, err)
	}
	workspace.ActiveRunID = ""
	workspace.State = common.WorkspaceStateActive
	if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
		return nil, false, fmt.Errorf("clear inactive run pointer: %w", err)
	}
	return cancelled, true, nil
}
