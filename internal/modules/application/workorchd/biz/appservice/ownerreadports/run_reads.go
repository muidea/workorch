package ownerreadports

import (
	"context"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadRunByOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) (*runmodel.Run, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID != "" {
		return ReadRunByWorkspaceOwnerCommand(ctx, hub, workspaceID, runID)
	}
	payload := map[string]any{
		events.PayloadKeyRunID: string(runID),
	}
	response, err := PublishCommandWithResponse(ctx, hub, runevents.TopicRunCommandGet, "run", string(runID), workspaceID, runID, payload)
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ReadRunByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, runID common.RunID) (*runmodel.Run, error) {
	if hub == nil || workspaceID == "" || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, runevents.TopicRunCommandGet, "run", string(runID), workspaceID, runID, map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
		events.PayloadKeyRunID:       string(runID),
	})
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
	response, err := PublishCommandWithResponse(ctx, hub, runevents.TopicRunCommandList, "run", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func ListRecoverableRuntimeGapRunCandidatesByOwnerCommand(ctx context.Context, hub events.Hub) ([]*runmodel.Run, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, runevents.TopicRunCommandList, "run", "runtime-gap-candidates", "", "", map[string]any{
		events.PayloadKeyStates:  []string{string(common.RunStateRunning)},
		events.PayloadKeyCompact: true,
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

func ListRunsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, runevents.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
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
