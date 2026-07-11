package runports

import (
	"context"
	"errors"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const StartupInactiveRunRecoveryReason = "startup inactive running run recovered: previous process has no active execution; workspace reset to idle manual activation"

func RecoverStartupInactiveRunningWorkspaceViews(ctx context.Context, hub magicevent.Hub) ([]appmodelcore.Run, error) {
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

func recoverStartupInactiveWorkspace(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace) (*runmodel.Run, bool, error) {
	if workspace == nil || workspace.ActiveRunID == "" {
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
	if recovered, changed, err := reconcileStartupGovernanceBlockedRun(ctx, hub, workspace, run); err != nil {
		return nil, false, err
	} else if changed {
		return recovered, true, nil
	}
	switch run.State {
	case common.RunStateBlockedOnDecision:
		workspace.State = common.WorkspaceStateActive
		workspace.ActiveRunID = run.ID
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return nil, false, fmt.Errorf("sync decision-blocked workspace state: %w", err)
		}
		return nil, true, nil
	case common.RunStateBlockedOnApproval:
		workspace.State = common.WorkspaceStateBlockedOnApproval
		workspace.ActiveRunID = run.ID
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return nil, false, fmt.Errorf("sync approval-blocked workspace state: %w", err)
		}
		return nil, true, nil
	case common.RunStatePending:
		workspace.ActiveRunID = ""
		workspace.State = common.WorkspaceStateActive
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return nil, false, fmt.Errorf("clear created run pointer: %w", err)
		}
		return nil, true, nil
	case common.RunStateActive:
	default:
		return nil, false, nil
	}
	switch workspace.State {
	case common.WorkspaceStateActive:
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

func reconcileStartupGovernanceBlockedRun(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace, run *runmodel.Run) (*runmodel.Run, bool, error) {
	if workspace == nil || run == nil || run.ID == "" || run.WorkspaceID != workspace.ID {
		return nil, false, nil
	}
	if runtimecore.IsTerminalRunState(run.State) {
		return nil, false, nil
	}
	if run.State == common.RunStateBlockedOnDecision || run.State == common.RunStateBlockedOnApproval {
		return nil, false, nil
	}
	if ticketID, ok, err := uniqueOpenDecisionTicketForRun(ctx, hub, workspace.ID, run.ID); err != nil {
		return nil, false, fmt.Errorf("list open decision tickets for active run %q: %w", run.ID, err)
	} else if ok {
		run.State = common.RunStateBlockedOnDecision
		run.BlockingTicketID = ticketID
		run.BlockingApprovalID = ""
		if err := UpdateRun(common.ContextWithRuntimeWorkspaceID(ctx, workspace.ID), hub, run); err != nil {
			return nil, false, fmt.Errorf("sync decision-blocked active run %q: %w", run.ID, err)
		}
		workspace.State = common.WorkspaceStateActive
		workspace.ActiveRunID = run.ID
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return nil, false, fmt.Errorf("sync decision-blocked active workspace %q: %w", workspace.ID, err)
		}
		return nil, true, nil
	}
	if approvalID, ok, err := uniqueOpenApprovalForRun(ctx, hub, workspace.ID, run.ID); err != nil {
		return nil, false, fmt.Errorf("list open approvals for active run %q: %w", run.ID, err)
	} else if ok {
		run.State = common.RunStateBlockedOnApproval
		run.BlockingTicketID = ""
		run.BlockingApprovalID = approvalID
		if err := UpdateRun(common.ContextWithRuntimeWorkspaceID(ctx, workspace.ID), hub, run); err != nil {
			return nil, false, fmt.Errorf("sync approval-blocked active run %q: %w", run.ID, err)
		}
		workspace.State = common.WorkspaceStateBlockedOnApproval
		workspace.ActiveRunID = run.ID
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, workspace); err != nil {
			return nil, false, fmt.Errorf("sync approval-blocked active workspace %q: %w", workspace.ID, err)
		}
		return nil, true, nil
	}
	return nil, false, nil
}

func uniqueOpenDecisionTicketForRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) (common.TicketID, bool, error) {
	items, err := governanceports.DecisionTicketsForQuery(ctx, hub, string(workspaceID), string(runID))
	if err != nil {
		return "", false, err
	}
	var ret common.TicketID
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || item.RunID != runID || item.State != common.DecisionTicketStateOpen {
			continue
		}
		if ret != "" && ret != item.ID {
			return "", false, nil
		}
		ret = item.ID
	}
	return ret, ret != "", nil
}

func uniqueOpenApprovalForRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) (common.ApprovalID, bool, error) {
	items, err := governanceports.ApprovalsForQuery(ctx, hub, string(workspaceID), string(runID))
	if err != nil {
		return "", false, err
	}
	var ret common.ApprovalID
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || item.RunID != runID || item.State != common.ApprovalStatePending {
			continue
		}
		if ret != "" && ret != item.ID {
			return "", false, nil
		}
		ret = item.ID
	}
	return ret, ret != "", nil
}
