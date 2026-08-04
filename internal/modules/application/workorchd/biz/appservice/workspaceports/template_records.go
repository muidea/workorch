package workspaceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func ReadWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, templateID string) (*workspacemodel.WorkspaceTemplate, error) {
	return ownerreadports.ReadWorkspaceTemplateByOwnerCommand(ctx, hub, templateID)
}

func ListWorkspaceTemplateRecords(ctx context.Context, hub magicevent.Hub) ([]*workspacemodel.WorkspaceTemplate, error) {
	return ownerreadports.ListWorkspaceTemplatesByOwnerCommand(ctx, hub)
}

func CreateWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, item *workspacemodel.WorkspaceTemplate) error {
	if hub == nil || item == nil {
		return common.ErrInvalidArgument
	}
	command := workspaceTemplateCommandFromOwner(item)
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandCreate, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceTemplateCommandCreate, "workspace_template", item.ID, "", "", "")
	_, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandCreate)
	return err
}

func UpdateWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, item *workspacemodel.WorkspaceTemplate) error {
	if hub == nil || item == nil {
		return common.ErrInvalidArgument
	}
	command := workspaceTemplateCommandFromOwner(item)
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandUpdate, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateWorkspaceTemplateCommand(command))
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceTemplateCommandUpdate, "workspace_template", item.ID, "", "", "")
	_, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandUpdate)
	return err
}

func DeleteWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, templateID string) error {
	if hub == nil || templateID == "" {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandDelete, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DeleteWorkspaceTemplateCommand{TemplateID: templateID})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceTemplateCommandDelete, "workspace_template", templateID, "", "", "")
	_, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceTemplateCommandDelete)
	return err
}

func workspaceTemplateCommandFromOwner(item *workspacemodel.WorkspaceTemplate) workspaceevents.CreateWorkspaceTemplateCommand {
	if item == nil {
		return workspaceevents.CreateWorkspaceTemplateCommand{}
	}
	return workspaceevents.CreateWorkspaceTemplateCommand{
		TemplateID:       item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    item.DefaultSoulID,
		SoulBindings:     soulbinding.FromStringMap(item.SoulBindings),
		ContextHints:     item.ContextHints.Clone(),
		EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies),
	}
}
