package appmodelcore

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func projectAuthoringSummary(item *AuthoringSummary) *projectioncontract.WorkspaceAuthoringSummary {
	if item == nil {
		return nil
	}
	return &projectioncontract.WorkspaceAuthoringSummary{
		State: item.State, FailurePhase: item.FailurePhase, FailureCode: item.FailureCode,
		ReviewCount: item.ReviewCount, InputRevision: item.InputRevision,
	}
}

func ProjectWorkspace(item Workspace) projectioncontract.Workspace {
	return projectioncontract.CloneWorkspace(projectioncontract.Workspace{
		Revision:                     item.Revision,
		ID:                           item.ID,
		Title:                        item.Title,
		Description:                  item.Description,
		DeliverableDirectory:         item.DeliverableDirectory,
		State:                        item.State,
		ActiveRunID:                  item.ActiveRunID,
		TemplateID:                   item.TemplateID,
		AdmittedStartupInputHash:     item.AdmittedStartupInputHash,
		AdmittedTemplateRevision:     item.AdmittedTemplateRevision,
		AdmittedTemplateSnapshotHash: item.AdmittedTemplateSnapshotHash,
		ProfileID:                    item.ProfileID,
		AuthoringSummary:             projectAuthoringSummary(item.AuthoringSummary),
		DefaultLLMSelection:          item.DefaultLLMSelection,
		DefaultSoulID:                item.DefaultSoulID,
		SoulBindings:                 item.SoulBindings,
		ContextHints:                 item.ContextHints,
		EvidencePolicies:             item.EvidencePolicies,
		Projection:                   projectioncontract.ProjectionStatusFromWorkspaceViewModel(item.ViewModel),
		ViewModel:                    item.ViewModel,
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
		BlockingTicketID:   item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID,
		FailureReason:      item.FailureReason,
		SealedLLMSelection: item.SealedLLMSelection,
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
