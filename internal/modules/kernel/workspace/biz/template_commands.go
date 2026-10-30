package workspace

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func (m *Biz) executeWorkspaceTemplateCommand(ctx context.Context, ev magicevent.Event) (workspaceCommandResponse, error) {
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
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
		if builtin, err := builtinWorkspaceTemplateByID(item.ID); err != nil {
			return workspaceCommandResponse{}, err
		} else if builtin != nil {
			return workspaceCommandResponse{}, common.ErrConflict
		}
		if err := m.validateTemplateChain(ctx, item); err != nil {
			return workspaceCommandResponse{}, err
		}
		if err := m.templates.Create(ctx, item); err != nil {
			return workspaceCommandResponse{}, err
		}
		if _, err := m.resolvedTemplate(ctx, item.ID); err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{Template: workspaceTemplateFromRepository(item)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandGet:
		cmd, ok := ev.Data().(workspaceevents.GetWorkspaceTemplateCommand)
		if !ok || cmd.TemplateID == "" || m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		item, err := m.resolvedTemplate(ctx, cmd.TemplateID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		dto := workspaceTemplateFromRepository(item)
		builtin, err := builtinWorkspaceTemplateByID(item.ID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		dto.Builtin = builtin != nil
		return workspaceCommandResponse{Template: dto}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandList:
		if _, ok := ev.Data().(workspaceevents.ListWorkspaceTemplatesCommand); !ok || m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		items, err := m.currentWorkspaceTemplates(ctx)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		ret := make([]*workspaceevents.WorkspaceTemplate, 0, len(items))
		for _, item := range items {
			effective, err := m.resolvedTemplate(ctx, item.ID)
			if err != nil {
				return workspaceCommandResponse{}, err
			}
			dto := workspaceTemplateFromRepository(effective)
			builtin, err := builtinWorkspaceTemplateByID(item.ID)
			if err != nil {
				return workspaceCommandResponse{}, err
			}
			dto.Builtin = builtin != nil
			ret = append(ret, dto)
		}
		return workspaceCommandResponse{Templates: ret}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandUpdate:
		cmd, ok := ev.Data().(workspaceevents.UpdateWorkspaceTemplateCommand)
		if !ok || m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		item, err := workspaceTemplateFromCommand(workspaceevents.CreateWorkspaceTemplateCommand(cmd))
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if builtin, err := builtinWorkspaceTemplateByID(item.ID); err != nil {
			return workspaceCommandResponse{}, err
		} else if builtin != nil {
			return workspaceCommandResponse{}, common.ErrConflict
		}
		if err := m.validateTemplateChain(ctx, item); err != nil {
			return workspaceCommandResponse{}, err
		}
		if err := m.validateTemplateDescendants(ctx, item); err != nil {
			return workspaceCommandResponse{}, err
		}
		if err := m.templates.Update(ctx, item); err != nil {
			return workspaceCommandResponse{}, err
		}
		if _, err := m.resolvedTemplate(ctx, item.ID); err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{Template: workspaceTemplateFromRepository(item)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandDelete:
		cmd, ok := ev.Data().(workspaceevents.DeleteWorkspaceTemplateCommand)
		if !ok || cmd.TemplateID == "" || m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		builtin, err := builtinWorkspaceTemplateByID(cmd.TemplateID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if builtin != nil {
			return workspaceCommandResponse{}, common.ErrConflict
		}
		items, err := m.currentWorkspaceTemplates(ctx)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		for _, item := range items {
			for _, parentID := range item.ParentTemplateIDs {
				if parentID == cmd.TemplateID {
					return workspaceCommandResponse{}, common.ErrConflict
				}
			}
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
	parents := make(map[string]bool)
	for _, parentID := range cmd.ParentTemplateIDs {
		if parentID == "" || parentID == cmd.TemplateID || strings.TrimSpace(parentID) != parentID || parents[parentID] {
			return nil, common.ErrInvalidArgument
		}
		parents[parentID] = true
	}
	return &workspacemodel.WorkspaceTemplate{
		ID:                cmd.TemplateID,
		ParentTemplateIDs: append([]string(nil), cmd.ParentTemplateIDs...),
		Name:              cmd.Name,
		Description:       cmd.Description,
		GoalText:          cmd.GoalText,
		DoDItems:          append([]string(nil), cmd.DoDItems...),
		DefaultSoulID:     cmd.DefaultSoulID,
		SoulBindings:      soulbinding.StringMap(cmd.SoulBindings),
		ContextHints:      cmd.ContextHints.Clone(),
		EvidencePolicies:  workorcheventpayload.CloneSlice(cmd.EvidencePolicies),
	}, nil
}

func workspaceTemplateFromRepository(item *workspacemodel.WorkspaceTemplate) *workspaceevents.WorkspaceTemplate {
	if item == nil {
		return nil
	}
	return &workspaceevents.WorkspaceTemplate{
		ID:                item.ID,
		ParentTemplateIDs: append([]string(nil), item.ParentTemplateIDs...),
		EffectiveSources:  append([]workspacemodel.TemplateSource(nil), item.EffectiveSources...),
		Name:              item.Name,
		Description:       item.Description,
		GoalText:          item.GoalText,
		DoDItems:          append([]string(nil), item.DoDItems...),
		DefaultSoulID:     item.DefaultSoulID,
		SoulBindings:      soulbinding.FromStringMap(item.SoulBindings),
		ContextHints:      item.ContextHints.Clone(),
		EvidencePolicies:  workorcheventpayload.CloneSlice(item.EvidencePolicies),
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

func (m *Biz) resolvedTemplate(ctx context.Context, id string) (*workspacemodel.WorkspaceTemplate, error) {
	return workspacemodel.ResolveWorkspaceTemplate(id, func(current string) (*workspacemodel.WorkspaceTemplate, error) {
		return m.currentWorkspaceTemplate(ctx, current)
	})
}

func (m *Biz) validateTemplateChain(ctx context.Context, proposed *workspacemodel.WorkspaceTemplate) error {
	_, err := workspacemodel.ResolveWorkspaceTemplate(proposed.ID, func(current string) (*workspacemodel.WorkspaceTemplate, error) {
		if current == proposed.ID {
			return proposed, nil
		}
		return m.currentWorkspaceTemplate(ctx, current)
	})
	return err
}

func (m *Biz) validateTemplateDescendants(ctx context.Context, proposed *workspacemodel.WorkspaceTemplate) error {
	items, err := m.currentWorkspaceTemplates(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == proposed.ID {
			continue
		}
		_, err := workspacemodel.ResolveWorkspaceTemplate(item.ID, func(current string) (*workspacemodel.WorkspaceTemplate, error) {
			if current == proposed.ID {
				return proposed, nil
			}
			return m.currentWorkspaceTemplate(ctx, current)
		})
		if err != nil {
			return err
		}
	}
	return nil
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
