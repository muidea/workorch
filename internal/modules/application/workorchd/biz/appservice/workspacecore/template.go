package workspacecore

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func NormalizeWorkspaceTemplate(template workorchmanagementcontract.WorkspaceTemplate) (workorchmanagementcontract.WorkspaceTemplate, error) {
	item := workorchmanagementcontract.WorkspaceTemplate{
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
		return workorchmanagementcontract.WorkspaceTemplate{}, common.ErrInvalidArgument
	}
	bindings, err := appmodelcore.NormalizeSoulBindings(template.SoulBindings)
	if err != nil {
		return workorchmanagementcontract.WorkspaceTemplate{}, err
	}
	item.SoulBindings = appmodelcore.MapSoulBindingIDs(bindings)
	return item, nil
}

func ApplyTemplateToWorkspace(item *workspacemodel.Workspace, template workorchmanagementcontract.WorkspaceTemplate) (*workspacemodel.Workspace, error) {
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
	contextHints, err := workorcheventpayload.FromMap(cloneContextMapWithoutCommonToolDefaults(template.ContextHints))
	if err != nil {
		return nil, err
	}
	evidencePolicies, err := workorcheventpayload.FromMapSlice(template.EvidencePolicies)
	if err != nil {
		return nil, err
	}
	item.ContextHints = contextHints
	item.EvidencePolicies = evidencePolicies
	record, err := WorkspaceTemplateToOwner(template)
	if err != nil {
		return nil, err
	}
	item.TemplateBindingHash = workspacemodel.WorkspaceTemplateSnapshotHash(record)
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
