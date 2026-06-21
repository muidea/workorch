package runports

import (
	"context"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func UpdateRun(ctx context.Context, hub events.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return ownercommandports.Publish(ctx, hub, runevents.TopicRunCommandUpdate, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyCommand: runcontracts.UpdateRunCommand{
			RunID:              item.ID,
			WorkspaceID:        item.WorkspaceID,
			GoalID:             item.GoalID,
			DoDID:              item.DoDID,
			DefinitionRevision: item.DefinitionRevision,
			InitialStepKind:    item.InitialStepKind,
			ExecutionContext:   cloneMap(item.ExecutionContext),
			State:              item.State,
			Priority:           item.Priority,
			BlockingTicketID:   item.BlockingTicketID,
			BlockingApprovalID: item.BlockingApprovalID,
			FailureReason:      item.FailureReason,
		},
	})
}

func FailRun(ctx context.Context, hub events.Hub, item *runmodel.Run, reason string) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return ownercommandports.Publish(ctx, hub, orchestratorevents.TopicOrchestratorCommandFailRun, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyRunID:         string(item.ID),
		events.PayloadKeyFailureReason: reason,
	})
}

func CancelRun(ctx context.Context, hub events.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return ownercommandports.Publish(ctx, hub, orchestratorevents.TopicOrchestratorCommandCancelRun, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyRunID: string(item.ID),
	})
}
