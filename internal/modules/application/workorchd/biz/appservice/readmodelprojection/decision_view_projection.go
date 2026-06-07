package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectDecisionView(item readmodelcore.Decision) readmodelcore.Decision {
	return readmodelcore.Decision{
		ID:             item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		DecisionKind:   item.DecisionKind,
		Question:       item.Question,
		State:          item.State,
		ResolutionID:   item.ResolutionID,
		ResolutionType: item.ResolutionType,
		AutoGovernance: item.AutoGovernance,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}
