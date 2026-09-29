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
		ID:                item.ID,
		ParentTemplateIDs: append([]string(nil), item.ParentTemplateIDs...),
		EffectiveSources:  fromManagementTemplateSources(item.EffectiveSources),
		Name:              item.Name,
		Description:       item.Description,
		GoalText:          item.GoalText,
		DoDItems:          append([]string(nil), item.DoDItems...),
		DefaultSoulID:     item.DefaultSoulID,
		SoulBindings:      cloneStringMap(item.SoulBindings),
		ContextHints:      contextHints,
		EvidencePolicies:  evidencePolicies,
	}, nil
}

func WorkspaceTemplateFromOwner(item *workspacemodel.WorkspaceTemplate) workorchmanagementcontract.WorkspaceTemplate {
	if item == nil {
		return workorchmanagementcontract.WorkspaceTemplate{}
	}
	return workorchmanagementcontract.WorkspaceTemplate{
		ID:                item.ID,
		ParentTemplateIDs: append([]string(nil), item.ParentTemplateIDs...),
		EffectiveSources:  toManagementTemplateSources(item.EffectiveSources),
		Name:              item.Name,
		Description:       item.Description,
		GoalText:          item.GoalText,
		DoDItems:          append([]string(nil), item.DoDItems...),
		DefaultSoulID:     item.DefaultSoulID,
		SoulBindings:      cloneStringMap(item.SoulBindings),
		ContextHints:      item.ContextHints.Map(),
		EvidencePolicies:  workorcheventpayload.MapSlice(item.EvidencePolicies),
	}
}

func fromManagementTemplateSources(items []workorchmanagementcontract.TemplateSource) []workspacemodel.TemplateSource {
	if len(items) == 0 {
		return nil
	}
	ret := make([]workspacemodel.TemplateSource, len(items))
	for idx, item := range items {
		ret[idx] = workspacemodel.TemplateSource{TemplateID: item.TemplateID, ContentHash: item.ContentHash}
	}
	return ret
}

func toManagementTemplateSources(items []workspacemodel.TemplateSource) []workorchmanagementcontract.TemplateSource {
	if len(items) == 0 {
		return nil
	}
	ret := make([]workorchmanagementcontract.TemplateSource, len(items))
	for idx, item := range items {
		ret[idx] = workorchmanagementcontract.TemplateSource{TemplateID: item.TemplateID, ContentHash: item.ContentHash}
	}
	return ret
}
