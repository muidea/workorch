package ownerreadports

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ReadWorkspaceTemplateByOwnerCommand(ctx context.Context, hub events.Hub, templateID string) (*repository.WorkspaceTemplate, error) {
	if hub == nil || templateID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[workspaceevents.GetWorkspaceTemplateCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandGet, workspaceevents.CommandDestination, "workspace_template", templateID, workspaceevents.GetWorkspaceTemplateCommand{TemplateID: templateID})
	if err != nil {
		return nil, err
	}
	if response.Template == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Template, nil
}

func ListWorkspaceTemplatesByOwnerCommand(ctx context.Context, hub events.Hub) ([]*repository.WorkspaceTemplate, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[workspaceevents.ListWorkspaceTemplatesCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandList, workspaceevents.CommandDestination, "workspace_template", "all", workspaceevents.ListWorkspaceTemplatesCommand{})
	if err != nil {
		return nil, err
	}
	return response.Templates, nil
}
