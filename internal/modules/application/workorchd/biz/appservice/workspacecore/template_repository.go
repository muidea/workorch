package workspacecore

import (
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func WorkspaceTemplateToOwner(item WorkspaceTemplate) (*workspacemodel.WorkspaceTemplate, error) {
	contextHints, err := workorcheventpayload.FromMap(item.ContextHints)
	if err != nil {
		return nil, err
	}
	evidencePolicies, err := workorcheventpayload.FromMapSlice(item.EvidencePolicies)
	if err != nil {
		return nil, err
	}
	return &workspacemodel.WorkspaceTemplate{
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

func WorkspaceTemplateFromOwner(item *workspacemodel.WorkspaceTemplate) WorkspaceTemplate {
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
