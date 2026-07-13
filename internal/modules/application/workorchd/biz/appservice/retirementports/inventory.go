package retirementports

import (
	"context"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/retirementcore"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

const retirementInventorySource = "workorchd/runtime-v1-retirement"

func LoadRuntimeV1Inventory(ctx context.Context, hub magicevent.Hub) (retirementcore.RuntimeV1Inventory, error) {
	if hub == nil {
		return retirementcore.RuntimeV1Inventory{}, common.ErrInvalidArgument
	}
	runs, err := sendRetirementCommand[runevents.ListRunsCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandList, runevents.CommandDestination, "run", runevents.ListRunsCommand{})
	if err != nil {
		return retirementcore.RuntimeV1Inventory{}, err
	}
	workspaces, err := sendRetirementCommand[workspaceevents.ListWorkspacesCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandList, workspaceevents.CommandDestination, "workspace", workspaceevents.ListWorkspacesCommand{})
	if err != nil {
		return retirementcore.RuntimeV1Inventory{}, err
	}
	tickets, err := sendRetirementCommand[decisionevents.ListTicketsCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandListTickets, decisionevents.CommandDestination, "decision_ticket", decisionevents.ListTicketsCommand{})
	if err != nil {
		return retirementcore.RuntimeV1Inventory{}, err
	}
	approvals, err := sendRetirementCommand[approvalevents.ListApprovalsCommand, approvalevents.ApprovalCommandResult](ctx, hub, approvalevents.TopicApprovalCommandList, approvalevents.CommandDestination, "approval", approvalevents.ListApprovalsCommand{})
	if err != nil {
		return retirementcore.RuntimeV1Inventory{}, err
	}
	return retirementcore.RuntimeV1Inventory{
		Runs:       runs.Runs,
		Workspaces: workspaces.Workspaces,
		Tickets:    tickets.Tickets,
		Approvals:  approvals.Approvals,
	}, nil
}

func sendRetirementCommand[Command any, Result any](ctx context.Context, hub magicevent.Hub, topic string, destination string, subjectType string, command Command) (Result, error) {
	var zero Result
	if hub == nil || topic == "" || destination == "" {
		return zero, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(topic, retirementInventorySource, destination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(topic, workorchevents.EventHubLaneScope{SubjectType: subjectType, SubjectID: "runtime-v1-retirement"}))
	response := hub.Send(event)
	if response == nil {
		return zero, fmt.Errorf("%w: retirement inventory command %s returned no response", common.ErrConflict, topic)
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return zero, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return zero, err
	}
	result, ok := data.(Result)
	if !ok {
		return zero, fmt.Errorf("%w: retirement inventory command %s returned %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}
