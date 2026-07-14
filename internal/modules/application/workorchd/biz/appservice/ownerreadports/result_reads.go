package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListResultsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandListByWorkspace, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, resultevents.TopicResultCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Result(hub.Send(event), resultevents.TopicResultCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

func ListResultsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*resultmodel.Result, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandListByRun, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), commandCtx, resultevents.ListResultsByRunCommand{RunID: runID})
	ownerevent.BindLane(event, resultevents.TopicResultCommandListByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Result(hub.Send(event), resultevents.TopicResultCommandListByRun)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}
