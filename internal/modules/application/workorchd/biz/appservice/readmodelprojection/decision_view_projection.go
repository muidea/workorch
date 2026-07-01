package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectDecisionView(item readmodelcore.Decision) readmodelcore.Decision {
	return readmodelcore.Decision{
		ID:                     item.ID,
		WorkspaceID:            item.WorkspaceID,
		RunID:                  item.RunID,
		StepID:                 item.StepID,
		Reason:                 item.Reason,
		DecisionKind:           item.DecisionKind,
		Question:               item.Question,
		Context:                ProjectDecisionContextView(item.Context),
		FailureClass:           item.FailureClass,
		FailureSignature:       item.FailureSignature,
		AllowedRecoveryActions: append([]string(nil), item.AllowedRecoveryActions...),
		State:                  item.State,
		ResolutionID:           item.ResolutionID,
		ResolutionType:         item.ResolutionType,
		ResolutionPayload:      ProjectDecisionPayloadView(item.ResolutionPayload),
		AutoGovernance:         item.AutoGovernance,
		CreatedAt:              item.CreatedAt,
		UpdatedAt:              item.UpdatedAt,
	}
}
