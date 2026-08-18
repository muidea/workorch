package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListArtifactsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	commandCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return collectResultOwnerPages(func(cursor string) ([]*resultmodel.Artifact, string, error) {
		event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByWorkspace, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), commandCtx, resultevents.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID, Cursor: cursor, Limit: resultevents.MaxListLimit})
		ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
		response, err := ownerevent.Result[resultevents.ListArtifactsByWorkspaceResult](hub.Send(event), resultevents.TopicArtifactCommandListByWorkspace)
		return response.Artifacts, response.NextCursor, err
	})
}

func ListArtifactsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return collectResultOwnerPages(func(cursor string) ([]*resultmodel.Artifact, string, error) {
		event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByRun, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), commandCtx, resultevents.ListArtifactsByRunCommand{WorkspaceID: workspaceID, RunID: runID, Cursor: cursor, Limit: resultevents.MaxListLimit})
		ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandListByRun, "run", string(runID), workspaceID, runID, "")
		response, err := ownerevent.Result[resultevents.ListArtifactsByRunResult](hub.Send(event), resultevents.TopicArtifactCommandListByRun)
		return response.Artifacts, response.NextCursor, err
	})
}
