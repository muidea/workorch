package workspacecore

import "github.com/muidea/workorch/internal/pkg/repository"

func WorkspaceTemplateToRepository(item WorkspaceTemplate) *repository.WorkspaceTemplate {
	return &repository.WorkspaceTemplate{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    item.DefaultSoulID,
		SoulBindings:     cloneStringMap(item.SoulBindings),
		ContextHints:     cloneContextMap(item.ContextHints),
		EvidencePolicies: cloneContextMapSlice(item.EvidencePolicies),
	}
}

func WorkspaceTemplateFromRepository(item *repository.WorkspaceTemplate) WorkspaceTemplate {
	if item == nil {
		return WorkspaceTemplate{}
	}
	return WorkspaceTemplate{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    item.DefaultSoulID,
		SoulBindings:     cloneStringMap(item.SoulBindings),
		ContextHints:     cloneContextMap(item.ContextHints),
		EvidencePolicies: cloneContextMapSlice(item.EvidencePolicies),
	}
}
