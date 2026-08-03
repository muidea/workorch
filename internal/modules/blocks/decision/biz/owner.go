package decision

import (
	"context"
	"errors"
	"strings"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/contracts"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Tickets           repository.DecisionTicketRepository
	Resolutions       repository.DecisionResolutionRepository
	EventHub          magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service         *Service
	tickets         repository.DecisionTicketRepository
	resolutions     repository.DecisionResolutionRepository
	topics          []string
	started         bool
	reconcileMu     sync.Mutex
	reconcileCancel context.CancelFunc
	reconcileDone   chan struct{}
}

func New(h magicevent.Hub, background magictask.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Tickets == nil || deps.Resolutions == nil || deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, background), service: NewService(deps.Tickets, deps.Resolutions), tickets: deps.Tickets, resolutions: deps.Resolutions}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
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
		m.SubscribeFunc(topic, m.handleCommand)
	}
	m.topics = append(m.topics, decisionevents.TopicOpenRuntimeGovernanceRequest)
	m.SubscribeFunc(decisionevents.TopicOpenRuntimeGovernanceRequest, m.handleRuntimeGovernanceRequest)
	m.topics = append(m.topics, decisionevents.TopicAcquireRuntimeCapabilities)
	m.SubscribeFunc(decisionevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	if err := m.startRuntimeGovernanceProposalReconciliation(); err != nil {
		return err
	}
	m.started = true
	return nil
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(decisionevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setDecisionRuntimeCapabilityResult(result, decisionevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument)
		return
	}
	response, err := decisionevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	setDecisionRuntimeCapabilityResult(result, response, err)
}

func setDecisionRuntimeCapabilityResult(result magicevent.Result, response decisionevents.AcquireRuntimeCapabilitiesResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeGovernanceRequest(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(decisionevents.OpenRuntimeGovernanceRequestCommand)
	if !ok || m == nil || m.service == nil || command.TicketID == "" || command.WorkspaceID == "" || command.RunID == "" ||
		command.DispatchID == "" || command.Transaction.AuthoritySnapshotID == "" || command.Dependency.Kind == "" || command.Dependency.Reason == "" ||
		!modulecommon.IsSupportedResolutionType(command.ContinueResolutionType) || !modulecommon.IsSupportedResolutionType(command.FailResolutionType) {
		setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{}, common.ErrInvalidArgument)
		return
	}
	ticket, created, err := m.service.EnsureTicketAndReturn(event.Context(), decisioncontracts.CreateTicketCommand{
		TicketID: command.TicketID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		StepID: common.StepID("runtime/" + command.DispatchID), Reason: command.Dependency.Reason,
		DecisionKind: "runtime_governance", Question: command.Dependency.Reason,
		FailureClass: "governance_dependency", FailureSignature: command.Dependency.Kind + ":" + command.Dependency.SubjectRef,
		AllowedRecoveryActions: []string{command.ContinueResolutionType, command.FailResolutionType},
	})
	if err != nil {
		setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{}, err)
		return
	}
	if created {
		if err := m.publishTicketCreated(event.Context(), ticket); err != nil {
			setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{}, err)
			return
		}
	}
	setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{
		TicketID: command.TicketID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		DispatchID: command.DispatchID, AuthoritySnapshotID: command.Transaction.AuthoritySnapshotID,
	}, nil)
}

func setRuntimeGovernanceRequestResult(result magicevent.Result, response decisionevents.OpenRuntimeGovernanceRequestResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.stopRuntimeGovernanceProposalReconciliation()
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.topics = nil
	m.service = nil
	m.tickets = nil
	m.resolutions = nil
	m.started = false
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setDecisionMagicResult(result, response, err)
}

