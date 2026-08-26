package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadWorkspaceTemplateByOwnerCommand(ctx context.Context, hub magicevent.Hub, templateID string) (*workspacemodel.WorkspaceTemplate, error) {
	if hub == nil || templateID == "" {
		return nil, common.ErrInvalidArgument
	}
	command := workspaceevents.GetWorkspaceTemplateCommand{TemplateID: templateID}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandGet, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceTemplateCommandGet, "workspace_template", templateID, "", "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.WorkspaceTemplateResult](hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandGet)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspaceeventmapping.Template(response.Template), nil
}

func ListWorkspaceTemplatesByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*workspacemodel.WorkspaceTemplate, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	command := workspaceevents.ListWorkspaceTemplatesCommand{}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandList, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceTemplateCommandList, "workspace_template", "all", "", "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.WorkspaceTemplateListResult](hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandList)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspaceeventmapping.Templates(response.Templates), nil
}
