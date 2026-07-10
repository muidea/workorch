package decision

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisionbiz "github.com/muidea/workorch/internal/modules/kernel/decision/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = decisionservice.Service
type CreateTicketCommand = decisioncontracts.CreateTicketCommand
type UpdateTicketCommand = decisioncontracts.UpdateTicketCommand
type ResolveTicketCommand = decisioncontracts.ResolveTicketCommand

type Dependencies struct {
	Tickets         repository.DecisionTicketRepository
	Resolutions     repository.DecisionResolutionRepository
	EvidenceRecords repository.EvidenceRecordRepository
	EventHub        magicevent.Hub
}

type Module struct {
	bizPtr      *decisionbiz.Service
	servicePtr  *decisionservice.Module
	eventHub    magicevent.Hub
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	observer    magicevent.SimpleObserver
	topics      []string
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
	eventHubBridge := events.NewMagicHubWithHubOwner(deps.EventHub, ModuleID)
	m.bizPtr = decisionbiz.NewService(deps.Tickets, deps.Resolutions, eventHubBridge, deps.EvidenceRecords)
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
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.topics = []string{
		decisionevents.TopicDecisionCommandEnsureResolved,
		decisionevents.TopicDecisionCommandEnsureTicket,
		decisionevents.TopicDecisionCommandCreateTicket,
		decisionevents.TopicDecisionCommandGetTicket,
		decisionevents.TopicDecisionCommandListTickets,
		decisionevents.TopicDecisionCommandListTicketsByRun,
		decisionevents.TopicDecisionCommandListTicketsByWS,
		decisionevents.TopicDecisionCommandUpdateTicket,
		decisionevents.TopicDecisionCommandDeleteTicket,
		decisionevents.TopicDecisionCommandCreateResolution,
		decisionevents.TopicDecisionCommandGetResolution,
		decisionevents.TopicDecisionCommandDeleteResolution,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.observer != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.observer.Unsubscribe(m.topics[idx])
		}
	}
	m.topics = nil
	m.observer = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.eventHub = nil
	m.tickets = nil
	m.resolutions = nil
	return nil
}

func (m *Module) Service() Service {
	if m == nil || m.servicePtr == nil {
		return nil
	}
	return m.servicePtr
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev.Context(), ev.ID(), ev.Data())
	setDecisionMagicResult(result, response, err)
}

