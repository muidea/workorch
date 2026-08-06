package appmodelcore

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ProjectWorkspace(item Workspace) projectioncontract.Workspace {
	return projectioncontract.CloneWorkspace(projectioncontract.Workspace{
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
		DefaultLLMTarget:     item.DefaultLLMTarget,
		DefaultSoulID:        item.DefaultSoulID,
		SoulBindings:         item.SoulBindings,
		ContextHints:         item.ContextHints,
		EvidencePolicies:     item.EvidencePolicies,
		Projection:           projectioncontract.ProjectionStatusFromWorkspaceViewModel(item.ViewModel),
		ViewModel:            item.ViewModel,
	})
}

func ProjectWorkspaces(items []Workspace) []projectioncontract.Workspace {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.Workspace, 0, len(items))
	for _, item := range items {
		ret = append(ret, ProjectWorkspace(item))
	}
	return ret
}

func ProjectRun(item Run) projectioncontract.Run {
	return projectioncontract.CloneRun(projectioncontract.Run{
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
		SealedLLMTarget:    item.SealedLLMTarget,
		RuntimeActivation:  item.RuntimeActivation,
		CreatedAt:          item.CreatedAt,
		StartedAt:          item.StartedAt,
		EndedAt:            item.EndedAt,
	})
}

func ProjectRuns(items []Run) []projectioncontract.Run {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.Run, 0, len(items))
	for _, item := range items {
		ret = append(ret, ProjectRun(item))
	}
	return ret
}
