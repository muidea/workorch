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
	commandCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return collectResultOwnerPages(func(cursor string) ([]*resultmodel.Result, string, error) {
		event := magicevent.NewEventWithContext(resultevents.TopicResultCommandListByWorkspace, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), commandCtx, resultevents.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID, Cursor: cursor, Limit: resultevents.MaxListLimit})
		ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicResultCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
		response, err := ownerevent.Result[resultevents.ListResultsByWorkspaceResult](hub.Send(event), resultevents.TopicResultCommandListByWorkspace)
		return response.Results, response.NextCursor, err
	})
}

func ListResultsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*resultmodel.Result, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return collectResultOwnerPages(func(cursor string) ([]*resultmodel.Result, string, error) {
		event := magicevent.NewEventWithContext(resultevents.TopicResultCommandListByRun, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), commandCtx, resultevents.ListResultsByRunCommand{WorkspaceID: workspaceID, RunID: runID, Cursor: cursor, Limit: resultevents.MaxListLimit})
		ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicResultCommandListByRun, "run", string(runID), workspaceID, runID, "")
		response, err := ownerevent.Result[resultevents.ListResultsByRunResult](hub.Send(event), resultevents.TopicResultCommandListByRun)
		return response.Results, response.NextCursor, err
	})
}
