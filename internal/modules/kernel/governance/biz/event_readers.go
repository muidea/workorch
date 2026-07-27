package governance

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

type eventTicketReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventTicketReader) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.send == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandGetTicket, governancecommon.ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.GetTicketCommand{WorkspaceID: workspaceID, TicketID: id})
	bindOwnerLane(event, decisionevents.TopicDecisionCommandGetTicket, "decision_ticket", string(id), workspaceID, "", "")
	response, err := decisionResponse(r.send(event), decisionevents.TopicDecisionCommandGetTicket)
	if err != nil {
		return nil, err
	}
	if response.Ticket == nil {
		return nil, common.ErrNotFound
	}
	return response.Ticket, nil
}

type eventApprovalReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventApprovalReader) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.send == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandGet, governancecommon.ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.GetApprovalCommand{WorkspaceID: workspaceID, ApprovalID: id})
	bindOwnerLane(event, approvalevents.TopicApprovalCommandGet, "approval", string(id), workspaceID, "", "")
	response, err := approvalResponse(r.send(event), approvalevents.TopicApprovalCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Approval == nil {
		return nil, common.ErrNotFound
	}
	return response.Approval, nil
}

type eventRunReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventRunReader) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.send == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, governancecommon.ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	bindOwnerLane(event, runevents.TopicRunCommandGet, "run", string(id), workspaceID, id, "")
	response, err := runResponse(r.send(event), runevents.TopicRunCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	return response.Run, nil
}

func (r eventRunReader) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	if r.send == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, governancecommon.ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	bindOwnerLane(event, runevents.TopicRunCommandGet, "run", string(id), workspaceID, id, "")
	response, err := runResponse(r.send(event), runevents.TopicRunCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	return response.Run, nil
}

func (r eventRunReader) List(ctx context.Context) ([]*runmodel.Run, error) {
	if r.send == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandList, governancecommon.ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.ListRunsCommand{})
	bindOwnerLane(event, runevents.TopicRunCommandList, "run", "all", workspaceID, "", "")
	response, err := runResponse(r.send(event), runevents.TopicRunCommandList)
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

type eventWorkspaceReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventWorkspaceReader) GetGoal(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.send == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetGoal, governancecommon.ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetGoalCommand{WorkspaceID: workspaceID, GoalID: id})
	bindOwnerLane(event, workspaceevents.TopicWorkspaceCommandGetGoal, "goal", string(id), workspaceID, "", "")
	response, err := workspaceResponse(r.send(event), workspaceevents.TopicWorkspaceCommandGetGoal)
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
	if r.send == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetDoD, governancecommon.ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetDoDCommand{WorkspaceID: workspaceID, DoDID: id})
	bindOwnerLane(event, workspaceevents.TopicWorkspaceCommandGetDoD, "dod", string(id), workspaceID, "", "")
	response, err := workspaceResponse(r.send(event), workspaceevents.TopicWorkspaceCommandGetDoD)
	if err != nil {
		return nil, err
	}
	if response.DoD == nil {
		return nil, common.ErrNotFound
	}
	return response.DoD, nil
}

type eventMemoryReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventMemoryReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if r.send == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandListByWorkspace, governancecommon.ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryByWorkspaceCommand{WorkspaceID: workspaceID})
	bindOwnerLane(event, memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := memoryResponse(r.send(event), memoryevents.TopicMemoryCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}
