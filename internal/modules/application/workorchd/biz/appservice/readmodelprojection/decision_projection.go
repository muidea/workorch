package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapDecision(item *decisionmodel.Ticket, resolution *decisionmodel.Resolution, autoGovernance *projectioncontract.AutoGovernanceSummary) projectioncontract.Decision {
	if item == nil {
		return projectioncontract.Decision{}
	}
	ret := projectioncontract.Decision{
		ID:                     string(item.ID),
		WorkspaceID:            string(item.WorkspaceID),
		RunID:                  string(item.RunID),
		StepID:                 string(item.StepID),
		Reason:                 item.Reason,
		DecisionKind:           item.DecisionKind,
		Question:               item.Question,
		Context:                governancecore.CloneDecisionPayload(item.Context),
		FailureClass:           firstNonEmptyReadModelString(item.FailureClass, governancecore.StringFromDecisionPayload(item.Context, "failureClass")),
		FailureSignature:       firstNonEmptyReadModelString(item.FailureSignature, governancecore.StringFromDecisionPayload(item.Context, "failureSignature")),
		AllowedRecoveryActions: firstNonEmptyReadModelStringSlice(item.AllowedRecoveryActions, governancecore.RuntimeRepairAllowedRecoveryActions(item.Context)),
		State:                  string(item.State),
		AutoGovernance:         autoGovernance,
		ResolvedAt:             item.ResolvedAt,
		ResolvedBy:             item.ResolvedBy,
		CreatedAt:              item.CreatedAt,
		UpdatedAt:              item.UpdatedAt,
	}
	if resolution != nil {
		ret.ResolutionID = string(resolution.ID)
		ret.ResolutionType = resolution.Type
		ret.ResolutionPayload = governancecore.CloneDecisionPayload(resolution.Payload)
		ret.ContinuationAction = resolution.ContinuationAction
		ret.ResumeRunID = string(resolution.ResumeRunID)
		ret.SuccessorRunID = string(resolution.SuccessorRunID)
	}
	return ret
}

func firstNonEmptyReadModelString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyReadModelStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return append([]string(nil), value...)
		}
	}
	return nil
}
