package ownerreadports

import (
	"context"

	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListResultsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicResultCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID},
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Result)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func ListResultsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*resultmodel.Result, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicResultCommandListByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListResultsByRunCommand{RunID: runID},
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Result)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
