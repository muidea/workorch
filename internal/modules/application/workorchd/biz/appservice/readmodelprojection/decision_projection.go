package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
)

func MapDecision(item *decisionmodel.Ticket, resolution *decisionmodel.Resolution, autoGovernance *governancecore.AutoGovernanceSummary) readmodelcore.Decision {
	if item == nil {
		return readmodelcore.Decision{}
	}
	ret := readmodelcore.Decision{
		ID:             string(item.ID),
		WorkspaceID:    string(item.WorkspaceID),
		RunID:          string(item.RunID),
		StepID:         string(item.StepID),
		Reason:         item.Reason,
		DecisionKind:   item.DecisionKind,
		Question:       item.Question,
		Context:        governancecore.CloneDecisionPayload(item.Context),
		State:          string(item.State),
		AutoGovernance: autoGovernance,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
	if resolution != nil {
		ret.ResolutionID = string(resolution.ID)
		ret.ResolutionType = resolution.Type
		ret.ResolutionPayload = governancecore.CloneDecisionPayload(resolution.Payload)
	}
	return ret
}
