package runports

import (
	"context"

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
	return ownercommandports.Publish(ctx, hub, events.TopicRunCommandUpdate, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
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
	return ownercommandports.Publish(ctx, hub, events.TopicRunCommandFail, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyCommand: runcontracts.FailRunCommand{RunID: item.ID, Reason: reason},
	})
}
