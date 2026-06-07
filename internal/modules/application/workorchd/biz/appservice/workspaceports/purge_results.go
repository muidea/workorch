package workspaceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func purgeWorkspaceResultsAndArtifacts(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	artifactResponse, err := ownercommandports.PublishWithResponse(ctx, hub, events.TopicArtifactCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID},
	})
	if err != nil {
		return err
	}
	artifacts, ok := artifactResponse.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range artifacts {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := ownercommandports.Publish(ctx, hub, events.TopicArtifactCommandDelete, "artifact", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyCommand: resultcontracts.DeleteArtifactCommand{ArtifactID: item.ID},
		}); err != nil {
			return err
		}
	}
	resultResponse, err := ownercommandports.PublishWithResponse(ctx, hub, events.TopicResultCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID},
	})
	if err != nil {
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
		if err := ownercommandports.Publish(ctx, hub, events.TopicResultCommandDelete, "result", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyCommand: resultcontracts.DeleteResultCommand{ResultID: item.ID},
		}); err != nil {
			return err
		}
	}
	return nil
}
