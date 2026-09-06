// Package eventmapping contains pure conversions from Workspace owner
// EventHub DTOs to detached read models. It owns no state and performs no
// lifecycle transition.
package eventmapping

import (
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func Workspace(item *workspaceevents.Workspace) *workspacemodel.Workspace {
	if item == nil {
		return nil
	}
	ret := &workspacemodel.Workspace{
		Revision: item.Revision, ID: item.ID, Title: item.Title, Description: item.Description,
		DeliverableDirectory: item.DeliverableDirectory, InternalDirectory: item.InternalDirectory,
		State: item.State, ActiveRunID: item.ActiveRunID, TemplateID: item.TemplateID, ProfileID: item.ProfileID,
		TemplateBindingHash: item.TemplateBindingHash,
		DefaultSoulID:       item.DefaultSoulID, SoulBindings: soulbinding.SoulIDMap(item.SoulBindings),
		ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies),
	}
	ret.DefinitionAdmissionCandidates = cloneAdmissionCandidates(item.DefinitionAdmissionCandidates)
	if item.StartupInput != nil {
		value := item.StartupInput.Clone()
		ret.StartupInput = &value
	}
	if item.ActivationAdmission != nil {
		value := item.ActivationAdmission.Normalized()
		ret.ActivationAdmission = &value
	}
	if item.DefaultLLMSelection != nil {
		value := *item.DefaultLLMSelection
		ret.DefaultLLMSelection = &value
	}
	if item.TerminalDelivery != nil {
		value := *item.TerminalDelivery
		ret.TerminalDelivery = &value
	}
	return ret
}

func Workspaces(items []*workspaceevents.Workspace) []*workspacemodel.Workspace {
	if items == nil {
		return nil
	}
	ret := make([]*workspacemodel.Workspace, 0, len(items))
	for _, item := range items {
		ret = append(ret, Workspace(item))
	}
	return ret
}

func Template(item *workspaceevents.WorkspaceTemplate) *workspacemodel.WorkspaceTemplate {
	if item == nil {
		return nil
	}
	return &workspacemodel.WorkspaceTemplate{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: soulbinding.StringMap(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}
}

func Templates(items []*workspaceevents.WorkspaceTemplate) []*workspacemodel.WorkspaceTemplate {
	if items == nil {
		return nil
	}
	ret := make([]*workspacemodel.WorkspaceTemplate, 0, len(items))
	for _, item := range items {
		ret = append(ret, Template(item))
	}
	return ret
}

func WorkspaceDTO(item *workspacemodel.Workspace) *workspaceevents.Workspace {
	if item == nil {
		return nil
	}
	ret := &workspaceevents.Workspace{Revision: item.Revision, ID: item.ID, Title: item.Title, Description: item.Description, DeliverableDirectory: item.DeliverableDirectory, InternalDirectory: item.InternalDirectory, State: item.State, ActiveRunID: item.ActiveRunID, TemplateID: item.TemplateID, ProfileID: item.ProfileID, DefaultSoulID: item.DefaultSoulID, SoulBindings: soulbinding.FromSoulIDMap(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}
	ret.TemplateBindingHash = item.TemplateBindingHash
	ret.DefinitionAdmissionCandidates = cloneAdmissionCandidates(item.DefinitionAdmissionCandidates)
	if item.StartupInput != nil {
		value := item.StartupInput.Clone()
		ret.StartupInput = &value
	}
	if item.ActivationAdmission != nil {
		value := item.ActivationAdmission.Normalized()
		ret.ActivationAdmission = &value
	}
	if item.DefaultLLMSelection != nil {
		value := *item.DefaultLLMSelection
		ret.DefaultLLMSelection = &value
	}
	if item.TerminalDelivery != nil {
		value := *item.TerminalDelivery
		ret.TerminalDelivery = &value
	}
	return ret
}

func cloneAdmissionCandidates(items []workspacemodel.DefinitionAdmissionCandidate) []workspacemodel.DefinitionAdmissionCandidate {
	if items == nil {
		return nil
	}
	ret := make([]workspacemodel.DefinitionAdmissionCandidate, len(items))
	for idx := range items {
		ret[idx] = items[idx].Clone()
	}
	return ret
}

func WorkspaceDTOs(items []*workspacemodel.Workspace) []*workspaceevents.Workspace {
	if items == nil {
		return nil
	}
	ret := make([]*workspaceevents.Workspace, 0, len(items))
	for _, item := range items {
		ret = append(ret, WorkspaceDTO(item))
	}
	return ret
}

func TemplateDTO(item *workspacemodel.WorkspaceTemplate) *workspaceevents.WorkspaceTemplate {
	if item == nil {
		return nil
	}
	return &workspaceevents.WorkspaceTemplate{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: soulbinding.FromStringMap(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}
}

func TemplateDTOs(items []*workspacemodel.WorkspaceTemplate) []*workspaceevents.WorkspaceTemplate {
	if items == nil {
		return nil
	}
	ret := make([]*workspaceevents.WorkspaceTemplate, 0, len(items))
	for _, item := range items {
		ret = append(ret, TemplateDTO(item))
	}
	return ret
}
