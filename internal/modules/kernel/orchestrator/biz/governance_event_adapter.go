package orchestrator

// Governance adapter is internal to the Runtime V2 owner Biz.

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type decisionEventTicketRepository struct {
	hub magicevent.Hub
}

func (r decisionEventTicketRepository) Create(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	command, err := createTicketCommandFromModel(item)
	if err != nil {
		return err
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandCreateTicket, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandCreateTicket, "decision_ticket", string(item.ID), item.WorkspaceID, item.RunID, item.StepID)
	_, err = decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandCreateTicket)
	return err
}

func (r decisionEventTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandGetTicket, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.GetTicketCommand{TicketID: id})
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandGetTicket, "decision_ticket", string(id), workspaceID, "", "")
	response, err := decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandGetTicket)
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
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTickets, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsCommand{})
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandListTickets, "decision_ticket", "all", workspaceID, "", "")
	response, err := decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandListTickets)
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}

func (r decisionEventTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTicketsByRun, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsByRunCommand{RunID: runID})
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandListTicketsByRun, "run", string(runID), workspaceID, runID, "")
	response, err := decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandListTicketsByRun)
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}

func (r decisionEventTicketRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTicketsByWS, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsByWorkspaceCommand{WorkspaceID: workspaceID})
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandListTicketsByWS, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandListTicketsByWS)
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}

func (r decisionEventTicketRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	command, err := updateTicketCommandFromModel(item)
	if err != nil {
		return err
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandUpdateTicket, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandUpdateTicket, "decision_ticket", string(item.ID), item.WorkspaceID, item.RunID, item.StepID)
	_, err = decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandUpdateTicket)
	return err
}

func (r decisionEventTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandDeleteTicket, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.DeleteTicketCommand{TicketID: id})
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandDeleteTicket, "decision_ticket", string(id), workspaceID, "", "")
	_, err := decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandDeleteTicket)
	return err
}

type decisionEventResolutionRepository struct {
	hub magicevent.Hub
}

func (r decisionEventResolutionRepository) Create(ctx context.Context, item *decisionmodel.Resolution) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	command, err := createResolutionCommandFromModel(item)
	if err != nil {
		return err
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandCreateResolution, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandCreateResolution, "decision_resolution", string(item.ID), workspaceID, "", "")
	_, err = decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandCreateResolution)
	return err
}

func (r decisionEventResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandGetResolution, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.GetResolutionCommand{TicketID: ticketID})
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandGetResolution, "decision_ticket", string(ticketID), workspaceID, "", "")
	response, err := decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandGetResolution)
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
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandDeleteResolution, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.DeleteResolutionCommand{TicketID: ticketID})
	bindGovernanceCommandLane(event, decisionevents.TopicDecisionCommandDeleteResolution, "decision_ticket", string(ticketID), workspaceID, "", "")
	_, err := decisionCommandResponse(r.hub.Send(event), decisionevents.TopicDecisionCommandDeleteResolution)
	return err
}

type approvalEventRepository struct {
	hub magicevent.Hub
}

