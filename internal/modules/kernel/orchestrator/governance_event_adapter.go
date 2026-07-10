package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type decisionEventTicketRepository struct {
	hub events.Hub
}

func (r decisionEventTicketRepository) Create(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendGovernanceOwnerCommand[decisionevents.CreateTicketCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandCreateTicket, decisionevents.CommandDestination, "decision_ticket", string(item.ID), item.WorkspaceID, item.RunID, item.StepID, createTicketCommandFromModel(item))
	return err
}

func (r decisionEventTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	response, err := sendGovernanceOwnerCommand[decisionevents.GetTicketCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandGetTicket, decisionevents.CommandDestination, "decision_ticket", string(id), "", "", "", decisionevents.GetTicketCommand{TicketID: id})
	if err != nil {
		return nil, err
	}
	item := response.Ticket
	if item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r decisionEventTicketRepository) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	response, err := sendGovernanceOwnerCommand[decisionevents.ListTicketsCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandListTickets, decisionevents.CommandDestination, "decision_ticket", "all", "", "", "", decisionevents.ListTicketsCommand{})
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}

func (r decisionEventTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	response, err := sendGovernanceOwnerCommand[decisionevents.ListTicketsByRunCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandListTicketsByRun, decisionevents.CommandDestination, "run", string(runID), "", runID, "", decisionevents.ListTicketsByRunCommand{RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}

func (r decisionEventTicketRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	response, err := sendGovernanceOwnerCommand[decisionevents.ListTicketsByWorkspaceCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandListTicketsByWS, decisionevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", "", decisionevents.ListTicketsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}

func (r decisionEventTicketRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendGovernanceOwnerCommand[decisionevents.UpdateTicketCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandUpdateTicket, decisionevents.CommandDestination, "decision_ticket", string(item.ID), item.WorkspaceID, item.RunID, item.StepID, updateTicketCommandFromModel(item))
	return err
}

func (r decisionEventTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	_, err := sendGovernanceOwnerCommand[decisionevents.DeleteTicketCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandDeleteTicket, decisionevents.CommandDestination, "decision_ticket", string(id), "", "", "", decisionevents.DeleteTicketCommand{TicketID: id})
	return err
}

type decisionEventResolutionRepository struct {
	hub events.Hub
}

func (r decisionEventResolutionRepository) Create(ctx context.Context, item *decisionmodel.Resolution) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendGovernanceOwnerCommand[decisionevents.CreateResolutionCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandCreateResolution, decisionevents.CommandDestination, "decision_resolution", string(item.ID), "", "", "", createResolutionCommandFromModel(item))
	return err
}

func (r decisionEventResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	response, err := sendGovernanceOwnerCommand[decisionevents.GetResolutionCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandGetResolution, decisionevents.CommandDestination, "decision_ticket", string(ticketID), "", "", "", decisionevents.GetResolutionCommand{TicketID: ticketID})
	if err != nil {
		return nil, err
	}
	item := response.Resolution
	if item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r decisionEventResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	_, err := sendGovernanceOwnerCommand[decisionevents.DeleteResolutionCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandDeleteResolution, decisionevents.CommandDestination, "decision_ticket", string(ticketID), "", "", "", decisionevents.DeleteResolutionCommand{TicketID: ticketID})
	return err
}

type approvalEventRepository struct {
	hub events.Hub
}

func (r approvalEventRepository) Create(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendGovernanceOwnerCommand[approvalevents.CreateApprovalCommand, approvalevents.ApprovalCommandResult](ctx, r.hub, approvalevents.TopicApprovalCommandCreate, approvalevents.CommandDestination, "approval", string(item.ID), item.WorkspaceID, item.RunID, item.StepID, createApprovalCommandFromModel(item))
	return err
}

func (r approvalEventRepository) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	response, err := sendGovernanceOwnerCommand[approvalevents.GetApprovalCommand, approvalevents.ApprovalCommandResult](ctx, r.hub, approvalevents.TopicApprovalCommandGet, approvalevents.CommandDestination, "approval", string(id), "", "", "", approvalevents.GetApprovalCommand{ApprovalID: id})
	if err != nil {
		return nil, err
	}
	item := response.Approval
	if item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r approvalEventRepository) List(ctx context.Context) ([]*approvalmodel.Approval, error) {
	response, err := sendGovernanceOwnerCommand[approvalevents.ListApprovalsCommand, approvalevents.ApprovalCommandResult](ctx, r.hub, approvalevents.TopicApprovalCommandList, approvalevents.CommandDestination, "approval", "all", "", "", "", approvalevents.ListApprovalsCommand{})
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func (r approvalEventRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*approvalmodel.Approval, error) {
	response, err := sendGovernanceOwnerCommand[approvalevents.ListApprovalsByRunCommand, approvalevents.ApprovalCommandResult](ctx, r.hub, approvalevents.TopicApprovalCommandListByRun, approvalevents.CommandDestination, "run", string(runID), "", runID, "", approvalevents.ListApprovalsByRunCommand{RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func (r approvalEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	response, err := sendGovernanceOwnerCommand[approvalevents.ListApprovalsByWorkspaceCommand, approvalevents.ApprovalCommandResult](ctx, r.hub, approvalevents.TopicApprovalCommandListByWorkspace, approvalevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", "", approvalevents.ListApprovalsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func (r approvalEventRepository) Update(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendGovernanceOwnerCommand[approvalevents.UpdateApprovalCommand, approvalevents.ApprovalCommandResult](ctx, r.hub, approvalevents.TopicApprovalCommandUpdate, approvalevents.CommandDestination, "approval", string(item.ID), item.WorkspaceID, item.RunID, item.StepID, updateApprovalCommandFromModel(item))
	return err
}

func (r approvalEventRepository) Delete(ctx context.Context, id common.ApprovalID) error {
	_, err := sendGovernanceOwnerCommand[approvalevents.DeleteApprovalCommand, approvalevents.ApprovalCommandResult](ctx, r.hub, approvalevents.TopicApprovalCommandDelete, approvalevents.CommandDestination, "approval", string(id), "", "", "", approvalevents.DeleteApprovalCommand{ApprovalID: id})
	return err
}

type rawMagicHubProvider interface {
	RawHub() magicevent.Hub
}

func sendGovernanceOwnerCommand[Command any, Result any](ctx context.Context, hub events.Hub, topic string, destination string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, command Command) (Result, error) {
	var zero Result
	if hub == nil || topic == "" {
		return zero, common.ErrInvalidArgument
	}
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	provider, ok := hub.(rawMagicHubProvider)
	if !ok || provider.RawHub() == nil {
		return zero, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(topic, ModuleID, destination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKey(events.Envelope{
		Topic:       topic,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		StepID:      string(stepID),
	}))
	response := provider.RawHub().Send(ev)
	if response == nil {
		return zero, common.ErrInvalidArgument
	}
	if responseErr := response.Error(); responseErr != nil {
		return zero, responseErr
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return zero, resultErr
	}
	typed, ok := data.(Result)
	if !ok {
		return zero, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return typed, nil
}

func createTicketCommandFromModel(item *decisionmodel.Ticket) decisionevents.CreateTicketCommand {
	if item == nil {
		return decisionevents.CreateTicketCommand{}
	}
	return decisionevents.CreateTicketCommand{
		TicketID:     item.ID,
		WorkspaceID:  item.WorkspaceID,
		RunID:        item.RunID,
		StepID:       item.StepID,
		Reason:       item.Reason,
		DecisionKind: item.DecisionKind,
		Question:     item.Question,
		Context:      cloneMap(item.Context),
	}
}

func updateTicketCommandFromModel(item *decisionmodel.Ticket) decisionevents.UpdateTicketCommand {
	if item == nil {
		return decisionevents.UpdateTicketCommand{}
	}
	return decisionevents.UpdateTicketCommand{
		TicketID:     item.ID,
		WorkspaceID:  item.WorkspaceID,
		RunID:        item.RunID,
		StepID:       item.StepID,
		Reason:       item.Reason,
		DecisionKind: item.DecisionKind,
		Question:     item.Question,
		Context:      cloneMap(item.Context),
		State:        item.State,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

func createResolutionCommandFromModel(item *decisionmodel.Resolution) decisionevents.CreateResolutionCommand {
	if item == nil {
		return decisionevents.CreateResolutionCommand{}
	}
	return decisionevents.CreateResolutionCommand{
		ResolutionID: item.ID,
		TicketID:     item.TicketID,
		Type:         item.Type,
		Payload:      cloneMap(item.Payload),
	}
}

func createApprovalCommandFromModel(item *approvalmodel.Approval) approvalevents.CreateApprovalCommand {
	if item == nil {
		return approvalevents.CreateApprovalCommand{}
	}
	return approvalevents.CreateApprovalCommand{
		ApprovalID:  item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      item.StepID,
		Reason:      item.Reason,
		Action:      item.Action,
		Context:     cloneMap(item.Context),
	}
}

func updateApprovalCommandFromModel(item *approvalmodel.Approval) approvalevents.UpdateApprovalCommand {
	if item == nil {
		return approvalevents.UpdateApprovalCommand{}
	}
	return approvalevents.UpdateApprovalCommand{
		ApprovalID:  item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      item.StepID,
		Reason:      item.Reason,
		Action:      item.Action,
		Context:     cloneMap(item.Context),
		State:       item.State,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}
