package decision

import (
	"context"

	decisionbiz "github.com/muidea/workorch/internal/modules/kernel/decision/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = decisionservice.Service
type CreateTicketCommand = decisionservice.CreateTicketCommand
type ResolveTicketCommand = decisionservice.ResolveTicketCommand

type Dependencies struct {
	Tickets     repository.DecisionTicketRepository
	Resolutions repository.DecisionResolutionRepository
	EventHub    events.Hub
}

type Module struct {
	bizPtr      *decisionbiz.Service
	servicePtr  *decisionservice.Module
	eventHub    events.Hub
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	unsub       []func()
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Tickets == nil || deps.Resolutions == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.tickets = deps.Tickets
	m.resolutions = deps.Resolutions
	m.bizPtr = decisionbiz.NewService(deps.Tickets, deps.Resolutions, deps.EventHub)
	m.servicePtr = decisionservice.New(decisionservice.Dependencies{
		CreateTicket:                  m.bizPtr.CreateTicket,
		EnsureTicket:                  m.bizPtr.EnsureTicket,
		ResolveTicket:                 m.bizPtr.ResolveTicket,
		EnsureResolvedTicket:          m.bizPtr.EnsureResolvedTicket,
		EnsureResolvedTicketAndReturn: m.bizPtr.EnsureResolvedTicketAndReturn,
	})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if len(m.unsub) > 0 {
		return nil
	}
	m.unsub = append(m.unsub, m.eventHub.Subscribe(events.TopicDecisionCommandEnsureResolved, m.handleEnsureResolvedCommand))
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(events.TopicDecisionCommandEnsureTicket, m.handleTicketCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandCreateTicket, m.handleTicketCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandGetTicket, m.handleTicketCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandListTickets, m.handleTicketCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandListTicketsByRun, m.handleTicketCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandListTicketsByWS, m.handleTicketCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandUpdateTicket, m.handleTicketCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandDeleteTicket, m.handleTicketCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandCreateResolution, m.handleResolutionCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandGetResolution, m.handleResolutionCommand),
		m.eventHub.Subscribe(events.TopicDecisionCommandDeleteResolution, m.handleResolutionCommand),
	)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	for idx := len(m.unsub) - 1; idx >= 0; idx-- {
		if m.unsub[idx] != nil {
			m.unsub[idx]()
		}
	}
	m.unsub = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.eventHub = nil
	m.tickets = nil
	m.resolutions = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

func (m *Module) handleEnsureResolvedCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil || m.tickets == nil {
		return common.ErrInvalidArgument
	}
	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID(stringPayload(envelope.Payload, events.PayloadKeyResolutionID)),
		TicketID:     common.TicketID(stringPayload(envelope.Payload, events.PayloadKeyBlockingTicketID)),
		WorkspaceID:  scopedWorkspaceID(envelope),
		RunID:        scopedRunID(envelope),
		StepID:       scopedStepID(envelope),
		Type:         stringPayload(envelope.Payload, events.PayloadKeyType),
	}
	if payload, _ := envelope.Payload[events.PayloadKeyPayload].(map[string]any); payload != nil {
		cmd.Payload = payload
	}
	ticket, resolution, err := m.servicePtr.EnsureResolvedTicketAndReturn(ctx, cmd)
	if err != nil {
		return err
	}
	return decisionCommandResponse(envelope, map[string]any{
		events.PayloadKeyTicket:     ticket,
		events.PayloadKeyResolution: resolution,
	})
}

