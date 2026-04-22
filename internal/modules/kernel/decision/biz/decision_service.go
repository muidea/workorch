package decision

import (
	"context"
	"errors"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionModel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	eventHub    events.Hub
}

func NewService(tickets repository.DecisionTicketRepository, resolutions repository.DecisionResolutionRepository, eventHub events.Hub) *Service {
	return &Service{tickets: tickets, resolutions: resolutions, eventHub: eventHub}
}

func (s *Service) CreateTicket(ctx context.Context, cmd decisionservice.CreateTicketCommand) error {
	if cmd.TicketID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	item := &decisionModel.Ticket{
		ID:          cmd.TicketID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      cmd.Reason,
		State:       common.DecisionTicketStateOpen,
	}
	if err := s.tickets.Create(ctx, item); err != nil {
		return err
	}
	if s.eventHub == nil {
		return nil
	}
	return s.eventHub.Publish(ctx, events.Envelope{
		Version:      events.Phase1EventVersion,
		Topic:        events.TopicTicketCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(item.StepID),
		Payload:      events.DecisionTicketPayload(item.Reason, item.State),
	})
}

func (s *Service) EnsureTicket(ctx context.Context, cmd decisionservice.CreateTicketCommand) error {
	if cmd.TicketID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	item, err := s.tickets.Get(ctx, cmd.TicketID)
	if err == nil {
		if item.WorkspaceID != cmd.WorkspaceID || item.RunID != cmd.RunID || item.StepID != cmd.StepID {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return s.CreateTicket(ctx, cmd)
}

func (s *Service) ResolveTicket(ctx context.Context, cmd decisionservice.ResolveTicketCommand) error {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return common.ErrInvalidArgument
	}

	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return err
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return common.ErrConflict
	}

	resolution := &decisionModel.Resolution{
		ID:       cmd.ResolutionID,
		TicketID: cmd.TicketID,
		Type:     cmd.Type,
		Payload:  cmd.Payload,
	}
	if err := s.resolutions.Create(ctx, resolution); err != nil {
		return err
	}

	ticket.State = common.DecisionTicketStateResolved
	if err := s.tickets.Update(ctx, ticket); err != nil {
		return err
	}
	if s.eventHub == nil {
		return nil
	}
	return s.eventHub.Publish(ctx, events.Envelope{
		Version:      events.Phase1EventVersion,
		Topic:        events.TopicTicketResolved,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(ticket.ID),
		WorkspaceID:  string(ticket.WorkspaceID),
		RunID:        string(ticket.RunID),
		StepID:       string(ticket.StepID),
		Payload: events.DecisionResolutionPayload(
			ticket.Reason,
			ticket.State,
			resolution.ID,
			resolution.Type,
			resolution.Payload,
		),
	})
}

func (s *Service) EnsureResolvedTicket(ctx context.Context, cmd decisionservice.ResolveTicketCommand) error {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return common.ErrInvalidArgument
	}

	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return err
	}
	if ticket.State == common.DecisionTicketStateResolved {
		return nil
	}
	return s.ResolveTicket(ctx, cmd)
}
