package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListResultsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[resultevents.ListResultsByWorkspaceCommand, resultevents.ResultCommandResult](ctx, hub, resultevents.TopicResultCommandListByWorkspace, resultevents.CommandDestination, "workspace", string(workspaceID), resultevents.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID})
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
	response, err := sendMagicCommand[resultevents.ListResultsByRunCommand, resultevents.ResultCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, resultevents.TopicResultCommandListByRun, resultevents.CommandDestination, "run", string(runID), resultevents.ListResultsByRunCommand{RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}
