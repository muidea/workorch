// Package eventmapping contains pure conversions from Workspace owner
// EventHub DTOs to detached read models. It owns no state and performs no
// lifecycle transition.
package eventmapping

import (
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
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
		DefaultSoulID: item.DefaultSoulID, SoulBindings: workspaceevents.SoulIDMapFromBindings(item.SoulBindings),
		ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies),
	}
	if item.DefaultLLMTarget != nil {
		value := *item.DefaultLLMTarget
		ret.DefaultLLMTarget = &value
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

func Goal(item *workspaceevents.Goal) *workspacemodel.Goal {
	if item == nil {
		return nil
	}
	return &workspacemodel.Goal{ID: item.ID, WorkspaceID: item.WorkspaceID, Text: item.Text, DefinitionRevision: item.DefinitionRevision, PreviousDefinitionHash: item.PreviousDefinitionHash, PreviousGoalID: item.PreviousGoalID, LineageState: item.LineageState}
}

func Goals(items []*workspaceevents.Goal) []*workspacemodel.Goal {
	if items == nil {
		return nil
	}
	ret := make([]*workspacemodel.Goal, 0, len(items))
	for _, item := range items {
		ret = append(ret, Goal(item))
	}
	return ret
}

func DoD(item *workspaceevents.DoD) *workspacemodel.DoD {
	if item == nil {
		return nil
	}
	return &workspacemodel.DoD{ID: item.ID, WorkspaceID: item.WorkspaceID, Items: append([]string(nil), item.Items...), State: item.State, DefinitionRevision: item.DefinitionRevision, PreviousDefinitionHash: item.PreviousDefinitionHash, PreviousDoDID: item.PreviousDoDID, LineageState: item.LineageState}
}

func DoDs(items []*workspaceevents.DoD) []*workspacemodel.DoD {
	if items == nil {
		return nil
	}
	ret := make([]*workspacemodel.DoD, 0, len(items))
	for _, item := range items {
		ret = append(ret, DoD(item))
	}
	return ret
}

func Template(item *workspaceevents.WorkspaceTemplate) *workspacemodel.WorkspaceTemplate {
	if item == nil {
		return nil
	}
	return &workspacemodel.WorkspaceTemplate{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: workspaceevents.StringMapFromBindings(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}
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
	ret := &workspaceevents.Workspace{Revision: item.Revision, ID: item.ID, Title: item.Title, Description: item.Description, DeliverableDirectory: item.DeliverableDirectory, InternalDirectory: item.InternalDirectory, State: item.State, ActiveRunID: item.ActiveRunID, TemplateID: item.TemplateID, ProfileID: item.ProfileID, DefaultSoulID: item.DefaultSoulID, SoulBindings: workspaceevents.SoulBindingsFromSoulIDMap(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}
	if item.DefaultLLMTarget != nil {
		value := *item.DefaultLLMTarget
		ret.DefaultLLMTarget = &value
	}
	if item.TerminalDelivery != nil {
		value := *item.TerminalDelivery
		ret.TerminalDelivery = &value
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

func GoalDTO(item *workspacemodel.Goal) *workspaceevents.Goal {
	if item == nil {
		return nil
	}
	return &workspaceevents.Goal{ID: item.ID, WorkspaceID: item.WorkspaceID, Text: item.Text, DefinitionRevision: item.DefinitionRevision, PreviousDefinitionHash: item.PreviousDefinitionHash, PreviousGoalID: item.PreviousGoalID, LineageState: item.LineageState}
}

func GoalDTOs(items []*workspacemodel.Goal) []*workspaceevents.Goal {
	if items == nil {
		return nil
	}
	ret := make([]*workspaceevents.Goal, 0, len(items))
	for _, item := range items {
		ret = append(ret, GoalDTO(item))
	}
	return ret
}

func DoDDTO(item *workspacemodel.DoD) *workspaceevents.DoD {
	if item == nil {
		return nil
	}
	return &workspaceevents.DoD{ID: item.ID, WorkspaceID: item.WorkspaceID, Items: append([]string(nil), item.Items...), State: item.State, DefinitionRevision: item.DefinitionRevision, PreviousDefinitionHash: item.PreviousDefinitionHash, PreviousDoDID: item.PreviousDoDID, LineageState: item.LineageState}
}

func DoDDTOs(items []*workspacemodel.DoD) []*workspaceevents.DoD {
	if items == nil {
		return nil
	}
	ret := make([]*workspaceevents.DoD, 0, len(items))
	for _, item := range items {
		ret = append(ret, DoDDTO(item))
	}
	return ret
}

func TemplateDTO(item *workspacemodel.WorkspaceTemplate) *workspaceevents.WorkspaceTemplate {
	if item == nil {
		return nil
	}
	return &workspaceevents.WorkspaceTemplate{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: workspaceevents.SoulBindingsFromStringMap(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}
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
