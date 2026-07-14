package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ReadWorkspaceTemplateByOwnerCommand(ctx context.Context, hub magicevent.Hub, templateID string) (*repository.WorkspaceTemplate, error) {
	if hub == nil || templateID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandGet, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetWorkspaceTemplateCommand{TemplateID: templateID})
	ownerevent.BindLane(event, workspaceevents.TopicWorkspaceTemplateCommandGet, "workspace_template", templateID, "", "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Template == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Template, nil
}

func ListWorkspaceTemplatesByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*repository.WorkspaceTemplate, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandList, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListWorkspaceTemplatesCommand{})
	ownerevent.BindLane(event, workspaceevents.TopicWorkspaceTemplateCommandList, "workspace_template", "all", "", "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandList)
	if err != nil {
		return nil, err
	}
	return response.Templates, nil
}
