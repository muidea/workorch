package workspaceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
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
	return ownercommandports.Publish(ctx, hub, events.TopicWorkspaceTemplateCommandCreate, "workspace_template", item.ID, "", "", map[string]any{
		events.PayloadKeyCommand: workspaceTemplateCommandFromRepository(item),
	})
}

func UpdateWorkspaceTemplateRecord(ctx context.Context, hub events.Hub, item *repository.WorkspaceTemplate) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	return ownercommandports.Publish(ctx, hub, events.TopicWorkspaceTemplateCommandUpdate, "workspace_template", item.ID, "", "", map[string]any{
		events.PayloadKeyCommand: workspaceTemplateCommandFromRepository(item),
	})
}

func DeleteWorkspaceTemplateRecord(ctx context.Context, hub events.Hub, templateID string) error {
	if templateID == "" {
		return common.ErrInvalidArgument
	}
	return ownercommandports.Publish(ctx, hub, events.TopicWorkspaceTemplateCommandDelete, "workspace_template", templateID, "", "", map[string]any{
		events.PayloadKeyCommand: workspacecontracts.DeleteWorkspaceTemplateCommand{TemplateID: templateID},
	})
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
