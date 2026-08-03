package workspaceports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func purgeWorkspaceResultsAndArtifacts(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
	artifactEvent := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByWorkspace, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(artifactEvent, resultevents.Contracts(), resultevents.TopicArtifactCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	artifactResponse, err := ownerevent.Result(hub.Send(artifactEvent), resultevents.TopicArtifactCommandListByWorkspace)
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
		event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandDelete, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.DeleteArtifactCommand{ArtifactID: item.ID})
		ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandDelete, "artifact", string(item.ID), workspaceID, item.RunID, "")
		if _, err := ownerevent.Result(hub.Send(event), resultevents.TopicArtifactCommandDelete); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	resultEvent := magicevent.NewEventWithContext(resultevents.TopicResultCommandListByWorkspace, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(resultEvent, resultevents.Contracts(), resultevents.TopicResultCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	resultResponse, err := ownerevent.Result(hub.Send(resultEvent), resultevents.TopicResultCommandListByWorkspace)
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
		event := magicevent.NewEventWithContext(resultevents.TopicResultCommandDelete, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.DeleteResultCommand{ResultID: item.ID})
		ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicResultCommandDelete, "result", string(item.ID), workspaceID, item.RunID, "")
		if _, err := ownerevent.Result(hub.Send(event), resultevents.TopicResultCommandDelete); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
