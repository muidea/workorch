package workspaceports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventvalue"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ReadWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, templateID string) (*repository.WorkspaceTemplate, error) {
	return ownerreadports.ReadWorkspaceTemplateByOwnerCommand(ctx, hub, templateID)
}

func ListWorkspaceTemplateRecords(ctx context.Context, hub magicevent.Hub) ([]*repository.WorkspaceTemplate, error) {
	return ownerreadports.ListWorkspaceTemplatesByOwnerCommand(ctx, hub)
}

func CreateWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, item *repository.WorkspaceTemplate) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	command, err := workspaceTemplateCommandFromRepository(item)
	if err != nil {
		return err
	}
	_, err = sendWorkspaceMagicCommand[workspaceevents.CreateWorkspaceTemplateCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandCreate, workspaceevents.CommandDestination, "workspace_template", item.ID, "", "", command)
	return err
}

func UpdateWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, item *repository.WorkspaceTemplate) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	command, err := workspaceTemplateCommandFromRepository(item)
	if err != nil {
		return err
	}
	_, err = sendWorkspaceMagicCommand[workspaceevents.UpdateWorkspaceTemplateCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandUpdate, workspaceevents.CommandDestination, "workspace_template", item.ID, "", "", workspaceevents.UpdateWorkspaceTemplateCommand(command))
	return err
}

func DeleteWorkspaceTemplateRecord(ctx context.Context, hub magicevent.Hub, templateID string) error {
	if templateID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendWorkspaceMagicCommand[workspaceevents.DeleteWorkspaceTemplateCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceTemplateCommandDelete, workspaceevents.CommandDestination, "workspace_template", templateID, "", "", workspaceevents.DeleteWorkspaceTemplateCommand{TemplateID: templateID})
	return err
}

func workspaceTemplateCommandFromRepository(item *repository.WorkspaceTemplate) (workspaceevents.CreateWorkspaceTemplateCommand, error) {
	if item == nil {
		return workspaceevents.CreateWorkspaceTemplateCommand{}, common.ErrInvalidArgument
	}
	contextHints, err := eventvalue.FromMap(item.ContextHints)
	if err != nil {
		return workspaceevents.CreateWorkspaceTemplateCommand{}, fmt.Errorf("invalid workspace template context hints: %w", err)
	}
	evidencePolicies, err := eventvalue.FromMapSlice(item.EvidencePolicies)
	if err != nil {
		return workspaceevents.CreateWorkspaceTemplateCommand{}, fmt.Errorf("invalid workspace template evidence policies: %w", err)
	}
	return workspaceevents.CreateWorkspaceTemplateCommand{
		TemplateID:       item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    item.DefaultSoulID,
		SoulBindings:     cloneStringMap(item.SoulBindings),
		ContextHints:     contextHints,
		EvidencePolicies: evidencePolicies,
	}, nil
}
