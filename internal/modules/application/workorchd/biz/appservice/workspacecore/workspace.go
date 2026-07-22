package workspacecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func MapWorkspace(item *workspacemodel.Workspace) appmodelcore.Workspace {
	if item == nil {
		return appmodelcore.Workspace{}
	}
	return appmodelcore.Workspace{
		ID:                   string(item.ID),
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                string(common.CanonicalWorkspaceOwnerState(item.State)),
		ActiveRunID:          string(item.ActiveRunID),
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultLLMTarget:     cloneProviderModelTarget(item.DefaultLLMTarget),
		TargetDeclaration:    cloneTargetDeclaration(item.TargetDeclaration),
		DefaultSoulID:        string(item.DefaultSoulID),
		SoulBindings:         appmodelcore.MapSoulBindingIDs(item.SoulBindings),
		ContextHints:         cloneContextMap(item.ContextHints.Map()),
		EvidencePolicies:     cloneContextMapSlice(workorcheventpayload.MapSlice(item.EvidencePolicies)),
	}
}

func WorkspaceModelFromView(item appmodelcore.Workspace) *workspacemodel.Workspace {
	if item.ID == "" {
		return nil
	}
	contextHints, err := workorcheventpayload.FromMap(item.ContextHints)
	if err != nil {
		return nil
	}
	evidencePolicies, err := workorcheventpayload.FromMapSlice(item.EvidencePolicies)
	if err != nil {
		return nil
	}
	return &workspacemodel.Workspace{
		ID:                   common.WorkspaceID(item.ID),
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                common.WorkspaceState(item.State),
		ActiveRunID:          common.RunID(item.ActiveRunID),
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultLLMTarget:     cloneProviderModelTarget(item.DefaultLLMTarget),
		TargetDeclaration:    cloneTargetDeclaration(item.TargetDeclaration),
		DefaultSoulID:        common.SoulID(item.DefaultSoulID),
		SoulBindings:         SoulBindingsFromView(item.SoulBindings),
		ContextHints:         contextHints,
		EvidencePolicies:     evidencePolicies,
	}
}

func cloneProviderModelTarget(input *runtimecontract.ProviderModelTarget) *runtimecontract.ProviderModelTarget {
	if input == nil {
		return nil
	}
	value := input.Clone()
	return &value
}

func cloneTargetDeclaration(input *runtimecontract.RuntimeTargetDeclaration) *runtimecontract.RuntimeTargetDeclaration {
	if input == nil {
		return nil
	}
	value := input.Clone()
	return &value
}
