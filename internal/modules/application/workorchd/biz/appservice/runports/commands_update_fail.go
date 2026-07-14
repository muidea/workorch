package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func UpdateRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandUpdate, "application/workorchd/appservice", runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.UpdateRunCommand{
		RunID:                  item.ID,
		WorkspaceID:            item.WorkspaceID,
		GoalID:                 item.GoalID,
		DoDID:                  item.DoDID,
		DefinitionRevision:     item.DefinitionRevision,
		InitialStepKind:        item.InitialStepKind,
		RuntimeBootstrap:       item.RuntimeBootstrap,
		RuntimeContractVersion: item.RuntimeContractVersion,
		RuntimeState:           item.RuntimeState,
		State:                  item.State,
		Priority:               item.Priority,
		BlockingTicketID:       item.BlockingTicketID,
		BlockingApprovalID:     item.BlockingApprovalID,
		FailureReason:          item.FailureReason,
	})
	ownerevent.BindLane(event, runevents.TopicRunCommandUpdate, "run", string(item.ID), item.WorkspaceID, item.ID, "")
	_, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandUpdate)
	return err
}

func FailRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run, reason string) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandFailRun, "application/workorchd/appservice", orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.FailRunCommand{
		RunID:  item.ID,
		Reason: reason,
	})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandFailRun, "run", string(item.ID), item.WorkspaceID, item.ID, "")
	_, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandFailRun)
	return err
}

func CancelRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandCancelRun, "application/workorchd/appservice", orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.CancelRunCommand{
		RunID: item.ID,
	})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandCancelRun, "run", string(item.ID), item.WorkspaceID, item.ID, "")
	_, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandCancelRun)
	return err
}
