package ownerreadports

import (
	"context"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListToolCallsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.ToolCall, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicOrchestratorCommandListToolCallsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*runmodel.ToolCall)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func ListPatchesByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.Patch, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicOrchestratorCommandListPatchesByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*runmodel.Patch)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func ListValidationResultsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.ValidationResult, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicOrchestratorCommandListValidationResultsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*runmodel.ValidationResult)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
