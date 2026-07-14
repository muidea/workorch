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
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByWorkspace, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, resultevents.TopicArtifactCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Result(hub.Send(event), resultevents.TopicArtifactCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}

func ListArtifactsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByRun, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), commandCtx, resultevents.ListArtifactsByRunCommand{RunID: runID})
	ownerevent.BindLane(event, resultevents.TopicArtifactCommandListByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Result(hub.Send(event), resultevents.TopicArtifactCommandListByRun)
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}
