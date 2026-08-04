package appmodelcore

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ProjectWorkspace(item Workspace) projectioncontract.Workspace {
	return projectioncontract.Workspace{
		ID:                   item.ID,
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		State:                item.State,
		ActiveRunID:          item.ActiveRunID,
		BlockingTicketID:     item.BlockingTicketID,
		BlockingApprovalID:   item.BlockingApprovalID,
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultLLMTarget:     runtimecontract.CloneProviderModelTarget(item.DefaultLLMTarget),
		DefaultSoulID:        item.DefaultSoulID,
		SoulBindings:         cloneStringMap(item.SoulBindings),
		ContextHints:         cloneAnyMap(item.ContextHints),
		EvidencePolicies:     cloneAnyMapSlice(item.EvidencePolicies),
		ViewModel:            cloneWorkspaceViewModel(item.ViewModel),
	}
}

func ProjectWorkspaces(items []Workspace) []projectioncontract.Workspace {
	ret := make([]projectioncontract.Workspace, 0, len(items))
	for _, item := range items {
		ret = append(ret, ProjectWorkspace(item))
	}
	return ret
}

func ProjectRun(item Run) projectioncontract.Run {
	ret := projectioncontract.Run{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		GoalID:             item.GoalID,
		DoDID:              item.DoDID,
		Lineage:            item.Lineage,
		DefinitionRevision: item.DefinitionRevision,
		InitialStepKind:    item.InitialStepKind,
		State:              item.State,
		CurrentStepID:      item.CurrentStepID,
		CurrentStepKind:    item.CurrentStepKind,
		CurrentStepState:   item.CurrentStepState,
		Priority:           item.Priority,
		BlockingTicketID:   item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID,
		FailureReason:      item.FailureReason,
		SealedLLMTarget:    runtimecontract.CloneProviderModelTarget(item.SealedLLMTarget),
		CreatedAt:          item.CreatedAt,
		StartedAt:          item.StartedAt,
		EndedAt:            item.EndedAt,
	}
	if item.RuntimeActivation != nil {
		value := *item.RuntimeActivation
		ret.RuntimeActivation = &value
	}
	return ret
}

func ProjectRuns(items []Run) []projectioncontract.Run {
	ret := make([]projectioncontract.Run, 0, len(items))
	for _, item := range items {
		ret = append(ret, ProjectRun(item))
	}
	return ret
}

func cloneWorkspaceViewModel(item projectioncontract.WorkspaceViewModel) projectioncontract.WorkspaceViewModel {
	ret := item
	ret.FailedWorkspaceIds = append([]string(nil), item.FailedWorkspaceIds...)
	if item.Sections != nil {
		ret.Sections = make(map[string]projectioncontract.WorkspaceViewModelSection, len(item.Sections))
		for key, value := range item.Sections {
			ret.Sections[key] = value
		}
	}
	return ret
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneAnyMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneAnyMap(item))
	}
	return ret
}
