package workspaceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
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
	return ownercommandports.Publish(ctx, hub, events.TopicWorkspaceCommandCreate, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: workspacecontracts.CreateWorkspaceCommand{
			WorkspaceID:          workspaceID,
			Title:                title,
			Description:          description,
			DeliverableDirectory: deliverableDirectory,
			InternalDirectory:    internalDirectory,
		},
	})
}

func UpdateWorkspaceRecord(ctx context.Context, hub events.Hub, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	return ownercommandports.Publish(ctx, hub, events.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, item.ActiveRunID, map[string]any{
		events.PayloadKeyCommand: workspacecontracts.UpdateWorkspaceCommand{
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
		},
	})
}

func DeleteWorkspaceRecord(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	return ownercommandports.Publish(ctx, hub, events.TopicWorkspaceCommandDelete, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: workspacecontracts.DeleteWorkspaceCommand{WorkspaceID: workspaceID},
	})
}
