package definitionports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDoD(ctx context.Context, hub magicevent.Hub, dodID common.DoDID) (*workspacemodel.DoD, error) {
	return ownerreadports.ReadDoDByOwnerCommand(ctx, hub, dodID)
}

func ListDoDsByWorkspace(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	return ownerreadports.ListDoDsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
}

func DefineDoD(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, dodID common.DoDID, items []string) (*workspacemodel.DoD, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDefineDoD, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DefineDoDCommand{
		DoDID:       dodID,
		WorkspaceID: workspaceID,
		Items:       append([]string(nil), items...),
	})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandDefineDoD, "dod", string(dodID), workspaceID, "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandDefineDoD)
	if err != nil {
		return nil, err
	}
	if response.DoD == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.DoD, nil
}

func UpdateDoD(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, dodID common.DoDID, items []string) (*workspacemodel.DoD, error) {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdateDoD, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateDoDCommand{
		WorkspaceID: workspaceID,
		DoDID:       dodID,
		Items:       append([]string(nil), items...),
	})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandUpdateDoD, "dod", string(dodID), workspaceID, "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandUpdateDoD)
	if err != nil {
		return nil, err
	}
	if response.DoD == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.DoD, nil
}

func DeleteDoD(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, dodID common.DoDID) error {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDeleteDoD, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DeleteDoDCommand{WorkspaceID: workspaceID, DoDID: dodID})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandDeleteDoD, "dod", string(dodID), workspaceID, "", "")
	_, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandDeleteDoD)
	return err
}
