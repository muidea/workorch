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
	response, err := PublishCommandWithResponse(ctx, hub, runevents.TopicRunCommandGetRuntimeCheckpoint, "runtime_checkpoint", string(checkpointID), "", "", map[string]any{
		events.PayloadKeyCheckpointID: string(checkpointID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyCheckpoint].(*runmodel.RuntimeCheckpoint)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ListRuntimeCheckpointsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, runevents.TopicRunCommandListCheckpointsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyCheckpoints].([]*runmodel.RuntimeCheckpoint)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
