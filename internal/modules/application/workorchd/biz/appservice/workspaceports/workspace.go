package workspaceports

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadWorkspaceRecord(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return ownerreadports.ReadWorkspaceByOwnerCommand(ctx, hub, workspaceID)
}

func ListWorkspaceRecords(ctx context.Context, hub events.Hub) ([]*workspacemodel.Workspace, error) {
	return ownerreadports.ListWorkspacesByOwnerCommand(ctx, hub)
}

func CreateWorkspaceRecord(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, title string, description string, deliverableDirectory string, internalDirectory string) error {
	_, err := sendWorkspaceMagicCommand[workspaceevents.CreateWorkspaceCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandCreate, workspaceevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", workspaceevents.CreateWorkspaceCommand{
		WorkspaceID:          workspaceID,
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
		InternalDirectory:    internalDirectory,
	})
	return err
}

func UpdateWorkspaceRecord(ctx context.Context, hub events.Hub, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendWorkspaceMagicCommand[workspaceevents.UpdateWorkspaceCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandUpdate, workspaceevents.CommandDestination, "workspace", string(item.ID), item.ID, item.ActiveRunID, workspaceevents.UpdateWorkspaceCommand{
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
		ContextHints:         cloneMap(item.ContextHints),
		EvidencePolicies:     cloneMapSlice(item.EvidencePolicies),
	})
	return err
}

func DeleteWorkspaceRecord(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	_, err := sendWorkspaceMagicCommand[workspaceevents.DeleteWorkspaceCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandDelete, workspaceevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID})
	return err
}
