package workspace

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func (m *Biz) executeWorkspaceTemplateCommand(ctx context.Context, ev magicevent.Event) (workspaceCommandResponse, error) {
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceTemplateCommandCreate:
		cmd, ok := ev.Data().(workspaceevents.CreateWorkspaceTemplateCommand)
		if !ok || m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		item, err := workspaceTemplateFromCommand(workspaceevents.CreateWorkspaceTemplateCommand(cmd))
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if err := m.templates.Create(ctx, item); err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{Template: workspaceTemplateFromRepository(item)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandGet:
		cmd, ok := ev.Data().(workspaceevents.GetWorkspaceTemplateCommand)
		if !ok || cmd.TemplateID == "" || m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		item, err := m.templates.Get(ctx, cmd.TemplateID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{Template: workspaceTemplateFromRepository(item)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandList:
		if _, ok := ev.Data().(workspaceevents.ListWorkspaceTemplatesCommand); !ok || m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		items, err := m.templates.List(ctx)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{Templates: workspaceTemplatesFromRepository(items)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandUpdate:
		cmd, ok := ev.Data().(workspaceevents.UpdateWorkspaceTemplateCommand)
		if !ok || m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		item, err := workspaceTemplateFromCommand(workspaceevents.CreateWorkspaceTemplateCommand(cmd))
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if err := m.templates.Update(ctx, item); err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{Template: workspaceTemplateFromRepository(item)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandDelete:
		cmd, ok := ev.Data().(workspaceevents.DeleteWorkspaceTemplateCommand)
		if !ok || cmd.TemplateID == "" || m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		if err := m.templates.Delete(ctx, cmd.TemplateID); err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{}, nil
	default:
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
}

func workspaceTemplateFromCommand(cmd workspaceevents.CreateWorkspaceTemplateCommand) (*workspacemodel.WorkspaceTemplate, error) {
	if cmd.TemplateID == "" || soulbinding.Validate(cmd.SoulBindings) != nil {
		return nil, common.ErrInvalidArgument
	}
	return &workspacemodel.WorkspaceTemplate{
		ID:               cmd.TemplateID,
		Name:             cmd.Name,
		Description:      cmd.Description,
		GoalText:         cmd.GoalText,
		DoDItems:         append([]string(nil), cmd.DoDItems...),
		DefaultSoulID:    cmd.DefaultSoulID,
		SoulBindings:     soulbinding.StringMap(cmd.SoulBindings),
		ContextHints:     cmd.ContextHints.Clone(),
		EvidencePolicies: workorcheventpayload.CloneSlice(cmd.EvidencePolicies),
	}, nil
}

func workspaceTemplateFromRepository(item *workspacemodel.WorkspaceTemplate) *workspaceevents.WorkspaceTemplate {
	if item == nil {
		return nil
	}
	return &workspaceevents.WorkspaceTemplate{
		ID:               item.ID,
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

func workspaceTemplatesFromRepository(items []*workspacemodel.WorkspaceTemplate) []*workspaceevents.WorkspaceTemplate {
	if items == nil {
		return nil
	}
	ret := make([]*workspaceevents.WorkspaceTemplate, 0, len(items))
	for _, item := range items {
		ret = append(ret, workspaceTemplateFromRepository(item))
	}
	return ret
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
