package workspaceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func ReadWorkspaceRecord(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return ownerreadports.ReadWorkspaceByOwnerCommand(ctx, hub, workspaceID)
}

func ListWorkspaceRecords(ctx context.Context, hub magicevent.Hub) ([]*workspacemodel.Workspace, error) {
	return ownerreadports.ListWorkspacesByOwnerCommand(ctx, hub)
}

func CreateWorkspaceRecord(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, title string, description string, deliverableDirectory string, internalDirectory string) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandCreate, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.CreateWorkspaceCommand{
		WorkspaceID:          workspaceID,
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
		InternalDirectory:    internalDirectory,
	})
	ownerevent.BindLane(event, workspaceevents.TopicWorkspaceCommandCreate, "workspace", string(workspaceID), workspaceID, "", "")
	_, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandCreate)
	return err
}

func UpdateWorkspaceRecord(ctx context.Context, hub magicevent.Hub, item *workspacemodel.Workspace) error {
	if hub == nil || item == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdate, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateWorkspaceCommand{
		WorkspaceID:          item.ID,
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                item.State,
		ActiveRunID:          item.ActiveRunID,
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultSoulID:        item.DefaultSoulID,
		SoulBindings:         cloneSoulBindingMap(item.SoulBindings),
		ContextHints:         item.ContextHints.Clone(),
		EvidencePolicies:     workorcheventpayload.CloneSlice(item.EvidencePolicies),
	})
	ownerevent.BindLane(event, workspaceevents.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, item.ActiveRunID, "")
	_, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandUpdate)
	return err
}

func DeleteWorkspaceRecord(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDelete, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, workspaceevents.TopicWorkspaceCommandDelete, "workspace", string(workspaceID), workspaceID, "", "")
	_, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandDelete)
	return err
}
