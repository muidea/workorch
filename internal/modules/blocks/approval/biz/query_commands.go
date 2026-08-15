package approval

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeApprovalQueryCommand(ctx context.Context, ev magicevent.Event) (approvalevents.ApprovalCommandResult, error) {
	switch ev.ID() {
	case approvalevents.TopicApprovalCommandGet:
		cmd, ok := ev.Data().(approvalevents.GetApprovalCommand)
		if !ok || cmd.ApprovalID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.approvals.Get(ctx, cmd.ApprovalID)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		if !approvalMatchesCommandScope(item, approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}) {
			return approvalevents.ApprovalCommandResult{}, common.ErrNotFound
		}
		return approvalevents.ApprovalCommandResult{Approval: item}, nil
	case approvalevents.TopicApprovalCommandList:
		if _, ok := ev.Data().(approvalevents.ListApprovalsCommand); !ok {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.approvals.List(ctx)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		return approvalevents.ApprovalCommandResult{Approvals: items}, nil
	case approvalevents.TopicApprovalCommandListByRun:
		cmd, ok := ev.Data().(approvalevents.ListApprovalsByRunCommand)
		if !ok || cmd.RunID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.approvals.ListByRunID(ctx, cmd.RunID)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		items = filterApprovalsByCommandScope(items, approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID})
		return approvalevents.ApprovalCommandResult{Approvals: items}, nil
	case approvalevents.TopicApprovalCommandListByWorkspace:
		cmd, ok := ev.Data().(approvalevents.ListApprovalsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.approvals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		return approvalevents.ApprovalCommandResult{Approvals: items}, nil
	default:
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
}
