package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func UpdateRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendRunMagicCommand[runevents.UpdateRunCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandUpdate, runevents.CommandDestination, "run", string(item.ID), item.WorkspaceID, item.ID, runevents.UpdateRunCommand{
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
	return err
}

func FailRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run, reason string) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendRunMagicCommand[orchestratorevents.FailRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandFailRun, orchestratorevents.CommandDestination, "run", string(item.ID), item.WorkspaceID, item.ID, orchestratorevents.FailRunCommand{
		RunID:  item.ID,
		Reason: reason,
	})
	return err
}

func CancelRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendRunMagicCommand[orchestratorevents.CancelRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandCancelRun, orchestratorevents.CommandDestination, "run", string(item.ID), item.WorkspaceID, item.ID, orchestratorevents.CancelRunCommand{
		RunID: item.ID,
	})
	return err
}
