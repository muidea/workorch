package workspaceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func ReadWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, templateID string) (*repository.WorkspaceTemplate, error) {
	return ownerreadports.ReadWorkspaceTemplateByOwnerCommand(ctx, hub, templateID)
}

func ListWorkspaceTemplateRecords(ctx context.Context, hub magicevent.Hub) ([]*repository.WorkspaceTemplate, error) {
	return ownerreadports.ListWorkspaceTemplatesByOwnerCommand(ctx, hub)
}

func CreateWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, item *repository.WorkspaceTemplate) error {
	if hub == nil || item == nil {
		return common.ErrInvalidArgument
	}
	command, err := workspaceTemplateCommandFromRepository(item)
	if err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandCreate, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.TopicWorkspaceTemplateCommandCreate, "workspace_template", item.ID, "", "", "")
	_, err = ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandCreate)
	return err
}

func UpdateWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, item *repository.WorkspaceTemplate) error {
	if hub == nil || item == nil {
		return common.ErrInvalidArgument
	}
	command, err := workspaceTemplateCommandFromRepository(item)
	if err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandUpdate, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateWorkspaceTemplateCommand(command))
	ownerevent.BindLane(event, workspaceevents.TopicWorkspaceTemplateCommandUpdate, "workspace_template", item.ID, "", "", "")
	_, err = ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandUpdate)
	return err
}

func DeleteWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, templateID string) error {
	if hub == nil || templateID == "" {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandDelete, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DeleteWorkspaceTemplateCommand{TemplateID: templateID})
	ownerevent.BindLane(event, workspaceevents.TopicWorkspaceTemplateCommandDelete, "workspace_template", templateID, "", "", "")
	_, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandDelete)
	return err
}

func workspaceTemplateCommandFromRepository(item *repository.WorkspaceTemplate) (workspaceevents.CreateWorkspaceTemplateCommand, error) {
	if item == nil {
		return workspaceevents.CreateWorkspaceTemplateCommand{}, common.ErrInvalidArgument
	}
	return workspaceevents.CreateWorkspaceTemplateCommand{
		TemplateID:       item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    item.DefaultSoulID,
		SoulBindings:     cloneStringMap(item.SoulBindings),
		ContextHints:     item.ContextHints.Clone(),
		EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies),
	}, nil
}
