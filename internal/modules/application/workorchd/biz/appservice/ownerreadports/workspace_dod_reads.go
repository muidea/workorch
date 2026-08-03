package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDoDByOwnerCommand(ctx context.Context, hub magicevent.Hub, dodID common.DoDID) (*workspacemodel.DoD, error) {
	if hub == nil || dodID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetDoD, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), commandCtx, workspaceevents.GetDoDCommand{
		WorkspaceID: workspaceID,
		DoDID:       dodID,
	})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandGetDoD, "dod", string(dodID), workspaceID, "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandGetDoD)
	if err != nil {
		return nil, err
	}
	if response.DoD == nil {
		return nil, common.ErrInvalidArgument
	}
	return workspaceeventmapping.DoD(response.DoD), nil
}

func ListDoDsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandListDoDsByWS, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), commandCtx, workspaceevents.ListDoDsByWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandListDoDsByWS, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandListDoDsByWS)
	if err != nil {
		return nil, err
	}
	return workspaceeventmapping.DoDs(response.DoDs), nil
}