func (r approvalEventRepository) Create(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	command, err := createApprovalCommandFromModel(item)
	if err != nil {
		return err
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandCreate, ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindGovernanceCommandLane(event, approvalevents.TopicApprovalCommandCreate, "approval", string(item.ID), item.WorkspaceID, item.RunID, item.StepID)
	_, err = approvalCommandResponse(r.hub.Send(event), approvalevents.TopicApprovalCommandCreate)
	return err
}

func (r approvalEventRepository) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandGet, ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.GetApprovalCommand{ApprovalID: id})
	bindGovernanceCommandLane(event, approvalevents.TopicApprovalCommandGet, "approval", string(id), workspaceID, "", "")
	response, err := approvalCommandResponse(r.hub.Send(event), approvalevents.TopicApprovalCommandGet)
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
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandList, ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListApprovalsCommand{})
	bindGovernanceCommandLane(event, approvalevents.TopicApprovalCommandList, "approval", "all", workspaceID, "", "")
	response, err := approvalCommandResponse(r.hub.Send(event), approvalevents.TopicApprovalCommandList)
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func (r approvalEventRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*approvalmodel.Approval, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandListByRun, ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListApprovalsByRunCommand{RunID: runID})
	bindGovernanceCommandLane(event, approvalevents.TopicApprovalCommandListByRun, "run", string(runID), workspaceID, runID, "")
	response, err := approvalCommandResponse(r.hub.Send(event), approvalevents.TopicApprovalCommandListByRun)
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func (r approvalEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandListByWorkspace, ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListApprovalsByWorkspaceCommand{WorkspaceID: workspaceID})
	bindGovernanceCommandLane(event, approvalevents.TopicApprovalCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := approvalCommandResponse(r.hub.Send(event), approvalevents.TopicApprovalCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func (r approvalEventRepository) Update(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	command, err := updateApprovalCommandFromModel(item)
	if err != nil {
		return err
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandUpdate, ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindGovernanceCommandLane(event, approvalevents.TopicApprovalCommandUpdate, "approval", string(item.ID), item.WorkspaceID, item.RunID, item.StepID)
	_, err = approvalCommandResponse(r.hub.Send(event), approvalevents.TopicApprovalCommandUpdate)
	return err
}

func (r approvalEventRepository) Delete(ctx context.Context, id common.ApprovalID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandDelete, ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.DeleteApprovalCommand{ApprovalID: id})
	bindGovernanceCommandLane(event, approvalevents.TopicApprovalCommandDelete, "approval", string(id), workspaceID, "", "")
	_, err := approvalCommandResponse(r.hub.Send(event), approvalevents.TopicApprovalCommandDelete)
	return err
}

func createTicketCommandFromModel(item *decisionmodel.Ticket) (decisionevents.CreateTicketCommand, error) {
	if item == nil {
		return decisionevents.CreateTicketCommand{}, common.ErrInvalidArgument
	}
	return decisionevents.CreateTicketCommand{
		TicketID:               item.ID,
		WorkspaceID:            item.WorkspaceID,
		RunID:                  item.RunID,
		StepID:                 item.StepID,
		Reason:                 item.Reason,
		DecisionKind:           item.DecisionKind,
		Question:               item.Question,
		Context:                item.Context.Clone().Fields,
		AutoGovernance:         item.AutoGovernance.Clone().Fields,
		FailureClass:           item.FailureClass,
		FailureSignature:       item.FailureSignature,
		AllowedRecoveryActions: append([]string(nil), item.AllowedRecoveryActions...),
	}, nil
}

func updateTicketCommandFromModel(item *decisionmodel.Ticket) (decisionevents.UpdateTicketCommand, error) {
	if item == nil {
		return decisionevents.UpdateTicketCommand{}, common.ErrInvalidArgument
	}
	return decisionevents.UpdateTicketCommand{
		TicketID:               item.ID,
		WorkspaceID:            item.WorkspaceID,
		RunID:                  item.RunID,
		StepID:                 item.StepID,
		Reason:                 item.Reason,
		DecisionKind:           item.DecisionKind,
		Question:               item.Question,
		Context:                item.Context.Clone().Fields,
		State:                  item.State,
		AutoGovernance:         item.AutoGovernance.Clone().Fields,
		FailureClass:           item.FailureClass,
		FailureSignature:       item.FailureSignature,
		AllowedRecoveryActions: append([]string(nil), item.AllowedRecoveryActions...),
		ResolvedAt:             item.ResolvedAt,
		ResolvedBy:             item.ResolvedBy,
		CreatedAt:              item.CreatedAt,
		UpdatedAt:              item.UpdatedAt,
	}, nil
}

func createResolutionCommandFromModel(item *decisionmodel.Resolution) (decisionevents.CreateResolutionCommand, error) {
	if item == nil {
		return decisionevents.CreateResolutionCommand{}, common.ErrInvalidArgument
	}
	return decisionevents.CreateResolutionCommand{
		ResolutionID:       item.ID,
		TicketID:           item.TicketID,
		Type:               item.Type,
		Payload:            item.Payload.Clone().Fields,
		ContinuationAction: item.ContinuationAction,
		ResumeRunID:        item.ResumeRunID,
		SuccessorRunID:     item.SuccessorRunID,
	}, nil
}

func createApprovalCommandFromModel(item *approvalmodel.Approval) (approvalevents.CreateApprovalCommand, error) {
	if item == nil {
		return approvalevents.CreateApprovalCommand{}, common.ErrInvalidArgument
	}
	return approvalevents.CreateApprovalCommand{
		ApprovalID:     item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		Action:         item.Action,
		Context:        item.Context.Clone().Fields,
		AutoGovernance: item.AutoGovernance.Clone().Fields,
	}, nil
}

func updateApprovalCommandFromModel(item *approvalmodel.Approval) (approvalevents.UpdateApprovalCommand, error) {
	if item == nil {
		return approvalevents.UpdateApprovalCommand{}, common.ErrInvalidArgument
	}
	return approvalevents.UpdateApprovalCommand{
		ApprovalID:     item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		Action:         item.Action,
		Context:        item.Context.Clone().Fields,
		State:          item.State,
		AutoGovernance: item.AutoGovernance.Clone().Fields,
		ResolvedAt:     item.ResolvedAt,
		ResolvedBy:     item.ResolvedBy,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}, nil
}
