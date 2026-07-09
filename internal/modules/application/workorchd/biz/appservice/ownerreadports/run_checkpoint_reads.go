package ownerreadports

import (
	"context"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadRuntimeCheckpointByOwnerCommand(ctx context.Context, hub events.Hub, checkpointID common.CheckpointID) (*runmodel.RuntimeCheckpoint, error) {
	if hub == nil || checkpointID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[runevents.GetRuntimeCheckpointCommand, runevents.RunCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, runevents.TopicRunCommandGetRuntimeCheckpoint, runevents.CommandDestination, "runtime_checkpoint", string(checkpointID), runevents.GetRuntimeCheckpointCommand{
		WorkspaceID:  workspaceID,
		CheckpointID: checkpointID,
	})
	if err != nil {
		return nil, err
	}
	if response.Checkpoint == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Checkpoint, nil
}

func ListRuntimeCheckpointsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[runevents.ListRuntimeCheckpointsByRunCommand, runevents.RunCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, runevents.TopicRunCommandListCheckpointsByRun, runevents.CommandDestination, "run", string(runID), runevents.ListRuntimeCheckpointsByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
	if err != nil {
		return nil, err
	}
	return response.Checkpoints, nil
}
