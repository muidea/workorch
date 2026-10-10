package governance

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type eventTicketReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventTicketReader) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.send == nil || workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandGetTicket, governancecommon.ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.GetTicketCommand{WorkspaceID: workspaceID, TicketID: id})
	bindOwnerLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandGetTicket, "decision_ticket", string(id), workspaceID, "", "")
	response, err := decisionResponse[decisionevents.GetTicketResult](r.send(event), decisionevents.TopicDecisionCommandGetTicket)
	if err != nil {
		return nil, err
	}
	if response.Ticket == nil {
		return nil, common.ErrNotFound
	}
	if response.Ticket.ID != id || response.Ticket.WorkspaceID != workspaceID {
		return nil, ownerResultIdentityError(decisionevents.TopicDecisionCommandGetTicket, fmt.Sprintf("ticket %s/%s", response.Ticket.WorkspaceID, response.Ticket.ID))
	}
	return decisionmodel.CloneTicket(response.Ticket), nil
}

type eventApprovalReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventApprovalReader) GetScoped(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) (*approvalmodel.Approval, error) {
	if r.send == nil || workspaceID == "" || runID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandGet, governancecommon.ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.GetApprovalCommand{WorkspaceID: workspaceID, RunID: runID, ApprovalID: id})
	bindOwnerLane(event, approvalevents.Contracts(), approvalevents.TopicApprovalCommandGet, "approval", string(id), workspaceID, runID, "")
	response, err := approvalResponse[approvalevents.GetApprovalResult](r.send(event), approvalevents.TopicApprovalCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Approval == nil {
		return nil, common.ErrNotFound
	}
	if response.Approval.ID != id || response.Approval.WorkspaceID != workspaceID || response.Approval.RunID != runID {
		return nil, ownerResultIdentityError(approvalevents.TopicApprovalCommandGet, fmt.Sprintf("approval %s/%s/%s", response.Approval.WorkspaceID, response.Approval.RunID, response.Approval.ID))
	}
	return approvalmodel.CloneApproval(response.Approval), nil
}

type eventRunReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventRunReader) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.send == nil || workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, governancecommon.ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	bindOwnerLane(event, runevents.Contracts(), runevents.TopicRunCommandGet, "run", string(id), workspaceID, id, "")
	response, err := runResponse[runevents.GetRunResult](r.send(event), runevents.TopicRunCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	if response.Run.ID != id || response.Run.WorkspaceID != workspaceID {
		return nil, ownerResultIdentityError(runevents.TopicRunCommandGet, fmt.Sprintf("run %s/%s", response.Run.WorkspaceID, response.Run.ID))
	}
	return runmodel.CloneRun(response.Run), nil
}

func (r eventRunReader) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	if r.send == nil || workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, governancecommon.ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	bindOwnerLane(event, runevents.Contracts(), runevents.TopicRunCommandGet, "run", string(id), workspaceID, id, "")
	response, err := runResponse[runevents.GetRunResult](r.send(event), runevents.TopicRunCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	if response.Run.ID != id || response.Run.WorkspaceID != workspaceID {
		return nil, ownerResultIdentityError(runevents.TopicRunCommandGet, fmt.Sprintf("run %s/%s", response.Run.WorkspaceID, response.Run.ID))
	}
	return runmodel.CloneRun(response.Run), nil
}

func (r eventRunReader) List(ctx context.Context) ([]*runmodel.Run, error) {
	if r.send == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandList, governancecommon.ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.ListRunsCommand{})
	bindOwnerLane(event, runevents.Contracts(), runevents.TopicRunCommandList, "run", "all", workspaceID, "", "")
	response, err := runResponse[runevents.ListRunsResult](r.send(event), runevents.TopicRunCommandList)
	if err != nil {
		return nil, err
	}
	for idx, item := range response.Runs {
		if item == nil || item.ID == "" || item.WorkspaceID == "" {
			return nil, ownerResultIdentityError(runevents.TopicRunCommandList, fmt.Sprintf("run item %d", idx))
		}
	}
	return runmodel.CloneRuns(response.Runs), nil
}

type eventWorkspaceReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventWorkspaceReader) GetGoal(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.send == nil || workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	command := workspaceevents.GetGoalCommand{WorkspaceID: workspaceID, GoalID: id}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetGoal, governancecommon.ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandGetGoal, "goal", string(id), workspaceID, "", "")
	response, err := workspaceResponse[workspaceevents.GoalResult](r.send(event), workspaceevents.TopicWorkspaceCommandGetGoal)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspacemodel.CloneGoal(response.Goal), nil
}

func (r eventWorkspaceReader) GetDoD(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.send == nil || workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	command := workspaceevents.GetDoDCommand{WorkspaceID: workspaceID, DoDID: id}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetDoD, governancecommon.ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandGetDoD, "dod", string(id), workspaceID, "", "")
	response, err := workspaceResponse[workspaceevents.DoDResult](r.send(event), workspaceevents.TopicWorkspaceCommandGetDoD)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspacemodel.CloneDoD(response.DoD), nil
}

type eventMemoryReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r eventMemoryReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if r.send == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandListByWorkspace, governancecommon.ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryByWorkspaceCommand{WorkspaceID: workspaceID, Limit: 200})
	bindOwnerLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := memoryResponse[memoryevents.ListMemoryByWorkspaceResult](r.send(event), memoryevents.TopicMemoryCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	for idx, item := range response.Items {
		if item == nil || item.ID == "" || item.WorkspaceID != workspaceID {
			return nil, ownerResultIdentityError(memoryevents.TopicMemoryCommandListByWorkspace, fmt.Sprintf("memory item %d", idx))
		}
	}
	return memorymodel.CloneMemoryItems(response.Items), nil
}

func ownerResultIdentityError(topic string, identity string) error {
	return fmt.Errorf("%w: event %s returned mismatched owner identity %s", common.ErrConflict, topic, identity)
}
