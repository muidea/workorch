package ownerreadports

import (
	"context"

	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"

	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListArtifactsByResultOwnerCommand(ctx context.Context, hub events.Hub, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	if hub == nil || resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := PublishCommandWithResponse(ctx, hub, resultevents.TopicArtifactCommandListByResult, "result", string(resultID), workspaceID, "", payloadWithWorkspaceScope(map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListArtifactsByResultCommand{ResultID: resultID},
	}, workspaceID))
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
