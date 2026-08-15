package decision

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeTicketQueryCommand(ctx context.Context, ev magicevent.Event) (decisionevents.DecisionCommandResult, error) {
	switch ev.ID() {
	case decisionevents.TopicDecisionCommandGetTicket:
		cmd, ok := ev.Data().(decisionevents.GetTicketCommand)
		if !ok || cmd.TicketID == "" {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.tickets.Get(ctx, cmd.TicketID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if !ticketMatchesDecisionScope(item, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID}) {
			return decisionevents.DecisionCommandResult{}, common.ErrNotFound
		}
		return decisionevents.DecisionCommandResult{Ticket: item}, nil
	case decisionevents.TopicDecisionCommandListTickets:
		if _, ok := ev.Data().(decisionevents.ListTicketsCommand); !ok {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.tickets.List(ctx)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{Tickets: items}, nil
	case decisionevents.TopicDecisionCommandListTicketsByRun:
		cmd, ok := ev.Data().(decisionevents.ListTicketsByRunCommand)
		if !ok || cmd.RunID == "" {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.tickets.ListByRunID(ctx, cmd.RunID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		items = filterTicketsByDecisionScope(items, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID})
		return decisionevents.DecisionCommandResult{Tickets: items}, nil
	case decisionevents.TopicDecisionCommandListTicketsByWS:
		cmd, ok := ev.Data().(decisionevents.ListTicketsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.tickets.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{Tickets: items}, nil
	default:
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Biz) executeTicketMutationCommand(ctx context.Context, ev magicevent.Event) (decisionevents.DecisionCommandResult, error) {
	if ev.ID() != decisionevents.TopicDecisionCommandDeleteTicket {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(decisionevents.DeleteTicketCommand)
	if !ok || cmd.TicketID == "" {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	scope := decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID}
	if !scope.hasScope() {
		ticket, err := m.resolveUniqueTicketForUnscopedCommand(ctx, cmd.TicketID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, ticket.WorkspaceID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
	} else if err := m.ensureTicketScopeForCommand(ctx, cmd.TicketID, scope); err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	if err := m.tickets.Delete(ctx, cmd.TicketID); err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	return decisionevents.DecisionCommandResult{}, nil
}
