package workspaceports

import (
	"context"
	"errors"

	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func purgeWorkspaceResultsAndArtifacts(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	artifactResponse, err := ownerreadports.PublishCommandWithResponse(ctx, hub, resultevents.TopicArtifactCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID},
	})
	var artifacts []*resultmodel.Artifact
	if err != nil {
		if !errors.Is(err, common.ErrNotFound) {
			return err
		}
	} else {
		var ok bool
		artifacts, ok = artifactResponse.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
		if !ok {
			return common.ErrInvalidArgument
		}
	}
	for _, item := range artifacts {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := ownercommandports.Publish(ctx, hub, resultevents.TopicArtifactCommandDelete, "artifact", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyCommand: resultcontracts.DeleteArtifactCommand{ArtifactID: item.ID},
		}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	resultResponse, err := ownerreadports.PublishCommandWithResponse(ctx, hub, resultevents.TopicResultCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID},
	})
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	results, ok := resultResponse.Payload[events.PayloadKeyItems].([]*resultmodel.Result)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := ownercommandports.Publish(ctx, hub, resultevents.TopicResultCommandDelete, "result", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyCommand: resultcontracts.DeleteResultCommand{ResultID: item.ID},
		}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
