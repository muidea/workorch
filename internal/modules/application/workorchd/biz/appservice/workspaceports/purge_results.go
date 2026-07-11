package workspaceports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func purgeWorkspaceResultsAndArtifacts(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
	artifactResponse, err := sendWorkspaceMagicCommand[resultevents.ListArtifactsByWorkspaceCommand, resultevents.ResultCommandResult](ctx, hub, resultevents.TopicArtifactCommandListByWorkspace, resultevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", resultevents.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID})
	var artifacts []*resultmodel.Artifact
	if err != nil {
		if !errors.Is(err, common.ErrNotFound) {
			return err
		}
	} else {
		artifacts = artifactResponse.Artifacts
	}
	for _, item := range artifacts {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if _, err := sendWorkspaceMagicCommand[resultevents.DeleteArtifactCommand, resultevents.ResultCommandResult](ctx, hub, resultevents.TopicArtifactCommandDelete, resultevents.CommandDestination, "artifact", string(item.ID), workspaceID, item.RunID, resultevents.DeleteArtifactCommand{ArtifactID: item.ID}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	resultResponse, err := sendWorkspaceMagicCommand[resultevents.ListResultsByWorkspaceCommand, resultevents.ResultCommandResult](ctx, hub, resultevents.TopicResultCommandListByWorkspace, resultevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", resultevents.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	results := resultResponse.Results
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if _, err := sendWorkspaceMagicCommand[resultevents.DeleteResultCommand, resultevents.ResultCommandResult](ctx, hub, resultevents.TopicResultCommandDelete, resultevents.CommandDestination, "result", string(item.ID), workspaceID, item.RunID, resultevents.DeleteResultCommand{ResultID: item.ID}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
