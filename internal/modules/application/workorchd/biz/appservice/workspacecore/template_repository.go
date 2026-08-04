package workspacecore

import (
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func WorkspaceTemplateToOwner(item workorchmanagementcontract.WorkspaceTemplate) (*workspacemodel.WorkspaceTemplate, error) {
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

func WorkspaceTemplateFromOwner(item *workspacemodel.WorkspaceTemplate) workorchmanagementcontract.WorkspaceTemplate {
	if item == nil {
		return workorchmanagementcontract.WorkspaceTemplate{}
	}
	return workorchmanagementcontract.WorkspaceTemplate{
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
