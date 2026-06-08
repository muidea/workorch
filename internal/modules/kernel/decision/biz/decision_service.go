package decision

import (
	"context"
	"errors"
	"reflect"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionModel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
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

func (s *Service) CreateTicket(ctx context.Context, cmd decisioncontracts.CreateTicketCommand) error {
	if cmd.TicketID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := &decisionModel.Ticket{
		ID:           cmd.TicketID,
		WorkspaceID:  cmd.WorkspaceID,
		RunID:        cmd.RunID,
		StepID:       cmd.StepID,
		Reason:       cmd.Reason,
		DecisionKind: cmd.DecisionKind,
		Question:     cmd.Question,
		Context:      clonePayload(cmd.Context),
		State:        common.DecisionTicketStateOpen,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.tickets.Create(ctx, item); err != nil {
		return err
	}
	if s.eventHub == nil {
		return nil
	}
	return s.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
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

func (s *Service) EnsureTicket(ctx context.Context, cmd decisioncontracts.CreateTicketCommand) error {
	if cmd.TicketID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	item, err := s.tickets.Get(ctx, cmd.TicketID)
	if err == nil {
		if item.WorkspaceID != cmd.WorkspaceID ||
			item.RunID != cmd.RunID ||
			item.StepID != cmd.StepID ||
			item.Reason != cmd.Reason ||
			item.DecisionKind != cmd.DecisionKind ||
			item.Question != cmd.Question ||
			!sameResolutionPayload(item.Context, cmd.Context) {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return s.CreateTicket(ctx, cmd)
}

func (s *Service) ResolveTicket(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) error {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return common.ErrInvalidArgument
	}
	if !modulecommon.IsSupportedResolutionType(cmd.Type) {
		return common.ErrInvalidArgument
	}
	ctx = resolveCommandContext(ctx, cmd)

	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return err
	}
	if !ticketMatchesResolveScope(ticket, cmd) {
		return common.ErrNotFound
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return common.ErrConflict
	}

	_, err = s.resolveTicketItem(ctx, ticket, cmd)
	return err
}

func (s *Service) resolveTicketItem(ctx context.Context, ticket *decisionModel.Ticket, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Resolution, error) {
	if ticket == nil || ticket.ID == "" {
		return nil, common.ErrInvalidArgument
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return nil, common.ErrConflict
	}

	ticket.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	resolution := &decisionModel.Resolution{
		ID:       cmd.ResolutionID,
		TicketID: cmd.TicketID,
		Type:     cmd.Type,
		Payload:  common.RedactSensitiveMap(clonePayload(cmd.Payload)),
	}
	if err := s.resolutions.Create(ctx, resolution); err != nil {
		return nil, err
	}

	ticket.State = common.DecisionTicketStateResolved
	if err := s.tickets.Update(ctx, ticket); err != nil {
		return nil, err
	}
	if s.eventHub == nil {
		return resolution, nil
	}
	if err := s.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
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
	}); err != nil {
		return nil, err
	}
	return resolution, nil
}

func (s *Service) EnsureResolvedTicket(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) error {
	_, _, err := s.EnsureResolvedTicketAndReturn(ctx, cmd)
	return err
}

func (s *Service) EnsureResolvedTicketAndReturn(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Ticket, *decisionModel.Resolution, error) {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return nil, nil, common.ErrInvalidArgument
	}
	if !modulecommon.IsSupportedResolutionType(cmd.Type) {
		return nil, nil, common.ErrInvalidArgument
	}
	ctx = resolveCommandContext(ctx, cmd)

	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return nil, nil, err
	}
	if !ticketMatchesResolveScope(ticket, cmd) {
		return nil, nil, common.ErrNotFound
	}
	if ticket.State == common.DecisionTicketStateResolved {
		resolution, err := s.resolutions.GetByTicketID(ctx, cmd.TicketID)
		if err != nil {
			return nil, nil, err
		}
		if resolution.ID != cmd.ResolutionID || resolution.Type != cmd.Type || !sameResolutionPayload(resolution.Payload, cmd.Payload) {
			return nil, nil, common.ErrConflict
		}
		return ticket, resolution, nil
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return nil, nil, common.ErrConflict
	}
	resolution, err := s.resolveTicketItem(ctx, ticket, cmd)
	if err != nil {
		return nil, nil, err
	}
	return ticket, resolution, nil
}

func ticketMatchesResolveScope(ticket *decisionModel.Ticket, cmd decisioncontracts.ResolveTicketCommand) bool {
	if ticket == nil {
		return false
	}
	if cmd.WorkspaceID != "" && ticket.WorkspaceID != cmd.WorkspaceID {
		return false
	}
	if cmd.RunID != "" && ticket.RunID != cmd.RunID {
		return false
	}
	if cmd.StepID != "" && ticket.StepID != cmd.StepID {
		return false
	}
	return true
}

func resolveCommandContext(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) context.Context {
	if cmd.WorkspaceID == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
}

func sameResolutionPayload(left map[string]any, right map[string]any) bool {
	if len(left) == 0 && len(right) == 0 {
		return true
	}
	return reflect.DeepEqual(left, right)
}

func clonePayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
