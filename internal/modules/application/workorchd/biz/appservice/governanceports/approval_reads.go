package governanceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadApproval(ctx context.Context, hub magicevent.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return ownerreadports.ReadApprovalByOwnerCommand(ctx, hub, approvalID)
}

func ListApprovals(ctx context.Context, hub magicevent.Hub) ([]*approvalmodel.Approval, error) {
	return ownerreadports.ListApprovalsByOwnerCommand(ctx, hub)
}

func ListApprovalsByWorkspace(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	return ownerreadports.ListApprovalsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
}

func ListApprovalsByRun(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*approvalmodel.Approval, error) {
	return ownerreadports.ListApprovalsByRunOwnerCommand(ctx, hub, runID)
}

func ApprovalsForQuery(ctx context.Context, hub magicevent.Hub, workspaceID string, runID string) ([]*approvalmodel.Approval, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	if workspaceID != "" {
		ctx = contextWithGovernanceScope(ctx, common.WorkspaceID(workspaceID))
	}
	var (
		items []*approvalmodel.Approval
		err   error
	)
	switch {
	case runID != "":
		items, err = ListApprovalsByRun(ctx, hub, common.RunID(runID))
	case workspaceID != "":
		items, err = ListApprovalsByWorkspace(ctx, hub, common.WorkspaceID(workspaceID))
	default:
		items, err = ListApprovals(ctx, hub)
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]*approvalmodel.Approval, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if workspaceID != "" && string(item.WorkspaceID) != workspaceID {
			continue
		}
		if runID != "" && string(item.RunID) != runID {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}
