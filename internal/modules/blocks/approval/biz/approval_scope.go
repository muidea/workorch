package approval

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type approvalCommandScope struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

func (s approvalCommandScope) hasScope() bool {
	return s.WorkspaceID != "" || s.RunID != "" || s.StepID != ""
}

func approvalMatchesCommandScope(item *approvalmodel.Approval, scope approvalCommandScope) bool {
	if item == nil {
		return false
	}
	if scope.WorkspaceID != "" && item.WorkspaceID != scope.WorkspaceID {
		return false
	}
	if scope.RunID != "" && item.RunID != scope.RunID {
		return false
	}
	if scope.StepID != "" && item.StepID != scope.StepID {
		return false
	}
	return true
}

func filterApprovalsByCommandScope(items []*approvalmodel.Approval, scope approvalCommandScope) []*approvalmodel.Approval {
	if len(items) == 0 || !scope.hasScope() {
		return items
	}
	ret := make([]*approvalmodel.Approval, 0, len(items))
	for _, item := range items {
		if approvalMatchesCommandScope(item, scope) {
			ret = append(ret, item)
		}
	}
	return ret
}

func (m *Biz) ensureApprovalScopeForCommand(ctx context.Context, approvalID common.ApprovalID, scope approvalCommandScope) (*approvalmodel.Approval, error) {
	if approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	if m == nil || m.approvals == nil {
		return nil, common.ErrInvalidArgument
	}
	if !scope.hasScope() {
		return m.resolveUniqueApprovalForUnscopedCommand(ctx, approvalID)
	}
	item, err := m.approvals.Get(ctx, approvalID)
	if err != nil {
		return nil, err
	}
	if !approvalMatchesCommandScope(item, scope) {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (m *Biz) resolveUniqueApprovalForUnscopedCommand(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	if approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	if m == nil || m.approvals == nil {
		return nil, common.ErrInvalidArgument
	}
	items, err := m.approvals.List(ctx)
	if err != nil {
		return nil, err
	}
	var matches []*approvalmodel.Approval
	var pendingMatches []*approvalmodel.Approval
	for _, item := range items {
		if item == nil || item.ID != approvalID {
			continue
		}
		matches = append(matches, item)
		if item.State == common.ApprovalStatePending {
			pendingMatches = append(pendingMatches, item)
		}
	}
	switch {
	case len(pendingMatches) == 1:
		return pendingMatches[0], nil
	case len(pendingMatches) > 1:
		return nil, common.ErrConflict
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return nil, common.ErrConflict
	default:
		return nil, common.ErrNotFound
	}
}
