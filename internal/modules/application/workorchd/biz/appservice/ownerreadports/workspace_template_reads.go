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
	response, err := PublishCommandWithResponse(ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandGet, "workspace_template", templateID, "", "", map[string]any{
		events.PayloadKeyTemplateID: templateID,
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyWorkspaceTemplate].(*repository.WorkspaceTemplate)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ListWorkspaceTemplatesByOwnerCommand(ctx context.Context, hub events.Hub) ([]*repository.WorkspaceTemplate, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandList, "workspace_template", "all", "", "", map[string]any{})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyWorkspaceTemplates].([]*repository.WorkspaceTemplate)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
