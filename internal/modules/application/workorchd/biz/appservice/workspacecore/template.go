package workspacecore

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceTemplate struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	GoalText         string            `json:"goalText,omitempty"`
	DoDItems         []string          `json:"doDItems,omitempty"`
	DefaultSoulID    string            `json:"defaultSoulId,omitempty"`
	SoulBindings     map[string]string `json:"soulBindings,omitempty"`
	ContextHints     map[string]any    `json:"contextHints,omitempty"`
	EvidencePolicies []map[string]any  `json:"evidencePolicies,omitempty"`
}

func NormalizeWorkspaceTemplate(template WorkspaceTemplate) (WorkspaceTemplate, error) {
	item := WorkspaceTemplate{
		ID:               strings.TrimSpace(template.ID),
		Name:             strings.TrimSpace(template.Name),
		Description:      strings.TrimSpace(template.Description),
		GoalText:         strings.TrimSpace(template.GoalText),
		DoDItems:         definitioncore.NormalizeDoDItems(template.DoDItems),
		DefaultSoulID:    strings.TrimSpace(template.DefaultSoulID),
		ContextHints:     cloneContextMap(template.ContextHints),
		EvidencePolicies: cloneContextMapSlice(template.EvidencePolicies),
	}
	if item.ID == "" || item.Name == "" {
		return WorkspaceTemplate{}, common.ErrInvalidArgument
	}
	bindings, err := appmodelcore.NormalizeSoulBindings(template.SoulBindings)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	item.SoulBindings = appmodelcore.MapSoulBindingIDs(bindings)
	return item, nil
}

func ApplyTemplateToWorkspace(item *workspacemodel.Workspace, template WorkspaceTemplate) (*workspacemodel.Workspace, error) {
	if item == nil || strings.TrimSpace(template.ID) == "" {
		return nil, common.ErrInvalidArgument
	}
	bindings, err := appmodelcore.NormalizeSoulBindings(template.SoulBindings)
	if err != nil {
		return nil, err
	}
	templateID := strings.TrimSpace(template.ID)
	item.DefaultSoulID = common.SoulID(strings.TrimSpace(template.DefaultSoulID))
	item.TemplateID = templateID
	item.ProfileID = templateID
	item.SoulBindings = bindings
	item.ContextHints = cloneContextMapWithoutCommonToolDefaults(template.ContextHints)
	item.EvidencePolicies = cloneContextMapSlice(template.EvidencePolicies)
	return item, nil
}

func cloneContextMapWithoutCommonToolDefaults(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		if isCommonToolDefaultContextKey(key) {
			continue
		}
		if mapped, ok := value.(map[string]any); ok {
			cloned := cloneContextMapWithoutCommonToolDefaults(mapped)
			if len(cloned) > 0 {
				ret[key] = cloned
			}
			continue
		}
		ret[key] = cloneContextValue(value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func isCommonToolDefaultContextKey(key string) bool {
	switch key {
	case "defaultMCPServerIds", "defaultCapabilityIds", "managedWorkspaceConfigs":
		return true
	default:
		return false
	}
}
