package orchestrator

import (
	"context"
	"fmt"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
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
	_, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandCreateTicket, "decision_ticket", string(item.ID), item.WorkspaceID, item.RunID, item.StepID, map[string]any{
		events.PayloadKeyTicket: item,
	})
	return err
}

func (r decisionEventTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	response, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandGetTicket, "decision_ticket", string(id), "", "", "", map[string]any{
		events.PayloadKeyBlockingTicketID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyTicket].(*decisionmodel.Ticket)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r decisionEventTicketRepository) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	response, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandListTickets, "decision_ticket", "all", "", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r decisionEventTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	response, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandListTicketsByRun, "run", string(runID), "", runID, "", map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r decisionEventTicketRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	response, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandListTicketsByWS, "workspace", string(workspaceID), workspaceID, "", "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r decisionEventTicketRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandUpdateTicket, "decision_ticket", string(item.ID), item.WorkspaceID, item.RunID, item.StepID, map[string]any{
		events.PayloadKeyTicket: item,
	})
	return err
}

func (r decisionEventTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	_, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandDeleteTicket, "decision_ticket", string(id), "", "", "", map[string]any{
		events.PayloadKeyBlockingTicketID: string(id),
	})
	return err
}

type decisionEventResolutionRepository struct {
	hub events.Hub
}

func (r decisionEventResolutionRepository) Create(ctx context.Context, item *decisionmodel.Resolution) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandCreateResolution, "decision_resolution", string(item.ID), "", "", "", map[string]any{
		events.PayloadKeyResolution: item,
	})
	return err
}

func (r decisionEventResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	response, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandGetResolution, "decision_ticket", string(ticketID), "", "", "", map[string]any{
		events.PayloadKeyBlockingTicketID: string(ticketID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyResolution].(*decisionmodel.Resolution)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r decisionEventResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	_, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicDecisionCommandDeleteResolution, "decision_ticket", string(ticketID), "", "", "", map[string]any{
		events.PayloadKeyBlockingTicketID: string(ticketID),
	})
	return err
}

type approvalEventRepository struct {
	hub events.Hub
}

func (r approvalEventRepository) Create(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicApprovalCommandCreate, "approval", string(item.ID), item.WorkspaceID, item.RunID, item.StepID, map[string]any{
		events.PayloadKeyApproval: item,
	})
	return err
}

func (r approvalEventRepository) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	response, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicApprovalCommandGet, "approval", string(id), "", "", "", map[string]any{
		events.PayloadKeyApprovalID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyApproval].(*approvalmodel.Approval)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r approvalEventRepository) List(ctx context.Context) ([]*approvalmodel.Approval, error) {
	response, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicApprovalCommandList, "approval", "all", "", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r approvalEventRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*approvalmodel.Approval, error) {
	response, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicApprovalCommandListByRun, "run", string(runID), "", runID, "", map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r approvalEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	response, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicApprovalCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r approvalEventRepository) Update(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicApprovalCommandUpdate, "approval", string(item.ID), item.WorkspaceID, item.RunID, item.StepID, map[string]any{
		events.PayloadKeyApproval: item,
	})
	return err
}

func (r approvalEventRepository) Delete(ctx context.Context, id common.ApprovalID) error {
	_, err := publishGovernanceEventCommand(ctx, r.hub, events.TopicApprovalCommandDelete, "approval", string(id), "", "", "", map[string]any{
		events.PayloadKeyApprovalID: string(id),
	})
	return err
}

func publishGovernanceEventCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, payload map[string]any) (events.Envelope, error) {
	if hub == nil || topic == "" {
		return events.Envelope{}, common.ErrInvalidArgument
	}
	err := hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: ModuleID,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(stepID),
		Payload:      payload,
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		return response, nil
	}
	if err != nil {
		return events.Envelope{}, err
	}
	return events.Envelope{}, fmt.Errorf("governance event command %s returned no response", topic)
}