func (m *Module) executeCommand(ctx context.Context, topic string, data any) (decisionevents.DecisionCommandResult, error) {
	if m == nil || m.tickets == nil || m.resolutions == nil || m.servicePtr == nil {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	if workspaceID := decisionCommandWorkspaceID(data); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch topic {
	case decisionevents.TopicDecisionCommandEnsureResolved:
		cmd, ok := data.(decisionevents.EnsureResolvedCommand)
		if !ok {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		if !decisionCommandHasScope(cmd) {
			ticket, err := m.resolveUniqueTicketForUnscopedCommand(ctx, cmd.TicketID)
			if err != nil {
				return decisionevents.DecisionCommandResult{}, err
			}
			cmd.WorkspaceID = ticket.WorkspaceID
			cmd.RunID = ticket.RunID
			cmd.StepID = ticket.StepID
			ctx = common.ContextWithRuntimeWorkspaceID(ctx, ticket.WorkspaceID)
		}
		ticket, resolution, err := m.servicePtr.EnsureResolvedTicketAndReturn(ctx, cmd)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{Ticket: ticket, Resolution: resolution}, nil
	case decisionevents.TopicDecisionCommandEnsureTicket:
		cmd, ok := data.(decisionevents.EnsureTicketCommand)
		if !ok {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.EnsureTicket(ctx, cmd); err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{}, nil
	case decisionevents.TopicDecisionCommandCreateTicket:
		cmd, ok := data.(decisionevents.CreateTicketCommand)
		if !ok {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.CreateTicket(ctx, cmd); err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{}, nil
	case decisionevents.TopicDecisionCommandGetTicket:
		cmd, ok := data.(decisionevents.GetTicketCommand)
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
		if _, ok := data.(decisionevents.ListTicketsCommand); !ok {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.tickets.List(ctx)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{Tickets: items}, nil
	case decisionevents.TopicDecisionCommandListTicketsByRun:
		cmd, ok := data.(decisionevents.ListTicketsByRunCommand)
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
		cmd, ok := data.(decisionevents.ListTicketsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.tickets.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{Tickets: items}, nil
	case decisionevents.TopicDecisionCommandUpdateTicket:
		cmd, ok := data.(decisionevents.UpdateTicketCommand)
		if !ok || cmd.TicketID == "" {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		item := ticketFromUpdateCommand(cmd)
		if !ticketMatchesDecisionScope(item, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID}) {
			return decisionevents.DecisionCommandResult{}, common.ErrNotFound
		}
		if err := m.tickets.Update(ctx, item); err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if err := m.publishTicketUpdated(ctx, item); err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{}, nil
	case decisionevents.TopicDecisionCommandDeleteTicket:
		cmd, ok := data.(decisionevents.DeleteTicketCommand)
		if !ok || cmd.TicketID == "" {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		scope := decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID}
		if !scope.hasScope() {
			ticket, err := m.resolveUniqueTicketForUnscopedCommand(ctx, cmd.TicketID)
			if err != nil {
				return decisionevents.DecisionCommandResult{}, err
			}
			ctx = common.ContextWithRuntimeWorkspaceID(ctx, ticket.WorkspaceID)
		} else if err := m.ensureTicketScopeForCommand(ctx, cmd.TicketID, scope); err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if err := m.tickets.Delete(ctx, cmd.TicketID); err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{}, nil
	case decisionevents.TopicDecisionCommandCreateResolution:
		cmd, ok := data.(decisionevents.CreateResolutionCommand)
		if !ok {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		item, err := resolutionFromCreateCommand(cmd)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		resolutionCtx, err := m.resolveResolutionTicketContextForCommand(ctx, item.TicketID, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID})
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if err := m.resolutions.Create(resolutionCtx, item); err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{}, nil
	case decisionevents.TopicDecisionCommandGetResolution:
		cmd, ok := data.(decisionevents.GetResolutionCommand)
		if !ok || cmd.TicketID == "" {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		resolutionCtx, err := m.resolveResolutionTicketContextForCommand(ctx, cmd.TicketID, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID})
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		item, err := m.resolutions.GetByTicketID(resolutionCtx, cmd.TicketID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{Resolution: item}, nil
	case decisionevents.TopicDecisionCommandDeleteResolution:
		cmd, ok := data.(decisionevents.DeleteResolutionCommand)
		if !ok || cmd.TicketID == "" {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		resolutionCtx, err := m.resolveResolutionTicketContextForCommand(ctx, cmd.TicketID, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID})
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if err := m.resolutions.DeleteByTicketID(resolutionCtx, cmd.TicketID); err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{}, nil
	default:
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
}

func setDecisionMagicResult(result magicevent.Result, data decisionevents.DecisionCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func (m *Module) publishTicketUpdated(ctx context.Context, item *decisionmodel.Ticket) error {
	if m == nil || m.eventHub == nil || item == nil {
		return nil
	}
	return events.PublishByContract(ctx, events.NewMagicHubWithHubOwner(m.eventHub, ModuleID), events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        decisionevents.TopicTicketUpdated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(item.StepID),
		Payload:      events.DecisionTicketPayload(item.Reason, item.State),
	})
}

type decisionCommandScope struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

func (s decisionCommandScope) hasScope() bool {
	return s.WorkspaceID != "" || s.RunID != "" || s.StepID != ""
}

func decisionCommandWorkspaceID(data any) common.WorkspaceID {
	switch cmd := data.(type) {
	case decisionevents.CreateTicketCommand:
		return cmd.WorkspaceID
	case decisionevents.UpdateTicketCommand:
		return cmd.WorkspaceID
	case decisionevents.DeleteTicketCommand:
		return cmd.WorkspaceID
	case decisionevents.CreateResolutionCommand:
		return cmd.WorkspaceID
	case decisionevents.EnsureResolvedCommand:
		return cmd.WorkspaceID
	case decisionevents.DeleteResolutionCommand:
		return cmd.WorkspaceID
	case decisionevents.GetTicketCommand:
		return cmd.WorkspaceID
	case decisionevents.ListTicketsByRunCommand:
		return cmd.WorkspaceID
	case decisionevents.ListTicketsByWorkspaceCommand:
		return cmd.WorkspaceID
	case decisionevents.GetResolutionCommand:
		return cmd.WorkspaceID
	default:
		return ""
	}
}

func decisionCommandHasScope(cmd decisionevents.EnsureResolvedCommand) bool {
	return cmd.WorkspaceID != "" || cmd.RunID != "" || cmd.StepID != ""
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

func ticketFromUpdateCommand(cmd decisioncontracts.UpdateTicketCommand) *decisionmodel.Ticket {
	return &decisionmodel.Ticket{
		ID:                     cmd.TicketID,
		WorkspaceID:            cmd.WorkspaceID,
		RunID:                  cmd.RunID,
		StepID:                 cmd.StepID,
		Reason:                 cmd.Reason,
		DecisionKind:           cmd.DecisionKind,
		Question:               cmd.Question,
		Context:                cloneMap(cmd.Context),
		State:                  cmd.State,
		AutoGovernance:         cloneMap(cmd.AutoGovernance),
		FailureClass:           cmd.FailureClass,
		FailureSignature:       cmd.FailureSignature,
		AllowedRecoveryActions: append([]string(nil), cmd.AllowedRecoveryActions...),
		ResolvedAt:             cmd.ResolvedAt,
		ResolvedBy:             cmd.ResolvedBy,
		CreatedAt:              cmd.CreatedAt,
		UpdatedAt:              cmd.UpdatedAt,
	}
}

func (m *Module) resolveResolutionTicketContextForCommand(ctx context.Context, ticketID common.TicketID, scope decisionCommandScope) (context.Context, error) {
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
			ctx = common.ContextWithRuntimeWorkspaceID(ctx, ticket.WorkspaceID)
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
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	}
	return ctx, nil
}

func (m *Module) ensureTicketScopeForCommand(ctx context.Context, ticketID common.TicketID, scope decisionCommandScope) error {
	if ticketID == "" {
		return common.ErrInvalidArgument
	}
	if m == nil || m.tickets == nil {
		return common.ErrInvalidArgument
	}
	if !scope.hasScope() {
		_, err := m.resolveUniqueTicketForUnscopedCommand(ctx, ticketID)
		return err
	}
	item, err := m.tickets.Get(ctx, ticketID)
	if err != nil {
		return err
	}
	if !ticketMatchesDecisionScope(item, scope) {
		return common.ErrNotFound
	}
	return nil
}

func (m *Module) resolveUniqueTicketForUnscopedCommand(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
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
	var openMatches []*decisionmodel.Ticket
	for _, item := range items {
		if item == nil || item.ID != ticketID {
			continue
		}
		matches = append(matches, item)
		if item.State == common.DecisionTicketStateOpen {
			openMatches = append(openMatches, item)
		}
	}
	switch {
	case len(openMatches) == 1:
		return openMatches[0], nil
	case len(openMatches) > 1:
		return nil, common.ErrConflict
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return nil, common.ErrConflict
	default:
		return nil, common.ErrNotFound
	}
}

func resolutionFromCreateCommand(cmd decisionevents.CreateResolutionCommand) (*decisionmodel.Resolution, error) {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return nil, common.ErrInvalidArgument
	}
	if !modulecommon.IsSupportedResolutionType(cmd.Type) {
		return nil, common.ErrInvalidArgument
	}
	return &decisionmodel.Resolution{
		ID:                 cmd.ResolutionID,
		TicketID:           cmd.TicketID,
		Type:               cmd.Type,
		Payload:            cloneMap(cmd.Payload),
		ContinuationAction: nonEmptyString(cmd.ContinuationAction, stringPayloadValue(cmd.Payload, "continuationAction")),
		ResumeRunID:        nonEmptyRunID(cmd.ResumeRunID, common.RunID(stringPayloadValue(cmd.Payload, "resumeRunId"))),
		SuccessorRunID:     nonEmptyRunID(cmd.SuccessorRunID, common.RunID(stringPayloadValue(cmd.Payload, "successorRunId"))),
	}, nil
}

func stringPayloadValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func nonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func nonEmptyRunID(values ...common.RunID) common.RunID {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneMap(value map[string]any) map[string]any {
	ret := make(map[string]any, len(value))
	for key, item := range value {
		ret[key] = item
	}
	return ret
}
