package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListApprovalsByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*approvalmodel.Approval, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	return collectApprovalPages(func(cursor string) ([]*approvalmodel.Approval, string, error) {
		event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandList, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListAllApprovalsCommand{Scope: approvalevents.ApprovalListScopeAll, Cursor: cursor, Limit: approvalevents.MaxListLimit})
		ownerevent.BindLane(event, approvalevents.Contracts(), approvalevents.TopicApprovalCommandList, "approval", "all", "", "", "")
		response, err := ownerevent.Approval[approvalevents.ListApprovalsResult](hub.Send(event), approvalevents.TopicApprovalCommandList)
		return response.Approvals, response.NextCursor, err
	})
}

func ListApprovalsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*approvalmodel.Approval, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return collectApprovalPages(func(cursor string) ([]*approvalmodel.Approval, string, error) {
		event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandListByRun, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListApprovalsByRunCommand{WorkspaceID: workspaceID, RunID: runID, Cursor: cursor, Limit: approvalevents.MaxListLimit})
		ownerevent.BindLane(event, approvalevents.Contracts(), approvalevents.TopicApprovalCommandListByRun, "run", string(runID), workspaceID, runID, "")
		response, err := ownerevent.Approval[approvalevents.ListApprovalsByRunResult](hub.Send(event), approvalevents.TopicApprovalCommandListByRun)
		return response.Approvals, response.NextCursor, err
	})
}

func ListApprovalsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return collectApprovalPages(func(cursor string) ([]*approvalmodel.Approval, string, error) {
		event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandListByWorkspace, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListApprovalsByWorkspaceCommand{WorkspaceID: workspaceID, Cursor: cursor, Limit: approvalevents.MaxListLimit})
		ownerevent.BindLane(event, approvalevents.Contracts(), approvalevents.TopicApprovalCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
		response, err := ownerevent.Approval[approvalevents.ListApprovalsByWorkspaceResult](hub.Send(event), approvalevents.TopicApprovalCommandListByWorkspace)
		return response.Approvals, response.NextCursor, err
	})
}

func collectApprovalPages(fetch func(string) ([]*approvalmodel.Approval, string, error)) ([]*approvalmodel.Approval, error) {
	const maxPages = 1024
	var ret []*approvalmodel.Approval
	cursor := ""
	seen := map[string]struct{}{}
	for page := 0; page < maxPages; page++ {
		items, next, err := fetch(cursor)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
		if next == "" {
			return ret, nil
		}
		if next == cursor {
			return nil, common.ErrConflict
		}
		if _, duplicate := seen[next]; duplicate {
			return nil, common.ErrConflict
		}
		seen[next] = struct{}{}
		cursor = next
	}
	return nil, common.ErrConflict
}
