package workspaceports

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ReadWorkspaceTemplateRecord(ctx context.Context, hub events.Hub, templateID string) (*repository.WorkspaceTemplate, error) {
	return ownerreadports.ReadWorkspaceTemplateByOwnerCommand(ctx, hub, templateID)
}

func ListWorkspaceTemplateRecords(ctx context.Context, hub events.Hub) ([]*repository.WorkspaceTemplate, error) {
	return ownerreadports.ListWorkspaceTemplatesByOwnerCommand(ctx, hub)
}

func CreateWorkspaceTemplateRecord(ctx context.Context, hub events.Hub, item *repository.WorkspaceTemplate) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendWorkspaceMagicCommand[workspaceevents.CreateWorkspaceTemplateCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandCreate, workspaceevents.CommandDestination, "workspace_template", item.ID, "", "", workspaceTemplateCommandFromRepository(item))
	return err
}

func UpdateWorkspaceTemplateRecord(ctx context.Context, hub events.Hub, item *repository.WorkspaceTemplate) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendWorkspaceMagicCommand[workspaceevents.UpdateWorkspaceTemplateCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandUpdate, workspaceevents.CommandDestination, "workspace_template", item.ID, "", "", workspaceevents.UpdateWorkspaceTemplateCommand(workspaceTemplateCommandFromRepository(item)))
	return err
}

func DeleteWorkspaceTemplateRecord(ctx context.Context, hub events.Hub, templateID string) error {
	if templateID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendWorkspaceMagicCommand[workspaceevents.DeleteWorkspaceTemplateCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandDelete, workspaceevents.CommandDestination, "workspace_template", templateID, "", "", workspaceevents.DeleteWorkspaceTemplateCommand{TemplateID: templateID})
	return err
}

func workspaceTemplateCommandFromRepository(item *repository.WorkspaceTemplate) workspacecontracts.CreateWorkspaceTemplateCommand {
	if item == nil {
		return workspacecontracts.CreateWorkspaceTemplateCommand{}
	}
	return workspacecontracts.CreateWorkspaceTemplateCommand{
		TemplateID:       item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    item.DefaultSoulID,
		SoulBindings:     cloneStringMap(item.SoulBindings),
		ContextHints:     cloneMap(item.ContextHints),
		EvidencePolicies: cloneMapSlice(item.EvidencePolicies),
	}
}
