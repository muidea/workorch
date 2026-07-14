package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadRuntimeCheckpointByOwnerCommand(ctx context.Context, hub magicevent.Hub, checkpointID common.CheckpointID) (*runmodel.RuntimeCheckpoint, error) {
	if hub == nil || checkpointID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGetRuntimeCheckpoint, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), commandCtx, runevents.GetRuntimeCheckpointCommand{
		WorkspaceID:  workspaceID,
		CheckpointID: checkpointID,
	})
	ownerevent.BindLane(event, runevents.TopicRunCommandGetRuntimeCheckpoint, "runtime_checkpoint", string(checkpointID), workspaceID, "", "")
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandGetRuntimeCheckpoint)
	if err != nil {
		return nil, err
	}
	if response.Checkpoint == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Checkpoint, nil
}

func ListRuntimeCheckpointsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandListCheckpointsByRun, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), commandCtx, runevents.ListRuntimeCheckpointsByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
	ownerevent.BindLane(event, runevents.TopicRunCommandListCheckpointsByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandListCheckpointsByRun)
	if err != nil {
		return nil, err
	}
	return response.Checkpoints, nil
}