func (m *Biz) executeCommand(ev magicevent.Event) (decisionevents.DecisionCommandResult, error) {
	ctx := ev.Context()
	topic := ev.ID()
	data := ev.Data()
	if m == nil || m.tickets == nil || m.resolutions == nil || m.service == nil {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID("")
	switch cmd := data.(type) {
	case decisionevents.CreateTicketCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.UpdateTicketCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.DeleteTicketCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.CreateResolutionCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.EnsureResolvedCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.DeleteResolutionCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.GetTicketCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.ListTicketsByRunCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.ListTicketsByWorkspaceCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.GetResolutionCommand:
		workspaceID = cmd.WorkspaceID
	}
	if workspaceID != "" {
		var scopeErr error
		ctx, scopeErr = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
		if scopeErr != nil {
			return decisionevents.DecisionCommandResult{}, scopeErr
		}
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
			ctx, err = common.BindRuntimeWorkspaceContext(ctx, ticket.WorkspaceID)
			if err != nil {
				return decisionevents.DecisionCommandResult{}, err
			}
		}
		resolveCommand := resolveTicketContract(cmd)
		if cmd.DecisionKind == "runtime_governance" {
			currentTicket, err := m.tickets.Get(ctx, cmd.TicketID)
			if err != nil {
				return decisionevents.DecisionCommandResult{}, err
			}
			if currentTicket.DecisionKind != cmd.DecisionKind {
				return decisionevents.DecisionCommandResult{}, common.ErrConflict
			}
			ticket, proposal, err := m.service.EnsureRuntimeGovernanceResolutionProposal(ctx, resolveCommand)
			if err != nil {
				return decisionevents.DecisionCommandResult{}, err
			}
			commit, err := m.commitRuntimeGovernanceTicketProposal(ctx, ticket, proposal)
			if err != nil {
				if errors.Is(err, common.ErrConflict) || errors.Is(err, common.ErrInvalidArgument) {
					_ = m.service.RejectRuntimeGovernanceResolutionProposal(ctx, resolveCommand, "aggregate_commit_rejected", err.Error())
				}
				return decisionevents.DecisionCommandResult{}, err
			}
			switch commit.Disposition {
			case orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted,
				orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate:
				ticket, resolution, transitioned, err := m.service.FinalizeRuntimeGovernanceResolutionProposalWithTransition(ctx, resolveCommand)
				if err != nil {
					return decisionevents.DecisionCommandResult{}, err
				}
				// The aggregate and public Decision resolution are already durable on a
				// duplicate replay. Re-emitting Decision evidence would create a second
				// owner-side audit effect even though the Runtime continuation is
				// only being joined after restart.
				if transitioned {
					if err := m.recordAndPublishResolvedTicket(ctx, ticket, resolution, true); err != nil {
						return decisionevents.DecisionCommandResult{}, err
					}
				}
				return decisionevents.DecisionCommandResult{Ticket: ticket, Resolution: resolution}, nil
			case orchestratorevents.RuntimeGovernanceResolutionDispositionStale:
				if err := m.service.RejectRuntimeGovernanceResolutionProposal(ctx, resolveCommand, "aggregate_commit_stale", "runtime governance proposal belongs to a stale aggregate authority"); err != nil {
					return decisionevents.DecisionCommandResult{}, err
				}
				return decisionevents.DecisionCommandResult{}, common.ErrConflict
			default:
				return decisionevents.DecisionCommandResult{}, common.ErrConflict
			}
		}
		ticket, resolution, transitioned, err := m.service.EnsureResolvedTicketAndReturnWithTransition(ctx, resolveCommand)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if err := m.recordAndPublishResolvedTicket(ctx, ticket, resolution, transitioned); err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		return decisionevents.DecisionCommandResult{Ticket: ticket, Resolution: resolution}, nil
	case decisionevents.TopicDecisionCommandEnsureTicket:
		cmd, ok := data.(decisionevents.EnsureTicketCommand)
		if !ok {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		ticket, created, err := m.service.EnsureTicketAndReturn(ctx, createTicketContract(cmd))
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if created {
			if err := m.publishTicketCreated(ctx, ticket); err != nil {
				return decisionevents.DecisionCommandResult{}, err
			}
		}
		return decisionevents.DecisionCommandResult{}, nil
	case decisionevents.TopicDecisionCommandCreateTicket:
		cmd, ok := data.(decisionevents.CreateTicketCommand)
		if !ok {
			return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
		}
		ticket, err := m.service.CreateTicketAndReturn(ctx, createTicketContract(cmd))
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if err := m.publishTicketCreated(ctx, ticket); err != nil {
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
		current, err := m.tickets.Get(ctx, cmd.TicketID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if !ticketMatchesDecisionScope(current, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID}) {
			return decisionevents.DecisionCommandResult{}, common.ErrNotFound
		}
		item := ticketFromUpdateCommand(current, cmd)
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
		ticket, err := m.tickets.Get(resolutionCtx, item.TicketID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		if ticket.DecisionKind == "runtime_governance" {
			return decisionevents.DecisionCommandResult{}, common.ErrConflict
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

func (m *Biz) commitRuntimeGovernanceTicketProposal(ctx context.Context, ticket *decisionmodel.Ticket, proposal *decisionmodel.RuntimeGovernanceResolutionProposal) (orchestratorevents.CommitRuntimeGovernanceResolutionResult, error) {
	if m == nil || m.EventHub() == nil || ticket == nil || proposal == nil || ticket.DecisionKind != "runtime_governance" {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	outcome := runtimecontract.RuntimeGovernanceResolutionOutcome("")
	switch proposal.ResolutionType {
	case modulecommon.ResolutionTypeProvideMissingInfo:
		outcome = runtimecontract.RuntimeGovernanceResolutionOutcomeContinue
	case modulecommon.ResolutionTypeRejectCurrentPath:
		outcome = runtimecontract.RuntimeGovernanceResolutionOutcomeFail
	default:
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	result := m.SendEvent(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
		ModuleID,
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		orchestratorevents.CommitRuntimeGovernanceTicketResolutionCommand{
			ResolutionID: proposal.ResolutionID, TicketID: ticket.ID,
			WorkspaceID: ticket.WorkspaceID, RunID: ticket.RunID,
			Outcome: outcome, Summary: runtimeGovernanceProposalSummary(ticket, proposal),
		},
	))
	if result == nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, resultErr
	}
	response, ok := data.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if !ok || response.WorkspaceID != ticket.WorkspaceID || response.RunID != ticket.RunID || response.ResolutionID != proposal.ResolutionID {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	if err := response.Validate(); err != nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err
	}
	return response, nil
}

func runtimeGovernanceProposalSummary(ticket *decisionmodel.Ticket, proposal *decisionmodel.RuntimeGovernanceResolutionProposal) string {
	if proposal != nil {
		if value, ok := proposal.Payload.String("summary"); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if ticket != nil && strings.TrimSpace(ticket.Reason) != "" {
		return strings.TrimSpace(ticket.Reason)
	}
	return "runtime governance resolution committed"
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

func (m *Biz) publishTicketUpdated(ctx context.Context, item *decisionmodel.Ticket) error {
	if m == nil || m.EventHub() == nil || item == nil {
		return nil
	}
	topic := decisionevents.TopicTicketUpdated
	if item.State == common.DecisionTicketStateSuperseded {
		topic = decisionevents.TopicTicketSuperseded
	}
	return m.postDecisionNotification(ctx, topic, ticketChangedData(item), item)
}

func (m *Biz) publishTicketCreated(ctx context.Context, ticket *decisionmodel.Ticket) error {
	return m.postDecisionNotification(ctx, decisionevents.TopicTicketCreated, ticketChangedData(ticket), ticket)
}

func (m *Biz) recordAndPublishResolvedTicket(ctx context.Context, ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution, transitioned bool) error {
	if err := m.recordDecisionResolutionEvidence(ctx, ticket, resolution); err != nil {
		return err
	}
	if !transitioned {
		return nil
	}
	return m.postDecisionNotification(ctx, decisionevents.TopicTicketResolved, decisionevents.TicketResolvedData{
		Ticket:             ticketChangedData(ticket),
		ResolutionID:       resolution.ID,
		ResolutionType:     resolution.Type,
		ResolutionPayload:  resolution.Payload.Clone().Fields,
		ContinuationAction: resolution.ContinuationAction,
		ResumeRunID:        resolution.ResumeRunID,
		SuccessorRunID:     resolution.SuccessorRunID,
	}, ticket)
}

func ticketChangedData(ticket *decisionmodel.Ticket) decisionevents.TicketChangedData {
	if ticket == nil {
		return decisionevents.TicketChangedData{}
	}
	return decisionevents.TicketChangedData{
		TicketID:     ticket.ID,
		WorkspaceID:  ticket.WorkspaceID,
		RunID:        ticket.RunID,
		StepID:       ticket.StepID,
		Reason:       ticket.Reason,
		DecisionKind: ticket.DecisionKind,
		State:        ticket.State,
	}
}

func (m *Biz) postDecisionNotification(ctx context.Context, topic string, data any, ticket *decisionmodel.Ticket) error {
	if m == nil || m.EventHub() == nil || ticket == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, decisionevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), events.EventHubLaneScope{
		SubjectType: "decision_ticket",
		SubjectID:   string(ticket.ID),
		WorkspaceID: string(ticket.WorkspaceID),
		RunID:       string(ticket.RunID),
		StepID:      string(ticket.StepID),
	}))
	m.PostEvent(event)
	return nil
}

type decisionCommandScope struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

func (s decisionCommandScope) hasScope() bool {
	return s.WorkspaceID != "" || s.RunID != "" || s.StepID != ""
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

func createTicketContract(cmd decisionevents.CreateTicketCommand) decisioncontracts.CreateTicketCommand {
	return decisioncontracts.CreateTicketCommand{
		TicketID:               cmd.TicketID,
		WorkspaceID:            cmd.WorkspaceID,
		RunID:                  cmd.RunID,
		StepID:                 cmd.StepID,
		Reason:                 cmd.Reason,
		DecisionKind:           cmd.DecisionKind,
		Question:               cmd.Question,
		Context:                workorcheventpayload.New(cmd.Context...),
		AutoGovernance:         workorcheventpayload.New(cmd.AutoGovernance...),
		FailureClass:           cmd.FailureClass,
		FailureSignature:       cmd.FailureSignature,
		AllowedRecoveryActions: append([]string(nil), cmd.AllowedRecoveryActions...),
	}
}

func resolveTicketContract(cmd decisionevents.EnsureResolvedCommand) decisioncontracts.ResolveTicketCommand {
	return decisioncontracts.ResolveTicketCommand{
		ResolutionID:       cmd.ResolutionID,
		TicketID:           cmd.TicketID,
		WorkspaceID:        cmd.WorkspaceID,
		RunID:              cmd.RunID,
		StepID:             cmd.StepID,
		Type:               cmd.Type,
		Payload:            workorcheventpayload.New(cmd.Payload...),
		ContinuationAction: cmd.ContinuationAction,
		ResumeRunID:        cmd.ResumeRunID,
		SuccessorRunID:     cmd.SuccessorRunID,
	}
}

func ticketFromUpdateCommand(current *decisionmodel.Ticket, cmd decisionevents.UpdateTicketCommand) *decisionmodel.Ticket {
	if current == nil {
		return nil
	}
	item := *current
	item.Reason = cmd.Reason
	item.DecisionKind = cmd.DecisionKind
	item.Question = cmd.Question
	item.Context = workorcheventpayload.New(cmd.Context...)
	item.AutoGovernance = workorcheventpayload.New(cmd.AutoGovernance...)
	item.FailureClass = cmd.FailureClass
	item.FailureSignature = cmd.FailureSignature
	item.AllowedRecoveryActions = append([]string(nil), cmd.AllowedRecoveryActions...)
	item.UpdatedAt = cmd.UpdatedAt
	return &item
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

func (m *Biz) ensureTicketScopeForCommand(ctx context.Context, ticketID common.TicketID, scope decisionCommandScope) error {
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
	payload := workorcheventpayload.New(cmd.Payload...)
	return &decisionmodel.Resolution{
		ID:                 cmd.ResolutionID,
		TicketID:           cmd.TicketID,
		Type:               cmd.Type,
		Payload:            payload,
		ContinuationAction: nonEmptyString(cmd.ContinuationAction, stringPayloadValue(payload, "continuationAction")),
		ResumeRunID:        nonEmptyRunID(cmd.ResumeRunID, common.RunID(stringPayloadValue(payload, "resumeRunId"))),
		SuccessorRunID:     nonEmptyRunID(cmd.SuccessorRunID, common.RunID(stringPayloadValue(payload, "successorRunId"))),
	}, nil
}