func (m *Module) handleTicketCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.tickets == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case events.TopicDecisionCommandEnsureTicket:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(decisionservice.CreateTicketCommand)
		if !ok {
			item, err := ticketFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
			cmd = decisionservice.CreateTicketCommand{
				TicketID:     item.ID,
				WorkspaceID:  item.WorkspaceID,
				RunID:        item.RunID,
				StepID:       item.StepID,
				Reason:       item.Reason,
				DecisionKind: item.DecisionKind,
				Question:     item.Question,
				Context:      item.Context,
			}
		}
		if err := m.servicePtr.EnsureTicket(ctx, cmd); err != nil {
			return err
		}
		return decisionCommandResponse(envelope, nil)
	case events.TopicDecisionCommandCreateTicket:
		item, err := ticketFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		if err := m.servicePtr.CreateTicket(ctx, decisionservice.CreateTicketCommand{
			TicketID:     item.ID,
			WorkspaceID:  item.WorkspaceID,
			RunID:        item.RunID,
			StepID:       item.StepID,
			Reason:       item.Reason,
			DecisionKind: item.DecisionKind,
			Question:     item.Question,
			Context:      item.Context,
		}); err != nil {
			return err
		}
		return decisionCommandResponse(envelope, nil)
	case events.TopicDecisionCommandGetTicket:
		item, err := m.tickets.Get(ctx, common.TicketID(stringPayload(envelope.Payload, events.PayloadKeyBlockingTicketID)))
		if err != nil {
			return err
		}
		if !ticketMatchesScope(item, envelope) {
			return common.ErrNotFound
		}
		return decisionCommandResponse(envelope, map[string]any{events.PayloadKeyTicket: item})
	case events.TopicDecisionCommandListTickets:
		items, err := m.tickets.List(ctx)
		if err != nil {
			return err
		}
		return decisionCommandResponse(envelope, map[string]any{events.PayloadKeyTickets: items})
	case events.TopicDecisionCommandListTicketsByRun:
		items, err := m.tickets.ListByRunID(ctx, common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID)))
		if err != nil {
			return err
		}
		items = filterTicketsByScope(items, envelope)
		return decisionCommandResponse(envelope, map[string]any{events.PayloadKeyTickets: items})
	case events.TopicDecisionCommandListTicketsByWS:
		items, err := m.tickets.ListByWorkspaceID(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return decisionCommandResponse(envelope, map[string]any{events.PayloadKeyTickets: items})
	case events.TopicDecisionCommandUpdateTicket:
		item, err := ticketFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if !ticketMatchesScope(item, envelope) {
			return common.ErrNotFound
		}
		if err := m.tickets.Update(ctx, item); err != nil {
			return err
		}
		return decisionCommandResponse(envelope, nil)
	case events.TopicDecisionCommandDeleteTicket:
		ticketID := common.TicketID(stringPayload(envelope.Payload, events.PayloadKeyBlockingTicketID))
		if err := m.ensureTicketScope(ctx, ticketID, envelope); err != nil {
			return err
		}
		if err := m.tickets.Delete(ctx, ticketID); err != nil {
			return err
		}
		return decisionCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func (m *Module) handleResolutionCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.resolutions == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case events.TopicDecisionCommandCreateResolution:
		item, err := resolutionFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.resolutions.Create(ctx, item); err != nil {
			return err
		}
		return decisionCommandResponse(envelope, nil)
	case events.TopicDecisionCommandGetResolution:
		item, err := m.resolutions.GetByTicketID(ctx, common.TicketID(stringPayload(envelope.Payload, events.PayloadKeyBlockingTicketID)))
		if err != nil {
			return err
		}
		return decisionCommandResponse(envelope, map[string]any{events.PayloadKeyResolution: item})
	case events.TopicDecisionCommandDeleteResolution:
		if err := m.resolutions.DeleteByTicketID(ctx, common.TicketID(stringPayload(envelope.Payload, events.PayloadKeyBlockingTicketID))); err != nil {
			return err
		}
		return decisionCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func (m *Module) ensureTicketScope(ctx context.Context, ticketID common.TicketID, envelope events.Envelope) error {
	if ticketID == "" {
		return common.ErrInvalidArgument
	}
	if !envelopeHasScope(envelope) {
		return nil
	}
	if m == nil || m.tickets == nil {
		return common.ErrInvalidArgument
	}
	item, err := m.tickets.Get(ctx, ticketID)
	if err != nil {
		return err
	}
	if !ticketMatchesScope(item, envelope) {
		return common.ErrNotFound
	}
	return nil
}

func envelopeHasScope(envelope events.Envelope) bool {
	return scopedWorkspaceID(envelope) != "" || scopedRunID(envelope) != "" || scopedStepID(envelope) != ""
}

func scopedWorkspaceID(envelope events.Envelope) common.WorkspaceID {
	workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
	if workspaceID == "" {
		workspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	return workspaceID
}

func scopedRunID(envelope events.Envelope) common.RunID {
	runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
	if runID == "" {
		runID = common.RunID(envelope.RunID)
	}
	return runID
}

func scopedStepID(envelope events.Envelope) common.StepID {
	stepID := common.StepID(stringPayload(envelope.Payload, events.PayloadKeyStepID))
	if stepID == "" {
		stepID = common.StepID(envelope.StepID)
	}
	return stepID
}

func ticketMatchesScope(item *decisionmodel.Ticket, envelope events.Envelope) bool {
	if item == nil {
		return false
	}
	workspaceID := scopedWorkspaceID(envelope)
	if workspaceID != "" && item.WorkspaceID != workspaceID {
		return false
	}
	runID := scopedRunID(envelope)
	if runID != "" && item.RunID != runID {
		return false
	}
	stepID := scopedStepID(envelope)
	if stepID != "" && item.StepID != stepID {
		return false
	}
	return true
}

func filterTicketsByScope(items []*decisionmodel.Ticket, envelope events.Envelope) []*decisionmodel.Ticket {
	if len(items) == 0 {
		return items
	}
	ret := make([]*decisionmodel.Ticket, 0, len(items))
	for _, item := range items {
		if ticketMatchesScope(item, envelope) {
			ret = append(ret, item)
		}
	}
	return ret
}

func ticketFromPayload(payload map[string]any) (*decisionmodel.Ticket, error) {
	item, ok := payload[events.PayloadKeyTicket].(*decisionmodel.Ticket)
	if ok && item != nil {
		return item, nil
	}
	ticketID := common.TicketID(stringPayload(payload, events.PayloadKeyBlockingTicketID))
	if ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	return &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID)),
		RunID:        common.RunID(stringPayload(payload, events.PayloadKeyRunID)),
		StepID:       common.StepID(stringPayload(payload, events.PayloadKeyStepID)),
		Reason:       stringPayload(payload, events.PayloadKeyReason),
		DecisionKind: stringPayload(payload, events.PayloadKeyKind),
		Question:     stringPayload(payload, events.PayloadKeyQuery),
		Context:      mapPayload(payload, events.PayloadKeyContext),
	}, nil
}

func resolutionFromPayload(payload map[string]any) (*decisionmodel.Resolution, error) {
	item, ok := payload[events.PayloadKeyResolution].(*decisionmodel.Resolution)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func mapPayload(payload map[string]any, key string) map[string]any {
	value, _ := payload[key].(map[string]any)
	if value == nil {
		return nil
	}
	ret := make(map[string]any, len(value))
	for key, item := range value {
		ret[key] = item
	}
	return ret
}

func decisionCommandResponse(request events.Envelope, payload map[string]any) error {
	return events.Response(events.Envelope{
		Topic:        request.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  request.SubjectType,
		SubjectID:    request.SubjectID,
		WorkspaceID:  request.WorkspaceID,
		RunID:        request.RunID,
		StepID:       request.StepID,
		Payload:      payload,
	})
}
