package decision

import (
	"context"
	"sort"
	"strings"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeGetTicketCommand(ctx context.Context, data any) (decisionevents.GetTicketResult, error) {
	cmd, ok := data.(decisionevents.GetTicketCommand)
	if !ok || cmd.TicketID == "" {
		return decisionevents.GetTicketResult{}, common.ErrInvalidArgument
	}
	item, err := m.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return decisionevents.GetTicketResult{}, err
	}
	if item.DeletionRequestedAt != "" || !ticketMatchesDecisionScope(item, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID}) {
		return decisionevents.GetTicketResult{}, common.ErrNotFound
	}
	return decisionevents.GetTicketResult{Ticket: decisionmodel.CloneTicket(item)}, nil
}

func (m *Biz) executeListTicketsCommand(ctx context.Context, data any) (decisionevents.ListTicketsResult, error) {
	cmd, ok := data.(decisionevents.ListTicketsCommand)
	if !ok || cmd.Scope != decisionevents.TicketListScopeAll {
		return decisionevents.ListTicketsResult{}, common.ErrInvalidArgument
	}
	items, err := m.tickets.List(ctx)
	if err != nil {
		return decisionevents.ListTicketsResult{}, err
	}
	page, next, err := paginateDecisionTickets(items, cmd.Cursor, cmd.Limit)
	return decisionevents.ListTicketsResult{Tickets: page, NextCursor: next}, err
}

func (m *Biz) executeListTicketsByRunCommand(ctx context.Context, data any) (decisionevents.ListTicketsByRunResult, error) {
	cmd, ok := data.(decisionevents.ListTicketsByRunCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
		return decisionevents.ListTicketsByRunResult{}, common.ErrInvalidArgument
	}
	items, err := m.tickets.ListByRunID(ctx, cmd.RunID)
	if err != nil {
		return decisionevents.ListTicketsByRunResult{}, err
	}
	items = filterTicketsByDecisionScope(items, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID})
	page, next, err := paginateDecisionTickets(items, cmd.Cursor, cmd.Limit)
	return decisionevents.ListTicketsByRunResult{Tickets: page, NextCursor: next}, err
}

func (m *Biz) executeListTicketsByWorkspaceCommand(ctx context.Context, data any) (decisionevents.ListTicketsByWorkspaceResult, error) {
	cmd, ok := data.(decisionevents.ListTicketsByWorkspaceCommand)
	if !ok || cmd.WorkspaceID == "" {
		return decisionevents.ListTicketsByWorkspaceResult{}, common.ErrInvalidArgument
	}
	items, err := m.tickets.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return decisionevents.ListTicketsByWorkspaceResult{}, err
	}
	page, next, err := paginateDecisionTickets(items, cmd.Cursor, cmd.Limit)
	return decisionevents.ListTicketsByWorkspaceResult{Tickets: page, NextCursor: next}, err
}

func paginateDecisionTickets(items []*decisionmodel.Ticket, cursor string, limit int) ([]*decisionmodel.Ticket, string, error) {
	cursor, limit, err := decisionevents.NormalizeListWindow(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	visible := make([]*decisionmodel.Ticket, 0, len(items))
	for _, item := range items {
		if item == nil || item.DeletionRequestedAt != "" {
			continue
		}
		visible = append(visible, item)
	}
	sort.Slice(visible, func(i, j int) bool {
		return compareDecisionTicketOrder(visible[i], visible[j]) < 0
	})
	start := 0
	for start < len(visible) && compareDecisionTicketToCursor(visible[start], cursor) <= 0 {
		start++
	}
	end := start + limit
	if end > len(visible) {
		end = len(visible)
	}
	page := decisionmodel.CloneTickets(visible[start:end])
	next := ""
	if end < len(visible) && len(page) > 0 {
		next = decisionevents.TicketCursor(page[len(page)-1])
	}
	return page, next, nil
}

func compareDecisionTicketOrder(left *decisionmodel.Ticket, right *decisionmodel.Ticket) int {
	if left == nil || right == nil {
		switch {
		case left == nil && right == nil:
			return 0
		case left == nil:
			return -1
		default:
			return 1
		}
	}
	if value := strings.Compare(string(left.WorkspaceID), string(right.WorkspaceID)); value != 0 {
		return value
	}
	if value := strings.Compare(string(left.RunID), string(right.RunID)); value != 0 {
		return value
	}
	return strings.Compare(string(left.ID), string(right.ID))
}

func compareDecisionTicketToCursor(item *decisionmodel.Ticket, cursor string) int {
	if cursor == "" {
		return 1
	}
	workspaceID, remainder, ok := strings.Cut(cursor, "\x00")
	if !ok {
		return strings.Compare(decisionevents.TicketCursor(item), cursor)
	}
	runID, ticketID, ok := strings.Cut(remainder, "\x00")
	if !ok || strings.Contains(ticketID, "\x00") {
		return strings.Compare(decisionevents.TicketCursor(item), cursor)
	}
	if item == nil {
		return strings.Compare("", cursor)
	}
	if value := strings.Compare(string(item.WorkspaceID), workspaceID); value != 0 {
		return value
	}
	if value := strings.Compare(string(item.RunID), runID); value != 0 {
		return value
	}
	return strings.Compare(string(item.ID), ticketID)
}

func (m *Biz) executePurgeWorkspaceCommand(ctx context.Context, data any) (decisionevents.PurgeWorkspaceDecisionsResult, error) {
	cmd, ok := data.(decisionevents.PurgeWorkspaceDecisionsCommand)
	if !ok || cmd.WorkspaceID == "" {
		return decisionevents.PurgeWorkspaceDecisionsResult{}, common.ErrInvalidArgument
	}
	items, err := m.tickets.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return decisionevents.PurgeWorkspaceDecisionsResult{}, err
	}
	deleted := 0
	for _, item := range items {
		if item == nil || item.WorkspaceID != cmd.WorkspaceID {
			continue
		}
		if err := m.service.PurgeDecisionAggregate(ctx, item); err != nil {
			return decisionevents.PurgeWorkspaceDecisionsResult{Deleted: deleted}, err
		}
		deleted++
	}
	return decisionevents.PurgeWorkspaceDecisionsResult{Deleted: deleted}, nil
}
