package workspaceports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func purgeWorkspaceResultsAndArtifacts(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
	boundCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return err
	}
	results, err := ownerreadports.ListResultsByWorkspaceOwnerCommand(boundCtx, hub, workspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			results = nil
		} else {
			return err
		}
	}
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		event := magicevent.NewEventWithContext(resultevents.TopicResultCommandDelete, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), boundCtx, resultevents.DeleteResultCommand{WorkspaceID: workspaceID, ResultID: item.ID})
		ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicResultCommandDelete, "result", string(item.ID), workspaceID, item.RunID, "")
		if _, err := ownerevent.Result[resultevents.DeleteResultResult](hub.Send(event), resultevents.TopicResultCommandDelete); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	// A failed historical aggregate delete may leave formal child metadata
	// after the parent is gone. Clean only those owner-validated orphans.
	artifacts, err := ownerreadports.ListArtifactsByWorkspaceOwnerCommand(boundCtx, hub, workspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	for _, item := range artifacts {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandDelete, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), boundCtx, resultevents.DeleteArtifactCommand{WorkspaceID: workspaceID, ArtifactID: item.ID})
		ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandDelete, "artifact", string(item.ID), workspaceID, item.RunID, "")
		if _, err := ownerevent.Result[resultevents.DeleteArtifactResult](hub.Send(event), resultevents.TopicArtifactCommandDelete); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
