package governance

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
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
)

type eventTicketReader struct {
	hub magicevent.Hub
}

func (r eventTicketReader) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := sendOwnerMagicCommand[decisionevents.GetTicketCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandGetTicket, decisionevents.CommandDestination, "decision_ticket", string(id), workspaceID, "", "", decisionevents.GetTicketCommand{WorkspaceID: workspaceID, TicketID: id})
	if err != nil {
		return nil, err
	}
	if response.Ticket == nil {
		return nil, common.ErrNotFound
	}
	return response.Ticket, nil
}

type eventResolutionReader struct {
	hub magicevent.Hub
}

func (r eventResolutionReader) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := sendOwnerMagicCommand[decisionevents.GetResolutionCommand, decisionevents.DecisionCommandResult](ctx, r.hub, decisionevents.TopicDecisionCommandGetResolution, decisionevents.CommandDestination, "decision_ticket", string(ticketID), workspaceID, "", "", decisionevents.GetResolutionCommand{WorkspaceID: workspaceID, TicketID: ticketID})
	if err != nil {
		return nil, err
	}
	if response.Resolution == nil {
		return nil, common.ErrNotFound
	}
	return response.Resolution, nil
}

type eventApprovalReader struct {
	hub magicevent.Hub
}

func (r eventApprovalReader) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := sendOwnerMagicCommand[approvalevents.GetApprovalCommand, approvalevents.ApprovalCommandResult](ctx, r.hub, approvalevents.TopicApprovalCommandGet, approvalevents.CommandDestination, "approval", string(id), workspaceID, "", "", approvalevents.GetApprovalCommand{WorkspaceID: workspaceID, ApprovalID: id})
	if err != nil {
		return nil, err
	}
	if response.Approval == nil {
		return nil, common.ErrNotFound
	}
	return response.Approval, nil
}

type eventRunReader struct {
	hub magicevent.Hub
}

func (r eventRunReader) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := sendOwnerMagicCommand[runevents.GetRunCommand, runevents.RunCommandResult](ctx, r.hub, runevents.TopicRunCommandGet, runevents.CommandDestination, "run", string(id), workspaceID, id, "", runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	return response.Run, nil
}

func (r eventRunReader) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	response, err := sendOwnerMagicCommand[runevents.GetRunCommand, runevents.RunCommandResult](ctx, r.hub, runevents.TopicRunCommandGet, runevents.CommandDestination, "run", string(id), workspaceID, id, "", runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	return response.Run, nil
}

func (r eventRunReader) List(ctx context.Context) ([]*runmodel.Run, error) {
	response, err := sendOwnerMagicCommand[runevents.ListRunsCommand, runevents.RunCommandResult](ctx, r.hub, runevents.TopicRunCommandList, runevents.CommandDestination, "run", "all", "", "", "", runevents.ListRunsCommand{})
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

type eventWorkspaceReader struct {
	hub magicevent.Hub
}

func (r eventWorkspaceReader) GetGoal(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := sendOwnerMagicCommand[workspaceevents.GetGoalCommand, workspaceevents.WorkspaceCommandResult](ctx, r.hub, workspaceevents.TopicWorkspaceCommandGetGoal, workspaceevents.CommandDestination, "goal", string(id), workspaceID, "", "", workspaceevents.GetGoalCommand{WorkspaceID: workspaceID, GoalID: id})
	if err != nil {
		return nil, err
	}
	if response.Goal == nil {
		return nil, common.ErrNotFound
	}
	return response.Goal, nil
}

func (r eventWorkspaceReader) GetDoD(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := sendOwnerMagicCommand[workspaceevents.GetDoDCommand, workspaceevents.WorkspaceCommandResult](ctx, r.hub, workspaceevents.TopicWorkspaceCommandGetDoD, workspaceevents.CommandDestination, "dod", string(id), workspaceID, "", "", workspaceevents.GetDoDCommand{WorkspaceID: workspaceID, DoDID: id})
	if err != nil {
		return nil, err
	}
	if response.DoD == nil {
		return nil, common.ErrNotFound
	}
	return response.DoD, nil
}

type eventMemoryReader struct {
	hub magicevent.Hub
}

func (r eventMemoryReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	response, err := sendOwnerMagicCommand[memoryevents.ListMemoryByWorkspaceCommand, memoryevents.MemoryCommandResult](ctx, r.hub, memoryevents.TopicMemoryCommandListByWorkspace, memoryevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", "", memoryevents.ListMemoryByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}
