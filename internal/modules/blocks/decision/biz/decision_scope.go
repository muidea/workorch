package decision

import (
	"context"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type decisionCommandScope struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

func (s decisionCommandScope) hasScope() bool {
	return s.WorkspaceID != "" || s.RunID != "" || s.StepID != ""
}

func ticketMatchesDecisionScope(item *decisionmodel.Ticket, scope decisionCommandScope) bool {
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

func filterTicketsByDecisionScope(items []*decisionmodel.Ticket, scope decisionCommandScope) []*decisionmodel.Ticket {
	if len(items) == 0 || !scope.hasScope() {
		return items
	}
	ret := make([]*decisionmodel.Ticket, 0, len(items))
	for _, item := range items {
		if ticketMatchesDecisionScope(item, scope) {
			ret = append(ret, item)
		}
	}
	return ret
}

func (m *Biz) resolveResolutionTicketContextForCommand(ctx context.Context, ticketID common.TicketID, scope decisionCommandScope) (context.Context, error) {
	if ticketID == "" {
		return ctx, common.ErrInvalidArgument
	}
	if m == nil || m.tickets == nil {
		return ctx, common.ErrInvalidArgument
	}
	if !scope.hasScope() {
		ticket, err := m.resolveUniqueTicketForUnscopedCommand(ctx, ticketID)
		if err != nil {
			return ctx, err
		}
		if ticket.WorkspaceID != "" {
			var bindErr error
			ctx, bindErr = common.BindRuntimeWorkspaceContext(ctx, ticket.WorkspaceID)
			if bindErr != nil {
				return ctx, bindErr
			}
		}
		return ctx, nil
	}
	item, err := m.tickets.Get(ctx, ticketID)
	if err != nil {
		return ctx, err
	}
	if !ticketMatchesDecisionScope(item, scope) {
		return ctx, common.ErrNotFound
	}
	if item.WorkspaceID != "" {
		var bindErr error
		ctx, bindErr = common.BindRuntimeWorkspaceContext(ctx, item.WorkspaceID)
		if bindErr != nil {
			return ctx, bindErr
		}
	}
	return ctx, nil
}

func (m *Biz) resolveUniqueTicketForUnscopedCommand(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	if ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	if m == nil || m.tickets == nil {
		return nil, common.ErrInvalidArgument
	}
	items, err := m.tickets.List(ctx)
	if err != nil {
		return nil, err
	}
	var matches []*decisionmodel.Ticket
	for _, item := range items {
		if item == nil || item.ID != ticketID {
			continue
		}
		matches = append(matches, item)
	}
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return nil, common.ErrConflict
	default:
		return nil, common.ErrNotFound
	}
}
