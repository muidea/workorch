package ownerreadports

import (
	"context"

	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListArtifactsByResultOwnerCommand(ctx context.Context, hub events.Hub, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	if hub == nil || resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicArtifactCommandListByResult, "result", string(resultID), "", "", map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListArtifactsByResultCommand{ResultID: resultID},
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
