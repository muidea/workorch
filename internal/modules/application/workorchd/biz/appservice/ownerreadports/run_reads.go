package ownerreadports

import (
	"context"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadRunByOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) (*runmodel.Run, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyRunID: string(runID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicRunCommandGet, "run", string(runID), workspaceID, runID, payload)
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ListRunsByOwnerCommand(ctx context.Context, hub events.Hub) ([]*runmodel.Run, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicRunCommandList, "run", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func ListRunsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
