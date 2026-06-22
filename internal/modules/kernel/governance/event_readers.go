package governance

import (
	"context"
	"fmt"

	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type eventTicketReader struct {
	hub events.Hub
}

func (r eventTicketReader) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	response, err := publishGovernanceReadCommand(ctx, r.hub, decisionevents.TopicDecisionCommandGetTicket, "decision_ticket", string(id), "", "", map[string]any{
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

type eventResolutionReader struct {
	hub events.Hub
}

func (r eventResolutionReader) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	response, err := publishGovernanceReadCommand(ctx, r.hub, decisionevents.TopicDecisionCommandGetResolution, "decision_ticket", string(ticketID), "", "", map[string]any{
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

type eventApprovalReader struct {
	hub events.Hub
}

func (r eventApprovalReader) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	response, err := publishGovernanceReadCommand(ctx, r.hub, approvalevents.TopicApprovalCommandGet, "approval", string(id), "", "", map[string]any{
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

type eventRunReader struct {
	hub events.Hub
}

func (r eventRunReader) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	response, err := publishGovernanceReadCommand(ctx, r.hub, runevents.TopicRunCommandGet, "run", string(id), "", id, map[string]any{
		events.PayloadKeyRunID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r eventRunReader) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	response, err := publishGovernanceReadCommand(ctx, r.hub, runevents.TopicRunCommandGet, "run", string(id), workspaceID, id, map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
		events.PayloadKeyRunID:       string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r eventRunReader) List(ctx context.Context) ([]*runmodel.Run, error) {
	response, err := publishGovernanceReadCommand(ctx, r.hub, runevents.TopicRunCommandList, "run", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

type eventWorkspaceReader struct {
	hub events.Hub
}

func (r eventWorkspaceReader) GetGoal(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := publishGovernanceReadCommand(ctx, r.hub, workspaceevents.TopicWorkspaceCommandGetGoal, "goal", string(id), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
		events.PayloadKeyGoalID:      string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyGoal].(*workspacemodel.Goal)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r eventWorkspaceReader) GetDoD(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := publishGovernanceReadCommand(ctx, r.hub, workspaceevents.TopicWorkspaceCommandGetDoD, "dod", string(id), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
		events.PayloadKeyDoDID:       string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyDoD].(*workspacemodel.DoD)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

type eventMemoryReader struct {
	hub events.Hub
}

func (r eventMemoryReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	response, err := publishGovernanceReadCommand(ctx, r.hub, memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func publishGovernanceReadCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) (events.Envelope, error) {
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
		Payload:      payload,
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		return response, nil
	}
	if err != nil {
		return events.Envelope{}, err
	}
	return events.Envelope{}, fmt.Errorf("governance read command %s returned no response", topic)
}
