package approval

import (
	"context"
	"sort"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeGetApprovalCommand(ctx context.Context, data any) (approvalevents.GetApprovalResult, error) {
	cmd, ok := data.(approvalevents.GetApprovalCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.ApprovalID == "" {
		return approvalevents.GetApprovalResult{}, common.ErrInvalidArgument
	}
	item, err := m.approvals.GetScoped(ctx, cmd.WorkspaceID, cmd.RunID, cmd.ApprovalID)
	if err != nil {
		return approvalevents.GetApprovalResult{}, err
	}
	if item.DeletionRequestedAt != "" {
		return approvalevents.GetApprovalResult{}, common.ErrNotFound
	}
	return approvalevents.GetApprovalResult{Approval: approvalmodel.CloneApproval(item)}, nil
}

func (m *Biz) executeListApprovalsCommand(ctx context.Context, data any) (approvalevents.ListApprovalsResult, error) {
	cmd, ok := data.(approvalevents.ListAllApprovalsCommand)
	if !ok || cmd.Scope != approvalevents.ApprovalListScopeAll {
		return approvalevents.ListApprovalsResult{}, common.ErrInvalidArgument
	}
	cursor, limit, err := approvalevents.NormalizeListWindow(cmd.Cursor, cmd.Limit)
	if err != nil {
		return approvalevents.ListApprovalsResult{}, err
	}
	items, err := m.approvals.List(ctx)
	if err != nil {
		return approvalevents.ListApprovalsResult{}, err
	}
	items, next, err := pageApprovals(items, cursor, limit)
	return approvalevents.ListApprovalsResult{Approvals: items, NextCursor: next}, err
}

func (m *Biz) executeListApprovalsByRunCommand(ctx context.Context, data any) (approvalevents.ListApprovalsByRunResult, error) {
	cmd, ok := data.(approvalevents.ListApprovalsByRunCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
		return approvalevents.ListApprovalsByRunResult{}, common.ErrInvalidArgument
	}
	cursor, limit, err := approvalevents.NormalizeListWindow(cmd.Cursor, cmd.Limit)
	if err != nil {
		return approvalevents.ListApprovalsByRunResult{}, err
	}
	items, err := m.approvals.ListByRunScope(ctx, cmd.WorkspaceID, cmd.RunID)
	if err != nil {
		return approvalevents.ListApprovalsByRunResult{}, err
	}
	items, next, err := pageApprovals(items, cursor, limit)
	return approvalevents.ListApprovalsByRunResult{Approvals: items, NextCursor: next}, err
}

func (m *Biz) executeListApprovalsByWorkspaceCommand(ctx context.Context, data any) (approvalevents.ListApprovalsByWorkspaceResult, error) {
	cmd, ok := data.(approvalevents.ListApprovalsByWorkspaceCommand)
	if !ok || cmd.WorkspaceID == "" {
		return approvalevents.ListApprovalsByWorkspaceResult{}, common.ErrInvalidArgument
	}
	cursor, limit, err := approvalevents.NormalizeListWindow(cmd.Cursor, cmd.Limit)
	if err != nil {
		return approvalevents.ListApprovalsByWorkspaceResult{}, err
	}
	items, err := m.approvals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return approvalevents.ListApprovalsByWorkspaceResult{}, err
	}
	items, next, err := pageApprovals(items, cursor, limit)
	return approvalevents.ListApprovalsByWorkspaceResult{Approvals: items, NextCursor: next}, err
}

func pageApprovals(items []*approvalmodel.Approval, cursor string, limit int) ([]*approvalmodel.Approval, string, error) {
	filtered := make([]*approvalmodel.Approval, 0, len(items))
	for _, item := range items {
		if item != nil && item.DeletionRequestedAt == "" {
			filtered = append(filtered, approvalmodel.CloneApproval(item))
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return approvalevents.ApprovalCursor(filtered[i]) < approvalevents.ApprovalCursor(filtered[j])
	})
	start := 0
	if cursor != "" {
		found := false
		for idx, item := range filtered {
			if approvalevents.ApprovalCursor(item) == cursor {
				start, found = idx+1, true
				break
			}
		}
		if !found {
			return nil, "", common.ErrInvalidArgument
		}
	}
	if start >= len(filtered) {
		return nil, "", nil
	}
	end := start + limit
	if end >= len(filtered) {
		return filtered[start:], "", nil
	}
	return filtered[start:end], approvalevents.ApprovalCursor(filtered[end-1]), nil
}
