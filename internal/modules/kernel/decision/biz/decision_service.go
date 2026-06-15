package decision

import (
	"context"
	"errors"
	"reflect"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
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
		Topic:        decisionevents.TopicTicketCreated,
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
	if err := s.publishResolvedTicketEvent(ctx, ticket, resolution); err != nil {
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

	// Orphan resolution recovery: a resolution file may already exist for this
	// ticket even when ticket.State is still "open" (e.g. an operator manually
	// reset the run back to blocked_on_decision without deleting the
	// resolution, or an external fixture wrote state=open while leaving the
	// resolution on disk). Production code never reverts a resolved ticket to
	// open, so this mismatch is recoverable. When a persisted resolution
	// matches the new request, accept it idempotently and sync the ticket
	// state to "resolved" so the run orchestrator's ResumeRun check at
	// runtime_lifecycle.go (ticket.State == Resolved) passes. Because the
	// current owner state is transitioning from open -> resolved again, we
	// must publish TopicTicketResolved so workorchd projections, SSE, and TUI
	// stay consistent with the repaired owner state.
	existingResolution, resErr := s.resolutions.GetByTicketID(ctx, cmd.TicketID)
	if resErr != nil && !errors.Is(resErr, common.ErrNotFound) {
		return nil, nil, resErr
	}
	if existingResolution != nil {
		if existingResolution.ID != cmd.ResolutionID ||
			existingResolution.Type != cmd.Type ||
			!sameResolutionPayload(existingResolution.Payload, cmd.Payload) {
			return nil, nil, common.ErrConflict
		}
		if ticket.State != common.DecisionTicketStateResolved {
			ticket.State = common.DecisionTicketStateResolved
			ticket.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err := s.tickets.Update(ctx, ticket); err != nil {
				return nil, nil, err
			}
			if err := s.publishResolvedTicketEvent(ctx, ticket, existingResolution); err != nil {
				return nil, nil, err
			}
		}
		return ticket, existingResolution, nil
	}

	if ticket.State == common.DecisionTicketStateResolved {
		// Resolution file was deleted but ticket state is still "resolved";
		// preserve the existing strict behavior. Re-creating a resolution
		// here would be a fresh write — treat as ErrConflict to surface the
		// invariant violation rather than silently accept.
		return nil, nil, common.ErrConflict
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

func (s *Service) publishResolvedTicketEvent(ctx context.Context, ticket *decisionModel.Ticket, resolution *decisionModel.Resolution) error {
	if s == nil || s.eventHub == nil || ticket == nil || resolution == nil {
		return nil
	}
	return s.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        decisionevents.TopicTicketResolved,
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
