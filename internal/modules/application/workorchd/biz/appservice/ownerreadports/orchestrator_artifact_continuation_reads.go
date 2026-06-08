package ownerreadports

import (
	"context"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ListArtifactContinuationsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*orchestratormodel.ArtifactContinuation)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
