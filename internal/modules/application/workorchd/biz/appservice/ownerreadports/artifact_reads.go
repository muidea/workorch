package ownerreadports

import (
	"context"

	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListArtifactsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicArtifactCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID},
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

func ListArtifactsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicArtifactCommandListByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListArtifactsByRunCommand{RunID: runID},
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
