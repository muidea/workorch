package workspacecore

import (
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func WorkspaceTemplateToRepository(item WorkspaceTemplate) (*repository.WorkspaceTemplate, error) {
	contextHints, err := workorcheventpayload.FromMap(item.ContextHints)
	if err != nil {
		return nil, err
	}
	evidencePolicies, err := workorcheventpayload.FromMapSlice(item.EvidencePolicies)
	if err != nil {
		return nil, err
	}
	return &repository.WorkspaceTemplate{
		ID:               item.ID,
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
		ContextHints:     item.ContextHints.Map(),
		EvidencePolicies: workorcheventpayload.MapSlice(item.EvidencePolicies),
	}
}
