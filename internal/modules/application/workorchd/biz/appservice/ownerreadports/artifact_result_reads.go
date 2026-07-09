package ownerreadports

import (
	"context"

	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListArtifactsByResultOwnerCommand(ctx context.Context, hub events.Hub, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	if hub == nil || resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[resultevents.ListArtifactsByResultCommand, resultevents.ResultCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, resultevents.TopicArtifactCommandListByResult, resultevents.CommandDestination, "result", string(resultID), resultevents.ListArtifactsByResultCommand{ResultID: resultID})
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}
